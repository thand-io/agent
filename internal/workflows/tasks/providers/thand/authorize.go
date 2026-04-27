package thand

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/internal/common"
	thandFunction "github.com/thand-io/agent/internal/workflows/functions/providers/thand"
	taskModel "github.com/thand-io/agent/internal/workflows/tasks/model"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	runner "github.com/thand-io/agent/sdk/workflows/runner"
)

const ThandAuthorizeTask = "authorize"

// temporalTaskView wraps a WorkflowTaskSupport and pins it to a coroutine-local
// Temporal context. This prevents the shared workflowTask from being mutated when
// multiple workflow.Go coroutines run concurrently and each needs its own ctx.
type temporalTaskView struct {
	sdkWorkflowsModel.WorkflowTaskSupport
	temporalCtx workflow.Context
}

func newTemporalTaskView(base sdkWorkflowsModel.WorkflowTaskSupport, ctx workflow.Context) *temporalTaskView {
	return &temporalTaskView{WorkflowTaskSupport: base, temporalCtx: ctx}
}

func (v *temporalTaskView) HasTemporalContext() bool {
	return v.temporalCtx != nil
}

func (v *temporalTaskView) GetTemporalContext() workflow.Context {
	return v.temporalCtx
}

func (v *temporalTaskView) WithTemporalContext(ctx workflow.Context) sdkWorkflowsModel.WorkflowTaskSupport {
	return &temporalTaskView{WorkflowTaskSupport: v.WorkflowTaskSupport, temporalCtx: ctx}
}

type AuthorizeTask struct {
	Revocation string                                   `json:"revocation"` // This is the state to request the revocation
	Notifiers  map[string]thandFunction.NotifierRequest `json:"notifiers"`  // Notifier configurations for sending authorization notifications
}

func (t *AuthorizeTask) HasRevocation() bool {
	return len(t.Revocation) > 0
}

func (t *AuthorizeTask) HasNotifiers() bool {
	return len(t.Notifiers) > 0
}

func (t *thandTask) executeAuthorizeTask(
	workflowTask *models.ElevateWorkflowTask,
	taskName string,
	call *taskModel.ThandTask) (any, error) {

	elevateRequest, err := workflowTask.GetContextAsElevationRequest()

	if err != nil {
		return nil, err
	}

	isApproved := workflowTask.IsApproved()

	if isApproved != nil && *isApproved {
		modelOutput := t.buildBasicModelOutput(elevateRequest)
		return &modelOutput, nil
	}

	return t.executeAuthorization(workflowTask, taskName, call, elevateRequest)
}

// buildBasicModelOutput creates the basic model output with timestamps
func (t *thandTask) buildBasicModelOutput(elevateRequest *models.ElevateRequestInternal) map[string]any {
	duration, _ := elevateRequest.AsDuration()
	authorizedAt := time.Now().UTC()
	revocationDate := authorizedAt.Add(duration)

	return map[string]any{
		"authorized_at": authorizedAt.Format(time.RFC3339),
		"revocation_at": revocationDate.Format(time.RFC3339),
	}
}

// authResult holds the result of an authorization operation
type authResult struct {
	Identity     string
	AuthRequest  *models.WorkflowRoleRequest
	AuthResponse *models.AuthorizeRoleResponse
	Error        error
}

// authTask represents an authorization task with all necessary context
type authTask struct {
	ProviderName string
	Identity     string
	AuthRequest  *models.WorkflowRoleRequest
}

