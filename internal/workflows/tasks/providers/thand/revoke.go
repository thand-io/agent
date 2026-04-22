package thand

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	thandFunction "github.com/thand-io/agent/internal/workflows/functions/providers/thand"
	taskModel "github.com/thand-io/agent/internal/workflows/tasks/model"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const ThandRevokeTask = "revoke"

type RevokeTask struct {
	Notifiers map[string]thandFunction.NotifierRequest `json:"notifiers"` // Notifier configurations for sending revocation notifications
}

func (t *RevokeTask) HasNotifiers() bool {
	return len(t.Notifiers) > 0
}

// ThandRevokeTask represents a custom task for Thand revocation
func (t *thandTask) executeRevokeTask(
	workflowTask *models.ElevateWorkflowTask,
	taskName string,
	call *taskModel.ThandTask) (any, error) {

	// CRITICAL - YOU CANNOT CALL FOR LOCAL ROLES AND PROVIDERS OR WORKFLOWS
	// These must be either activities or sub-workflows to be executed
	// by a worker that has registered itself as being able to support
	// either a role, provider or workflow.

	log := workflowTask.GetLogger()

	elevateRequest, err := workflowTask.GetContextAsElevationRequest()

	if err != nil {
		return nil, err
	}

	// Parse the revoke task configuration
	var revokeCallTask RevokeTask
	err = common.ConvertInterfaceToInterface(call.With, &revokeCallTask)
	if err != nil {
		log.WithError(err).Error("Failed to parse revoke task configuration")
		// Continue without notifiers if parsing fails
	}

	return t.executeRevocationTask(workflowTask, taskName, call, elevateRequest, &revokeCallTask)
}

// revokeResult holds the result of a revocation operation
type revokeResult struct {
	Identity string
	Output   any
	Error    error
}

// revokeTask represents a revocation task with all necessary context
type revokeTask struct {
	EntryID      string
	ProviderName string
	Identity     string
	DeviceID     string
	RevokeReq    *models.WorkflowRevokeRoleRequest
}

func hydrateAuthorizeResponse(
	workflowTask *models.ElevateWorkflowTask,
	identityID string,
	log *sdkWorkflowsModel.LogBuilder,
) *models.AuthorizeRoleResponse {
	req := workflowTask.GetContextAsMap()
	if req == nil {
		return nil
	}

	authorizationsMap, ok := req["authorizations"]
	if !ok {
		log.WithField("identity", identityID).Debug("No authorizations found in context for revocation")
		return nil
	}

	if objectMap, ok := authorizationsMap.(map[string]any); ok {
		if identityMap, ok := objectMap[identityID].(map[string]any); ok {
			localResponse := models.AuthorizeRoleResponse{}
			if err := common.ConvertMapToInterface(identityMap, &localResponse); err != nil {
				log.WithError(err).WithField("identity", identityID).Warn("Failed to convert authorize response")
				return nil
			}
			return &localResponse
		}
	}

	if authzMap, ok := authorizationsMap.(map[string]*models.AuthorizeRoleResponse); ok {
		if authResp, ok := authzMap[identityID]; ok {
			return authResp
		}
	}

	return nil
}

