package thand

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
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
)

const ThandAuthorizeTask = "authorize"

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
	workflowTask *models.WorkflowTask,
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
	AuthRequest  *models.AuthorizeRoleRequest
	AuthResponse *models.AuthorizeRoleResponse
	Error        error
}

// authTask represents an authorization task with all necessary context
type authTask struct {
	ProviderName string
	Identity     string
	AuthRequest  models.AuthorizeRoleRequest
	ThandAuthReq thandFunction.ThandAuthorizeRequest
}

// temporalAuthResult represents the result of an authorization operation for temporal communication
type temporalAuthResult struct {
	Index        int
	Identity     string
	AuthRequest  *models.AuthorizeRoleRequest
	AuthResponse *models.AuthorizeRoleResponse
	Err          error
}

// executeAuthorization performs the main authorization workflow
func (t *thandTask) executeAuthorization(
	workflowTask *models.WorkflowTask,
	taskName string,
	call *taskModel.ThandTask,
	elevateRequest *models.ElevateRequestInternal,
) (any, error) {

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

		providerCall, err := t.config.GetProviderByName(providerName)
		if err != nil {
			return nil, fmt.Errorf("failed to get provider: %w", err)
		}

		validateOutput, err := t.validateRoleAndBuildOutput(providerCall, *elevateRequest)
		if err != nil {
			return nil, err
		}

		maps.Copy(modelOutput, validateOutput)

		for _, identityId := range elevateRequest.Identities {

			identityObj := t.resolveIdentity(identityId)

			if identityObj == nil {
				logrus.Warnf("failed to resolve identity: %s", identityId)
				continue
			}

			if identityObj.GetUser() == nil {
				logrus.Warnf("resolved identity has no user: %s", identityId)
				continue
			}

			identityObj.ID = identityId
			authReq := models.AuthorizeRoleRequest{
				RoleRequest: &models.RoleRequest{
					User:     identityObj.GetUser(),
					Role:     elevateRequest.Role,
					Duration: &duration,
				},
			}

			thandAuthReq := thandFunction.ThandAuthorizeRequest{
				AuthorizeRoleRequest: authReq,
				Provider:             providerName,
			}

			authTasks = append(authTasks, authTask{
				ProviderName: providerName,
				Identity:     identityId,
				AuthRequest:  authReq,
				ThandAuthReq: thandAuthReq,
			})

			log.WithFields(models.Fields{
				"user":     authReq.User.GetIdentity(),
				"source":   authReq.User.Source,
				"username": authReq.User.Username,
				"role":     authReq.Role.GetName(),
				"provider": providerName,
				"duration": duration,
			}).Info("Preparing authorization logic")
		}
	}

	var authResults []authResult

	if workflowTask.HasTemporalContext() {
		authResults, err = t.executeTemporalParallel(workflowTask, taskName, call, authTasks)
	} else {
		authResults, err = t.executeGoParallel(workflowTask, authTasks)
	}

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
		requests[req.Identity] = &req.AuthRequest
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

	workflowTask.SetContextKeyValue(models.VarsContextApproved, true)
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

// executeTemporalParallel executes authorization tasks in parallel using Temporal
func (t *thandTask) executeTemporalParallel(
	workflowTask *models.WorkflowTask,
	taskName string,
	call *taskModel.ThandTask,
	authTasks []authTask,
) ([]authResult, error) {

	temporalContext := workflowTask.GetTemporalContext()
	serviceClient := t.config.GetServices()

	ao := workflow.ActivityOptions{
		TaskQueue:           serviceClient.GetTemporal().GetTaskQueue(),
		StartToCloseTimeout: time.Minute * 5,
	}
	aoctx := workflow.WithActivityOptions(temporalContext, ao)

	// Create channel and results slice
	results := make([]authResult, len(authTasks))
	resultCh := workflow.NewChannel(temporalContext)

	// Start all tasks in parallel using workflow.Go
	for i, task := range authTasks {
		taskIndex := i
		authTask := task

		workflow.Go(temporalContext, func(ctx workflow.Context) {
			var authOut models.AuthorizeRoleResponse
			err := workflow.ExecuteActivity(
				aoctx,
				// TODO(hugh): Replace with direct call to AuthorizeActivity
				thandFunction.ThandAuthorizeFunction,
				workflowTask,
				taskName,
				model.CallFunction{
					Call: thandFunction.ThandNotifyFunction,
					With: call.With.AsMap(),
				},
				authTask.ThandAuthReq,
			).Get(ctx, &authOut)

			// Send result through channel
			resultCh.Send(ctx, temporalAuthResult{
				Index:        taskIndex,
				Identity:     authTask.Identity,
				AuthRequest:  &authTask.AuthRequest,
				AuthResponse: &authOut,
				Err:          err,
			})
		})
	}

	// Collect all results
	for range authTasks {
		var result temporalAuthResult
		resultCh.Receive(temporalContext, &result)
		results[result.Index] = authResult{
			Identity:     result.Identity,
			AuthRequest:  result.AuthRequest,
			AuthResponse: result.AuthResponse,
			Error:        result.Err,
		}
	}

	return results, nil
}

