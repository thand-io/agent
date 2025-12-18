package thand

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	thandFunction "github.com/thand-io/agent/internal/workflows/functions/providers/thand"
	taskModel "github.com/thand-io/agent/internal/workflows/tasks/model"
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
	workflowTask *models.WorkflowTask,
	taskName string,
	call *taskModel.ThandTask) (any, error) {

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
	ProviderName      string
	Identity          string
	RevokeReq         models.RevokeRoleRequest
	AuthorizeResponse *models.AuthorizeRoleResponse
}

// temporalRevokeResult represents the result of a revocation operation for temporal communication
type temporalRevokeResult struct {
	Index    int
	Identity string
	Output   any
	Err      error
}

func (t *thandTask) executeRevocationTask(
	workflowTask *models.WorkflowTask,
	taskName string,
	call *taskModel.ThandTask,
	elevateRequest *models.ElevateRequestInternal,
	revokeCallTask *RevokeTask,
) (any, error) {

	if !elevateRequest.IsValid() {
		return nil, errors.New("invalid elevate request")
	}

	log := workflowTask.GetLogger()

	duration, err := elevateRequest.AsDuration()
	if err != nil {
		return nil, fmt.Errorf("failed to get duration: %w", err)
	}

	revokedAt := time.Now().UTC()

	modelOutput := map[string]any{
		"revoked":    true,
		"revoked_at": revokedAt.Format(time.RFC3339),
	}

	// Collect all revocation tasks
	var revokeTasks []revokeTask

	for _, providerName := range elevateRequest.Providers {
		for _, identity := range elevateRequest.Identities {
			var authorizeResponse *models.AuthorizeRoleResponse

			// Try to hydrate the authorization response for this identity
			req := workflowTask.GetContextAsMap()
			if req != nil {

				authorizationsMap, ok := req["authorizations"]

				if !ok {
					log.WithField("identity", identity).Debug("No authorizations found in context for revocation")
					continue
				}

				if objectMap, ok := authorizationsMap.(map[string]any); ok {
					if identityMap, ok := objectMap[identity].(map[string]any); ok {
						localResponse := models.AuthorizeRoleResponse{}
						if err := common.ConvertMapToInterface(identityMap, &localResponse); err != nil {
							log.WithError(err).WithField("identity", identity).Warn("Failed to convert authorize response")
						}
						authorizeResponse = &localResponse
					}
				} else if authzMap, ok := authorizationsMap.(map[string]*models.AuthorizeRoleResponse); ok {
					if authResp, ok := authzMap[identity]; ok {
						authorizeResponse = authResp
					}
				}
			}

			identityObj := t.resolveIdentity(identity)

			if identityObj == nil {
				log.WithField("identity", identity).Warn("Failed to resolve identity for revocation, skipping")
				continue
			}

			user := identityObj.GetUser()

			if user == nil {
				log.WithField("identity", identity).Warn("Resolved identity has no user for revocation, skipping")
				continue
			}

			revokeReq := models.RevokeRoleRequest{
				RoleRequest: &models.RoleRequest{
					User:     user,
					Role:     elevateRequest.Role,
					Duration: &duration,
				},
				AuthorizeRoleResponse: authorizeResponse,
			}

			revokeTasks = append(revokeTasks, revokeTask{
				ProviderName:      providerName,
				Identity:          identity,
				RevokeReq:         revokeReq,
				AuthorizeResponse: authorizeResponse,
			})

			log.WithFields(models.Fields{
				"user":     identity,
				"role":     elevateRequest.Role.GetName(),
				"provider": providerName,
				"duration": duration,
			}).Info("Preparing revocation logic")
		}
	}

	var revokeResults []revokeResult

	if workflowTask.HasTemporalContext() {
		revokeResults, err = executeTemporalRevokeParallel(workflowTask, taskName, call, revokeTasks)
	} else {
		revokeResults, err = executeGoRevokeParallel(t.config, workflowTask, revokeTasks)
	}

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

// executeTemporalRevokeParallel executes revocation tasks in parallel using Temporal
func executeTemporalRevokeParallel(
	workflowTask *models.WorkflowTask,
	taskName string,
	call *taskModel.ThandTask,
	revokeTasks []revokeTask,
) ([]revokeResult, error) {

	temporalContext := workflowTask.GetTemporalContext()

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute * 5,
	}
	aoctx := workflow.WithActivityOptions(temporalContext, ao)

	// Create channel and results slice
	results := make([]revokeResult, len(revokeTasks))
	resultCh := workflow.NewChannel(temporalContext)

	// Start all tasks in parallel using workflow.Go
	for i, task := range revokeTasks {
		taskIndex := i
		revokeTask := task

		workflow.Go(temporalContext, func(ctx workflow.Context) {
			var revokeOut any

			thandRevokeReq := thandFunction.ThandRevokeRequest{
				Provider:          revokeTask.ProviderName,
				RevokeRoleRequest: revokeTask.RevokeReq,
			}

			err := workflow.ExecuteActivity(
				aoctx,
				thandFunction.ThandRevokeFunction,
				workflowTask,
				taskName,
				call,
				thandRevokeReq,
			).Get(ctx, &revokeOut)

			// Send result through channel
			resultCh.Send(ctx, temporalRevokeResult{
				Index:    taskIndex,
				Identity: revokeTask.Identity,
				Output:   revokeOut,
				Err:      err,
			})
		})
	}

	// Collect all results
	for range revokeTasks {
		var result temporalRevokeResult
		resultCh.Receive(temporalContext, &result)
		results[result.Index] = revokeResult{
			Identity: result.Identity,
			Output:   result.Output,
			Error:    result.Err,
		}
	}

	return results, nil
}

// executeGoRevokeParallel executes revocation tasks in parallel using Go routines and WaitGroup
func executeGoRevokeParallel(
	config *config.Config,
	workflowTask *models.WorkflowTask,
	revokeTasks []revokeTask,
) ([]revokeResult, error) {

	results := make([]revokeResult, len(revokeTasks))
	var wg sync.WaitGroup

	for i, task := range revokeTasks {
		wg.Add(1)
		go func(index int, revokeTask revokeTask) {
			defer wg.Done()

			providerCall, err := config.GetProviderByName(revokeTask.ProviderName)
			if err != nil {
				results[index] = revokeResult{
					Identity: revokeTask.Identity,
					Output:   nil,
					Error:    fmt.Errorf("failed to get provider: %w", err),
				}
				return
			}

			revokeOut, err := providerCall.GetClient().RevokeRole(
				workflowTask.GetContext(), &revokeTask.RevokeReq,
			)

			results[index] = revokeResult{
				Identity: revokeTask.Identity,
				Output:   revokeOut,
				Error:    err,
			}
		}(i, task)
	}

	wg.Wait()

	return results, nil
}

// makeRevocationNotifications sends notifications about access revocation
func (t *thandTask) makeRevocationNotifications(
	workflowTask *models.WorkflowTask,
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

			log.WithFields(models.Fields{
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
		log.WithError(err).WithFields(models.Fields{
			"taskName": taskName,
		}).Error("Failed to execute revocation notifications")

		return err
	}

	// Process results using shared helper
	if err := processNotificationResults(notifyResults, "Revocation notification"); err != nil {

		log.WithError(err).WithFields(models.Fields{
			"taskName": taskName,
		}).Error("Failed to process revocation notification results")

		return err
	}

	return nil
}
