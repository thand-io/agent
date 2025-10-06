package thand

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/functions"
	"go.temporal.io/sdk/workflow"
)

// AuthorizeFunction implements access authorization based on roles and workflows
type authorizeFunction struct {
	config *config.Config
	*functions.BaseFunction
}

// NewAuthorizeFunction creates a new authorization Function
func NewAuthorizeFunction(config *config.Config) *authorizeFunction {
	return &authorizeFunction{
		config: config,
		BaseFunction: functions.NewBaseFunction(
			"thand.authorize",
			"Authorizes access based on roles and workflows",
			"1.0.0",
		),
	}
}

// GetRequiredParameters returns the required parameters for authorization
func (t *authorizeFunction) GetRequiredParameters() []string {
	return []string{
		"revocation",
	}
}

// GetOptionalParameters returns optional parameters with defaults
func (t *authorizeFunction) GetOptionalParameters() map[string]any {
	return map[string]any{}
}

// ValidateRequest validates the input parameters
func (t *authorizeFunction) ValidateRequest(
	workflowTask *models.WorkflowTask,
	call *model.CallFunction,
	input any,
) error {
	return nil
}

type ThandAuthorizeRequest struct {
	Revocation string `json:"revocation"` // This is the state to request the revocation
}

func (r *ThandAuthorizeRequest) IsValid() bool {
	return len(r.Revocation) > 0
}

// Execute performs the authorization logic
func (t *authorizeFunction) Execute(
	workflowTask *models.WorkflowTask,
	call *model.CallFunction,
	input any,
) (any, error) {
	elevateRequest, authRequest, err := t.validateAndParseRequests(workflowTask, call)
	if err != nil {
		return nil, err
	}

	if t.isWorkflowAlreadyApproved(workflowTask) {
		modelOutput := t.buildBasicModelOutput(elevateRequest)
		return &modelOutput, nil
	}

	return t.executeAuthorization(workflowTask, elevateRequest, authRequest)
}

// validateAndParseRequests validates and parses the incoming requests
func (t *authorizeFunction) validateAndParseRequests(
	workflowTask *models.WorkflowTask,
	call *model.CallFunction,
) (*models.ElevateRequestInternal, *ThandAuthorizeRequest, error) {
	req := workflowTask.GetContextAsMap()
	if req == nil {
		return nil, nil, errors.New("request cannot be nil")
	}

	var elevateRequest models.ElevateRequestInternal
	if err := common.ConvertMapToInterface(req, &elevateRequest); err != nil {
		return nil, nil, fmt.Errorf("failed to convert request: %w", err)
	}

	if !elevateRequest.IsValid() {
		return nil, nil, errors.New("invalid elevate request")
	}

	var authRequest ThandAuthorizeRequest
	if err := common.ConvertMapToInterface(call.With, &authRequest); err != nil {
		return nil, nil, fmt.Errorf("failed to convert auth request: %w", err)
	}

	if !authRequest.IsValid() {
		logrus.Infoln("No revocation state provided. Just handling via the cleanup state")
	}

	return &elevateRequest, &authRequest, nil
}

// isWorkflowAlreadyApproved checks if the workflow has already been approved
func (t *authorizeFunction) isWorkflowAlreadyApproved(workflowTask *models.WorkflowTask) bool {
	if !workflowTask.HasTemporalContext() {
		return false
	}

	attrs := workflow.GetTypedSearchAttributes(workflowTask.GetTemporalContext())
	hasBeenApproved, hasAttr := attrs.GetBool(models.TypedSearchAttributeApproved)

	if hasAttr && hasBeenApproved {
		logrus.Info("Workflow has already been approved")
		return true
	}

	return false
}

// buildBasicModelOutput creates the basic model output with timestamps
func (t *authorizeFunction) buildBasicModelOutput(elevateRequest *models.ElevateRequestInternal) map[string]any {
	duration, _ := elevateRequest.AsDuration()
	authorizedAt := time.Now().UTC()
	revocationDate := authorizedAt.Add(duration)

	return map[string]any{
		"authorized_at": authorizedAt.Format(time.RFC3339),
		"revocation_at": revocationDate.Format(time.RFC3339),
	}
}