// executeGoParallel executes authorization tasks in parallel using Go routines and WaitGroup
func (t *thandTask) executeGoParallel(
	workflowTask *models.WorkflowTask,
	authTasks []authTask,
) ([]authResult, error) {

	results := make([]authResult, len(authTasks))
	var wg sync.WaitGroup

	for i, task := range authTasks {
		wg.Add(1)
		go func(index int, authTask authTask) {
			defer wg.Done()

			providerCall, err := t.config.GetProviderByName(authTask.ProviderName)
			if err != nil {
				results[index] = authResult{
					Identity:     authTask.Identity,
					AuthRequest:  &authTask.AuthRequest,
					AuthResponse: nil,
					Error:        fmt.Errorf("failed to get provider: %w", err),
				}
				return
			}

			authOut, err := providerCall.GetClient().AuthorizeRole(
				workflowTask.GetContext(), &authTask.AuthRequest,
			)

			results[index] = authResult{
				Identity:     authTask.Identity,
				AuthRequest:  &authTask.AuthRequest,
				AuthResponse: authOut,
				Error:        err,
			}
		}(i, task)
	}

	wg.Wait()

	return results, nil
}

// validateRoleAndBuildOutput validates the role and builds the initial model output
func (t *thandTask) validateRoleAndBuildOutput(
	providerCall *models.Provider,
	elevateRequest models.ElevateRequestInternal,
) (map[string]any, error) {
	modelOutput := map[string]any{}

	validateOut, err := models.ValidateRole(providerCall.GetClient(), elevateRequest)
	if err != nil {
		logrus.WithFields(models.Fields{
			"error": err,
			"role":  elevateRequest.Role,
		}).Error("Failed to validate role")
		return nil, err
	}

	if len(validateOut) > 0 {
		maps.Copy(modelOutput, validateOut)
	}

	return modelOutput, nil
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
	workflowTask *models.WorkflowTask,
	revocationTask string,
	revocationAt time.Time,
) error {

	log := workflowTask.GetLogger()

	newTask := workflowTask.Clone().(*models.WorkflowTask)
	newTask.SetEntrypoint(revocationTask)

	serviceClient := t.config.GetServices()

	// If we have a temporal client, we can use that to schedule the revocation
	if serviceClient.HasTemporal() && serviceClient.GetTemporal().HasClient() {

		signalInput := models.TemporalTerminationRequest{
			Reason:      "Revocation scheduled",
			ScheduledAt: &revocationAt,
		}

		if len(revocationTask) > 0 {
			signalInput.EntryPoint = revocationTask
		}

		temporalClient := serviceClient.GetTemporal().GetClient()

		err := temporalClient.SignalWorkflow(
			workflowTask.GetContext(),
			workflowTask.WorkflowID,
			models.TemporalEmptyRunId,
			models.TemporalTerminateSignalName,
			signalInput,
		)

		if err != nil {
			log.WithError(err).Error("Failed to signal workflow for revocation")
			return fmt.Errorf("failed to signal workflow: %w", err)
		}

		log.WithFields(models.Fields{
			"task": newTask.GetTaskName(),
			"url":  t.config.GetResumeCallbackUrl(newTask),
		}).Info("Scheduled revocation via Temporal")

	} else if t.config.GetServices().HasScheduler() {

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
						log.WithFields(models.Fields{
							"status_code": response.StatusCode(),
							"body":        response.Body(),
						}).Error("Revoke endpoint returned non-200 status")
						return
					}

					log.WithFields(models.Fields{
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
	workflowTask *models.WorkflowTask,
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
	for providerKey, notifierRequest := range authorizeTask.Notifiers {
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

			log.WithFields(models.Fields{
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
		log.WithError(err).WithFields(models.Fields{
			"taskName": taskName,
		}).Error("Failed to execute authorization notifications")

		return err
	}

	// Process results using shared helper
	if err := processNotificationResults(notifyResults, "Authorization notification"); err != nil {

		log.WithError(err).WithFields(models.Fields{
			"taskName": taskName,
		}).Error("Failed to process authorization notification results")

		return err
	}

	return nil
}