// executeAuthorization performs the main authorization workflow
func (t *thandTask) executeAuthorization(
	workflowTask *models.ElevateWorkflowTask,
	taskName string,
	call *taskModel.ThandTask,
	elevateRequest *models.ElevateRequestInternal,
) (any, error) {

	// CRITICAL - YOU CANNOT CALL FOR LOCAL ROLES AND PROVIDERS OR WORKFLOWS
	// These must be either activities or sub-workflows to be executed
	// by a worker that has registered itself as being able to support
	// either a role, provider or workflow.

	log := workflowTask.GetLogger()

	// Send notification to the requester if notifier is configured
	var authorizeCallTask AuthorizeTask
	err := common.ConvertInterfaceToInterface(call.With, &authorizeCallTask)

	if err != nil {
		log.WithError(err).Error("Failed to convert call.With to authorizeCallTask")
		return nil, err
	}

	duration, err := elevateRequest.AsDuration()

	if err != nil {
		return nil, fmt.Errorf("failed to get duration: %w", err)
	}

	authorizedAt := time.Now().UTC()
	revocationDate := authorizedAt.Add(duration)

	modelOutput := map[string]any{
		"authorized_at": authorizedAt.Format(time.RFC3339),
		"revocation_at": revocationDate.Format(time.RFC3339),
	}

	// Collect all authorization tasks
	var authTasks []authTask

	if len(elevateRequest.Providers) == 0 {
		return nil, fmt.Errorf("no providers specified for authorization")
	}

	if len(elevateRequest.Identities) == 0 {
		return nil, fmt.Errorf("no identities specified for authorization")
	}

	for _, providerName := range elevateRequest.Providers {

		for _, identityId := range elevateRequest.Identities {

			// Check if we have tenants specified in our request. If so, we need
			// to create an authorization task for each identity and tenant combination
			// if there are no tenants, we just create one task per identity
			if len(elevateRequest.Tenants) == 0 {
				elevateRequest.Tenants = []string{""} // Use empty string to indicate no tenant
			}

			for _, tenantId := range elevateRequest.Tenants {

				authReq := models.WorkflowRoleRequest{
					WorkflowID: workflowTask.GetWorkflowID(),
					Identity:   identityId,
					Role:       elevateRequest.Role,
					Duration:   &duration,
					Tenant:     tenantId,
				}

				authTasks = append(authTasks, authTask{
					ProviderName: providerName,
					Identity:     identityId,
					AuthRequest:  &authReq,
				})

				log.WithFields(logrus.Fields{
					"identity": identityId,
					"role":     authReq.Role.GetName(),
					"provider": providerName,
					"duration": duration,
					"tenant":   tenantId,
				}).Info("Preparing authorization logic")

			}
		}
	}

	var authResults []authResult

	authResults, err = t.executeParallel(workflowTask, authTasks)

	if err != nil {

		log.WithError(err).Error("Failed to execute authorization tasks")
		return nil, err

	}

	// Process results
	requests := make(map[string]*models.AuthorizeRoleRequest)
	authorizations := make(map[string]*models.AuthorizeRoleResponse)
	returnedErrors := []error{}

	if len(authResults) == 0 {
		return nil, fmt.Errorf("no authorization results returned")
	}

	for _, result := range authResults {
		if result.Error != nil {
			log.WithError(result.Error).WithField("identity", result.Identity).Error("Authorization failed")

			foundError := unwrapTemporalError(result.Error)

			returnedErrors = append(returnedErrors, fmt.Errorf(
				"authorization error, failed to authorize: %s - returned with the error: %s", result.Identity, foundError.Error()))
			continue
		}
		authorizations[result.Identity] = result.AuthResponse
	}

	for _, req := range authTasks {
		var dur *time.Duration
		if req.AuthRequest.Duration != nil {
			d := *req.AuthRequest.Duration
			dur = &d
		}
		// Create a non-composite role from the workflow's base role definition
		// The role will be resolved properly by the provider if needed
		requests[req.Identity] = &models.AuthorizeRoleRequest{
			Identity: &models.Identity{ID: req.AuthRequest.Identity},
			Tenant:   &models.ProviderTenant{ID: req.AuthRequest.Tenant},
			Role: &models.CompositeRole{
				Role:      *req.AuthRequest.Role,
				Composite: false, // Explicitly set - this is a base role from workflow
			},
			Duration: dur,
		}
	}

	if len(returnedErrors) > 0 && len(authorizations) == 0 {

		return nil, temporal.NewApplicationErrorWithCause(
			fmt.Sprintf("One or more authorizations failed: %d errors, %d authorizations", len(returnedErrors), len(authorizations)),
			"AuthorizationError",
			errors.Join(returnedErrors...),
		)
	}

	// Schedule revocation if revocation state provided
	if err := t.scheduleRevocation(workflowTask, authorizeCallTask.Revocation, revocationDate); err != nil {
		log.WithError(err).Error("Failed to schedule revocation")
		return nil, fmt.Errorf("failed to schedule revocation: %w", err)
	}

	workflowTask.SetContextKeyValue(sdkConstants.VarsContextApproved, true)
	workflowTask.SetContextKeyValue("authorizations", authorizations)

	if authorizeCallTask.HasNotifiers() {

		err = t.makeAuthorizationNotifications(
			workflowTask,
			taskName,
			&authorizeCallTask,
			elevateRequest,
			requests,
			authorizations,
		)

		if err != nil {
			log.WithError(err).Warn("Failed to send authorization notifications, continuing anyway")
			// Don't fail the authorization if notification fails
		}
	}

	return modelOutput, nil
}