// executeAuthorization performs the main authorization workflow
func (t *authorizeFunction) executeAuthorization(
	workflowTask *models.WorkflowTask,
	elevateRequest *models.ElevateRequestInternal,
	authRequest *ThandAuthorizeRequest,
) (any, error) {
	duration, err := elevateRequest.AsDuration()
	if err != nil {
		return nil, fmt.Errorf("failed to get duration: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"user":     elevateRequest.User,
		"role":     elevateRequest.Role,
		"provider": elevateRequest.Provider,
		"duration": duration,
	}).Info("Executing authorization logic")

	providerCall, err := t.config.GetProviderByName(elevateRequest.Provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	modelOutput, err := t.validateRoleAndBuildOutput(providerCall, *elevateRequest)
	if err != nil {
		return nil, err
	}

	authorizedAt := time.Now().UTC()
	revocationDate := authorizedAt.Add(duration)

	maps.Copy(modelOutput, map[string]any{
		"authorized_at": authorizedAt.Format(time.RFC3339),
		"revocation_at": revocationDate.Format(time.RFC3339),
	})

	if err := t.scheduleRevocation(workflowTask, *authRequest, revocationDate); err != nil {
		logrus.WithError(err).Error("Failed to schedule revocation")
		return nil, fmt.Errorf("failed to schedule revocation: %w", err)
	}

	authOut, err := providerCall.GetClient().AuthorizeRole(
		workflowTask.GetContext(), elevateRequest.User, elevateRequest.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to authorize user: %w", err)
	}

	maps.Copy(modelOutput, authOut)

	t.updateTemporalSearchAttributes(workflowTask)

	logrus.WithFields(logrus.Fields{
		"authorized_at": authorizedAt.Format(time.RFC3339),
		"revocation_at": revocationDate.Format(time.RFC3339),
	}).Info("Scheduled revocation")

	return &modelOutput, nil
}

// validateRoleAndBuildOutput validates the role and builds the initial model output
func (t *authorizeFunction) validateRoleAndBuildOutput(
	providerCall *models.Provider,
	elevateRequest models.ElevateRequestInternal,
) (map[string]any, error) {
	modelOutput := map[string]any{}

	validateOut, err := models.ValidateRole(providerCall.GetClient(), elevateRequest)
	if err != nil {
		logrus.WithFields(logrus.Fields{
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

// updateTemporalSearchAttributes updates the temporal search attributes if applicable
func (t *authorizeFunction) updateTemporalSearchAttributes(workflowTask *models.WorkflowTask) {
	if !workflowTask.HasTemporalContext() {
		return
	}

	err := workflow.UpsertTypedSearchAttributes(workflowTask.GetTemporalContext(),
		models.TypedSearchAttributeApproved.ValueSet(true),
	)

	if err != nil {
		logrus.WithError(err).Error("Failed to upsert search attributes")
	}
}

// Add to your function
func (t *authorizeFunction) scheduleRevocation(
	workflowTask *models.WorkflowTask,
	authRequest ThandAuthorizeRequest,
	revocationAt time.Time,
) error {

	revocationTask := authRequest.Revocation

	newTask := workflowTask.Clone().(*models.WorkflowTask)
	newTask.SetEntrypoint(revocationTask)

	serviceClient := t.config.GetServices()

	// If we have a temporal client, we can use that to schedule the revocation
	if serviceClient.HasTemporal() && serviceClient.GetTemporal().HasClient() {

		signalName := models.TemporalResumeSignalName
		var signalInput any

		// If the user has not provided a revocation task, we just terminate
		if len(authRequest.Revocation) == 0 {
			signalName = models.TemporalTerminateSignalName
			signalInput = models.TemporalTerminationRequest{
				Reason:      "No revocation state provided",
				ScheduledAt: revocationAt,
			}
		} else {
			// Otherwise send the new task as the signal input to resume the workflow
			// and set an execution timeout
			// TODO: Fiigure out how to delay the signal until the revocation time
			signalInput = newTask
		}

		temporalClient := serviceClient.GetTemporal().GetClient()

		err := temporalClient.SignalWorkflow(
			workflowTask.GetContext(),
			workflowTask.WorkflowID,
			models.TemporalEmptyRunId,
			signalName,
			signalInput,
		)

		if err != nil {
			logrus.WithError(err).Error("Failed to signal workflow for revocation")
			return fmt.Errorf("failed to signal workflow: %w", err)
		}

		logrus.WithFields(logrus.Fields{
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
						logrus.WithError(err).Error("Failed to call revoke endpoint")
						return
					}

					if response.StatusCode() != http.StatusOK {
						logrus.WithFields(logrus.Fields{
							"status_code": response.StatusCode(),
							"body":        response.Body(),
						}).Error("Revoke endpoint returned non-200 status")
						return
					}

					logrus.WithFields(logrus.Fields{
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

		logrus.Error("No scheduler available to schedule revocation")
		return fmt.Errorf("no scheduler available to schedule revocation")

	}

	return nil

}