func (t *thandTask) executeRevocationTask(
	workflowTask *models.ElevateWorkflowTask,
	taskName string,
	call *taskModel.ThandTask,
	elevateRequest *models.ElevateRequestInternal,
	revokeCallTask *RevokeTask,
) (any, error) {

	if !elevateRequest.IsValid() {
		return nil, errors.New("invalid elevate request")
	}

	log := workflowTask.GetLogger()

	revokedAt := time.Now().UTC()

	modelOutput := map[string]any{
		"revoked":    true,
		"revoked_at": revokedAt.Format(time.RFC3339),
	}

	plan, err := t.requireExecutionPlan(workflowTask)
	if err != nil {
		return nil, err
	}

	var revokeTasks []revokeTask
	for _, entry := range plan.Entries {
		if strings.TrimSpace(entry.ProviderName) == "" {
			return nil, fmt.Errorf("execution plan entry is missing provider name")
		}
		if entry.AuthorizeRequest == nil {
			return nil, fmt.Errorf("execution plan entry for provider %q is missing authorize request", entry.ProviderName)
		}

		identityID := identityKeyFromAuthorizeRequest(entry.AuthorizeRequest)
		if identityID == "" {
			return nil, fmt.Errorf("execution plan entry for provider %q is missing identity information", entry.ProviderName)
		}
		authorizeResponse := hydrateAuthorizeResponse(workflowTask, identityID, log)

		revokeReq := models.WorkflowRevokeRoleRequest{
			RevokeRoleRequest: &models.RevokeRoleRequest{
				AuthorizeRoleRequest:  models.CloneAuthorizeRoleRequest(entry.AuthorizeRequest),
				AuthorizeRoleResponse: authorizeResponse,
			},
			AuthorizeRoleResponse: authorizeResponse,
		}

		revokeTasks = append(revokeTasks, revokeTask{
			EntryID:      entry.EntryID,
			ProviderName: entry.ProviderName,
			Identity:     identityID,
			DeviceID:     entry.DeviceID,
			RevokeReq:    &revokeReq,
		})

		log.WithFields(logrus.Fields{
			"user":     identityID,
			"role":     entry.AuthorizeRequest.Role.GetName(),
			"provider": entry.ProviderName,
			"duration": entry.AuthorizeRequest.Duration,
			"tenant":   authorizeRequestTenantID(entry.AuthorizeRequest),
		}).Info("Preparing revocation logic")
	}

	var revokeResults []revokeResult

	revokeResults, err = t.executeRevokeParallel(workflowTask, revokeTasks)

	if err != nil {
		return nil, err
	}

	// Process results
	revocations := make(map[string]any)
	returnedErrors := []error{}

	for _, result := range revokeResults {
		if result.Error != nil {
			log.WithError(result.Error).WithField("identity", result.Identity).Error("Revocation failed")

			foundError := unwrapTemporalError(result.Error)

			returnedErrors = append(returnedErrors, fmt.Errorf(
				"revocation error, failed to revoke: %s - returned with the error: %s", result.Identity, foundError.Error()))
			continue
		}
		revocations[result.Identity] = result.Output
	}

	if len(returnedErrors) > 0 && len(revocations) == 0 {
		return nil, temporal.NewApplicationErrorWithCause(
			fmt.Sprintf("One or more revocations failed: %d errors, %d revocations", len(returnedErrors), len(revocations)),
			"RevocationError",
			errors.Join(returnedErrors...),
		)
	}

	modelOutput["revocations"] = revocations

	// Send notifications if configured
	if revokeCallTask.HasNotifiers() {

		err = t.makeRevocationNotifications(
			workflowTask,
			taskName,
			revokeCallTask,
			elevateRequest,
			revocations,
		)

		if err != nil {
			log.WithError(err).Warn("Failed to send revocation notifications, continuing anyway")
			// Don't fail the revocation if notification fails
		}
	}

	return &modelOutput, nil
}

// runRevokeTask executes a single revocation task and returns its result.
// When a Temporal context is available, it dispatches a child workflow using
// the parent workflow's task queue by default. If the request carries a
// DeviceID, it waits for a fresh live route for that device and retries until
// it can reconcile revocation on the device's task queue instead. Otherwise it
// falls back to local provider execution.
func (t *thandTask) runRevokeTask(
	workflowTask sdkWorkflowsModel.WorkflowTaskSupport,
	task revokeTask,
) revokeResult {

	// Temporal path: dispatch a child workflow to the agent with this provider
	if workflowTask.HasTemporalContext() {
		ctx := workflowTask.GetTemporalContext()

		wfName := models.CreateTemporalProviderWorkflowName(
			task.ProviderName, models.TemporalRevokeRoleWorkflowName)

		taskQueue := workflowTask.GetTaskQueue()
		deviceID := ""
		deviceID = strings.TrimSpace(task.DeviceID)

		baseWorkflowID := models.CreateChildWorkflowIDForEntry(
			workflowTask.GetWorkflowID(),
			"revokeRole",
			task.EntryID,
		)

		retryDelay := deviceRouteRevokeInitialRetry
		attempt := 0
		for {
			if deviceID != "" {
				route, _, err := t.waitForFreshDeviceRoute(
					ctx,
					deviceID,
					deviceRouteRevokeAttemptLimit,
				)
				if err != nil {
					if isDeviceRouteUnavailableError(err) || errors.Is(err, errDeviceRouteWaitExpired) {
						if sleepErr := workflow.Sleep(ctx, retryDelay); sleepErr != nil {
							return revokeResult{
								Identity: task.Identity,
								Error:    sleepErr,
							}
						}
						retryDelay = nextDeviceRouteRetryDelay(retryDelay)
						attempt++
						continue
					}
					return revokeResult{
						Identity: task.Identity,
						Error:    err,
					}
				}
				taskQueue = route.TaskQueue
			}

			childOpts := workflow.ChildWorkflowOptions{
				WorkflowID:               childWorkflowIDForAttempt(baseWorkflowID, attempt),
				TaskQueue:                taskQueue,
				WorkflowExecutionTimeout: deviceRouteRevokeAttemptLimit,
				WorkflowRunTimeout:       deviceRouteRevokeAttemptLimit,
			}
			childOpts = childWorkflowOptionsForTaskQueue(workflowTask.GetTaskQueue(), taskQueue, childOpts)
			childCtx := workflow.WithChildOptions(ctx, childOpts)

			req := models.WorkflowRevokeRoleRequest{
				RevokeRoleRequest:     models.CloneRevokeRoleRequest(task.RevokeReq.RevokeRoleRequest),
				AuthorizeRoleResponse: task.RevokeReq.AuthorizeRoleResponse,
			}

			var resp models.RevokeRoleResponse
			err := workflow.ExecuteChildWorkflow(childCtx, wfName, req).Get(childCtx, &resp)
			if err == nil {
				return revokeResult{
					Identity: task.Identity,
					Output:   &resp,
					Error:    nil,
				}
			}
			if !isTemporalTimeoutError(err) && !isTransientBrokerRevokeError(err) {
				return revokeResult{
					Identity: task.Identity,
					Error:    err,
				}
			}

			if sleepErr := workflow.Sleep(ctx, retryDelay); sleepErr != nil {
				return revokeResult{
					Identity: task.Identity,
					Error:    sleepErr,
				}
			}
			retryDelay = nextDeviceRouteRetryDelay(retryDelay)
			attempt++
		}
	}

	// Non-Temporal fallback: execute locally
	providerCall, err := t.config.GetProviderByName(task.ProviderName)
	if err != nil {
		return revokeResult{
			Identity: task.Identity,
			Error:    fmt.Errorf("failed to get provider: %w", err),
		}
	}
	revokeOut, err := providerCall.RevokeRole(workflowTask.GetContext(), models.CloneRevokeRoleRequest(task.RevokeReq.RevokeRoleRequest))
	return revokeResult{
		Identity: task.Identity,
		Output:   revokeOut,
		Error:    err,
	}
}