// When a Temporal context is available, it dispatches a child workflow using
// the parent workflow's task queue (typically the agent identity), assuming
// the provider is registered on that worker. Otherwise it falls back to local
// provider execution.
func (t *thandTask) runAuthTask(
	workflowTask sdkWorkflowsModel.WorkflowTaskSupport,
	task authTask,
) authResult {

	// Temporal path: dispatch a child workflow to the agent with this provider
	if workflowTask.HasTemporalContext() {
		ctx := workflowTask.GetTemporalContext()

		wfName := models.CreateTemporalProviderWorkflowName(
			task.ProviderName, models.TemporalAuthorizeRoleWorkflowName)

		// Create unique child workflow ID using hash of composite identifier
		// (provider + role + identity + tenant) to ensure uniqueness across
		// different identities/tenants requesting the same role
		childOpts := workflow.ChildWorkflowOptions{
			WorkflowID: models.CreateChildWorkflowID(
				workflowTask.GetWorkflowID(),
				"authorizeRole",
				task.ProviderName,
				task.AuthRequest,
			),
			TaskQueue: workflowTask.GetTaskQueue(),
		}
		ctx = workflow.WithChildOptions(ctx, childOpts)

		req := task.AuthRequest

		var resp models.AuthorizeRoleResponse
		err := workflow.ExecuteChildWorkflow(ctx, wfName, req).Get(ctx, &resp)
		if err != nil {
			return authResult{
				Identity:    task.Identity,
				AuthRequest: task.AuthRequest,
				Error:       err,
			}
		}
		return authResult{
			Identity:     task.Identity,
			AuthRequest:  task.AuthRequest,
			AuthResponse: &resp,
			Error:        nil,
		}
	}

	// Non-Temporal fallback: execute locally
	providerCall, err := t.config.GetProviderByName(task.ProviderName)
	if err != nil {
		return authResult{
			Identity:    task.Identity,
			AuthRequest: task.AuthRequest,
			Error:       fmt.Errorf("failed to get provider: %w", err),
		}
	}
	authRoleReq, err := models.CreateAuthorizeRoleRequest(
		t.config,
		providerCall,
		task.AuthRequest,
	)
	if err != nil {
		return authResult{
			Identity:    task.Identity,
			AuthRequest: task.AuthRequest,
			Error:       fmt.Errorf("failed to create authorize role request: %w", err),
		}
	}
	authOut, err := providerCall.AuthorizeRole(workflowTask.GetContext(), authRoleReq)
	return authResult{
		Identity:     task.Identity,
		AuthRequest:  task.AuthRequest,
		AuthResponse: authOut,
		Error:        err,
	}
}

// executeParallel runs all authorization tasks concurrently. It uses Temporal
// goroutines when a Temporal context is present, and standard goroutines otherwise.
func (t *thandTask) executeParallel(
	workflowTask *models.ElevateWorkflowTask,
	authTasks []authTask,
) ([]authResult, error) {

	results := make([]authResult, len(authTasks))

	if workflowTask.HasTemporalContext() {
		type indexedResult struct {
			Index  int
			Result authResult
		}
		ctx := workflowTask.GetTemporalContext()
		resultCh := workflow.NewChannel(ctx)

		for i, task := range authTasks {
			taskIndex, at := i, task
			workflow.Go(ctx, func(wfCtx workflow.Context) {
				resultCh.Send(wfCtx, indexedResult{
					Index:  taskIndex,
					Result: t.runAuthTask(newTemporalTaskView(workflowTask, wfCtx), at),
				})
			})
		}

		for range authTasks {
			var r indexedResult
			resultCh.Receive(ctx, &r)
			results[r.Index] = r.Result
		}
	} else {
		var wg sync.WaitGroup
		for i, task := range authTasks {
			wg.Add(1)
			go func(index int, at authTask) {
				defer wg.Done()
				results[index] = t.runAuthTask(workflowTask, at)
			}(i, task)
		}
		wg.Wait()
	}

	return results, nil
}

func (t *thandTask) GetExport() *model.Export {
	return &model.Export{
		As: model.NewObjectOrRuntimeExpr(
			model.RuntimeExpression{
				Value: "${ $context + . }",
			},
		),
	}
}

// Add to your function
func (t *thandTask) scheduleRevocation(
	workflowTask *models.ElevateWorkflowTask,
	revocationTask string,
	revocationAt time.Time,
) error {

	log := workflowTask.GetLogger()

	// If we have a temporal context, use it to schedule the revocation
	// via the local activity to signal the workflow.
	if workflowTask.HasTemporalContext() {

		terminationRequest := models.TemporalTerminationRequest{
			Reason:      "Revocation scheduled",
			ScheduledAt: &revocationAt,
		}

		if len(revocationTask) > 0 {
			terminationRequest.EntryPoint = revocationTask
		}

		ctx := workflowTask.GetTemporalContext()

		// WorkflowInfo
		workflowInfo := workflow.GetInfo(ctx)

		ao := workflow.LocalActivityOptions{
			StartToCloseTimeout: 10 * time.Minute,
			RetryPolicy:         runner.DefaultRetryPolicy,
		}

		actx := workflow.WithLocalActivityOptions(ctx, ao)

		fut := workflow.ExecuteLocalActivity(
			actx,
			sdkConstants.TemporalSignalWorkflowActivityName,
			workflowInfo.WorkflowExecution.ID,
			workflowInfo.WorkflowExecution.RunID,
			sdkWorkflowsModel.TemporalTerminateSignalName,
			terminationRequest,
		)

		err := fut.Get(ctx, nil)

		if err != nil {
			return fmt.Errorf("failed to signal workflow: %w", err)
		}

		log.WithFields(logrus.Fields{
			"task": workflowTask.GetTaskName(),
			"url":  t.config.GetResumeCallbackUrl(workflowTask),
		}).Info("Scheduled revocation via Temporal")

	} else if t.config.GetServices().HasScheduler() {

		newWorkflowTask := workflowTask.Clone().(*sdkWorkflowsModel.WorkflowTask)
		newWorkflowTask.SetEntrypoint(revocationTask)

		newTask := models.NewElevateWorkflowTask(newWorkflowTask)

		err := t.config.GetServices().GetScheduler().AddJob(
			models.NewAtJob(
				revocationAt,
				func() {

					// Make call to revoke the user
					callingUrl := t.config.GetResumeCallbackUrl(newTask)

					logrus.WithFields(logrus.Fields{
						"task": newTask.GetTaskName(),
						"url":  callingUrl,
					}).Info("Executing scheduled revocation")

					response, err := common.InvokeHttpRequest(&model.HTTPArguments{
						Method: http.MethodGet,
						Endpoint: &model.Endpoint{
							URITemplate: &model.LiteralUri{
								Value: callingUrl,
							},
						},
					})

					if err != nil {
						log.WithError(err).Error("Failed to call revoke endpoint")
						return
					}

					if response.StatusCode() != http.StatusOK {
						log.WithFields(logrus.Fields{
							"status_code": response.StatusCode(),
						}).Error("Revoke endpoint returned non-200 status")
						return
					}

					log.WithFields(logrus.Fields{
						"revocation_task": newTask.GetTaskName(),
						"workflow":        workflowTask,
					}).Info("Scheduled revocation")

				},
			),
		)

		if err != nil {
			return fmt.Errorf("failed to schedule revocation: %w", err)
		}

	} else {

		log.Error("No scheduler available to schedule revocation")
		return fmt.Errorf("no scheduler available to schedule revocation")

	}

	return nil

}

func (t *thandTask) makeAuthorizationNotifications(
	workflowTask *models.ElevateWorkflowTask,
	taskName string,
	authorizeTask *AuthorizeTask,
	elevateRequest *models.ElevateRequestInternal,
	authRequests map[string]*models.AuthorizeRoleRequest,
	authorizations map[string]*models.AuthorizeRoleResponse,
) error {

	log := workflowTask.GetLogger()

	log.Info("Preparing authorization notifications")

	// Build notification tasks for each provider
	var notifyTasks []notifyTask
	for _, providerKey := range slices.Sorted(maps.Keys(authorizeTask.Notifiers)) {
		notifierRequest := authorizeTask.Notifiers[providerKey]
		// Create an AuthorizerNotifier for each provider
		authorizeNotifier := NewAuthorizerNotifier(
			t.config,
			workflowTask,
			elevateRequest,
			&notifierRequest,
			providerKey,
			authRequests,
			authorizations,
		)

		// Get recipients for this notifier
		recipients := authorizeNotifier.GetRecipients()

		// Build notification tasks for each recipient
		for _, recipientId := range recipients {

			recipientIdentity := t.resolveIdentity(recipientId)

			if recipientIdentity == nil {
				log.WithField("recipient", recipientId).
					Error("Failed to resolve recipient identity")
				continue
			}

			recipientIdentity.ID = recipientId
			recipientPayload := authorizeNotifier.GetPayload(recipientIdentity)

			notifyTasks = append(notifyTasks, notifyTask{
				Recipient: recipientId,
				CallFunc:  authorizeNotifier.GetCallFunction(recipientIdentity),
				Payload:   recipientPayload,
				Provider:  authorizeNotifier.GetProviderName(),
			})

			log.WithFields(logrus.Fields{
				"recipient":   recipientId,
				"provider":    authorizeNotifier.GetProviderName(),
				"providerKey": providerKey,
			}).Debug("Prepared authorization notification task")
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
		}).Error("Failed to execute authorization notifications")

		return err
	}

	// Process results using shared helper
	if err := processNotificationResults(notifyResults, "Authorization notification"); err != nil {

		log.WithError(err).WithFields(logrus.Fields{
			"taskName": taskName,
		}).Error("Failed to process authorization notification results")

		return err
	}

	return nil
}