// executeRevokeParallel runs all revocation tasks concurrently. It uses Temporal
// goroutines when a Temporal context is present, and standard goroutines otherwise.
func (t *thandTask) executeRevokeParallel(
	workflowTask *models.ElevateWorkflowTask,
	revokeTasks []revokeTask,
) ([]revokeResult, error) {

	results := make([]revokeResult, len(revokeTasks))

	if workflowTask.HasTemporalContext() {
		type indexedResult struct {
			Index  int
			Result revokeResult
		}
		ctx := workflowTask.GetTemporalContext()
		resultCh := workflow.NewChannel(ctx)

		for i, task := range revokeTasks {
			taskIndex, rt := i, task
			workflow.Go(ctx, func(wfCtx workflow.Context) {
				resultCh.Send(wfCtx, indexedResult{
					Index:  taskIndex,
					Result: t.runRevokeTask(newTemporalTaskView(workflowTask, wfCtx), rt),
				})
			})
		}

		for range revokeTasks {
			var r indexedResult
			resultCh.Receive(ctx, &r)
			results[r.Index] = r.Result
		}
	} else {
		var wg sync.WaitGroup
		for i, task := range revokeTasks {
			wg.Add(1)
			go func(index int, rt revokeTask) {
				defer wg.Done()
				results[index] = t.runRevokeTask(workflowTask, rt)
			}(i, task)
		}
		wg.Wait()
	}

	return results, nil
}

// makeRevocationNotifications sends notifications about access revocation
func (t *thandTask) makeRevocationNotifications(
	workflowTask *models.ElevateWorkflowTask,
	taskName string,
	revokeTask *RevokeTask,
	elevateRequest *models.ElevateRequestInternal,
	revocations map[string]any,
) error {

	log := workflowTask.GetLogger()

	log.Info("Preparing revocation notifications")

	// Build notification tasks for each provider
	var notifyTasks []notifyTask
	for providerKey, notifierRequest := range revokeTask.Notifiers {
		// Create a RevokeNotifier for each provider
		revokeNotifier := NewRevokeNotifier(
			t.config,
			workflowTask,
			elevateRequest,
			&notifierRequest,
			providerKey,
			revocations,
		)

		// Get recipients for this notifier
		recipients := revokeNotifier.GetRecipients()

		// Build notification tasks for each recipient
		for _, recipientId := range recipients {

			recipientIdentity := t.resolveIdentity(recipientId)

			if recipientIdentity == nil {
				log.WithField("recipient", recipientId).Warn("Failed to resolve recipient identity for revocation notification, skipping")
				continue
			}

			recipientIdentity.ID = recipientId
			recipientPayload := revokeNotifier.GetPayload(recipientIdentity)

			notifyTasks = append(notifyTasks, notifyTask{
				Recipient: recipientId,
				CallFunc:  revokeNotifier.GetCallFunction(recipientIdentity),
				Payload:   recipientPayload,
				Provider:  revokeNotifier.GetProviderName(),
			})

			log.WithFields(logrus.Fields{
				"recipient":   recipientId,
				"provider":    revokeNotifier.GetProviderName(),
				"providerKey": providerKey,
			}).Debug("Prepared revocation notification task")
		}
	}

	// Execute all notifications in parallel
	var err error
	var notifyResults []notifyResult

	if workflowTask.HasTemporalContext() {
		notifyResults, err = t.executeNotifyTemporalParallel(workflowTask, fmt.Sprintf("%s.notify", taskName), notifyTasks)
	} else {
		notifyResults, err = t.executeNotifyGoParallel(workflowTask, notifyTasks)
	}

	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"taskName": taskName,
		}).Error("Failed to execute revocation notifications")

		return err
	}

	// Process results using shared helper
	if err := processNotificationResults(notifyResults, "Revocation notification"); err != nil {

		log.WithError(err).WithFields(logrus.Fields{
			"taskName": taskName,
		}).Error("Failed to process revocation notification results")

		return err
	}

	return nil
}
