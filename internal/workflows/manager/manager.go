package manager

import (
	"context"
	"fmt"
	"strings"
	"time"

	swctx "github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/config"
	models "github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/functions"
	"github.com/thand-io/agent/internal/workflows/functions/providers/aws"
	"github.com/thand-io/agent/internal/workflows/functions/providers/gcp"
	"github.com/thand-io/agent/internal/workflows/functions/providers/slack"
	"github.com/thand-io/agent/internal/workflows/functions/providers/thand"
	"github.com/thand-io/agent/internal/workflows/runner"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
)

// WorkflowManager manages workflow lifecycle and execution using the official SDK
type WorkflowManager struct {
	config    *config.Config
	functions *functions.FunctionRegistry
}

// NewWorkflowManager creates a new workflow manager
func NewWorkflowManager(cfg *config.Config) *WorkflowManager {

	wm := WorkflowManager{
		config:    cfg,
		functions: functions.NewFunctionRegistry(cfg),
	}

	for _, provider := range []functions.FunctionCollection{

		thand.NewThandCollection(cfg),
		slack.NewSlackCollection(cfg),
		gcp.NewGCPCollection(cfg),
		aws.NewAWSCollection(cfg),
	} {
		provider.RegisterFunctions(wm.functions)
	}

	// If we have temporal configured, then we can register
	// all the activities and workflows

	if cfg.GetServices().HasTemporal() {

		// Register our activities
		err := wm.registerActivities()
		if err != nil {
			logrus.WithError(err).Error("Failed to register activities")
		}

		// Register our workflows
		err = wm.registerWorkflows()
		if err != nil {
			logrus.WithError(err).Error("Failed to register workflows")
		}
	}

	return &wm
}

// CreateWorkflow creates a workflow from a model.Workflow instance
func (m *WorkflowManager) CreateWorkflow(
	ctx context.Context,
	request models.ElevateRequest,
) (*models.WorkflowRequest, error) {
	// Create the workflow request which includes the redirect URL
	// and user session, the actual execution happens in the
	// ResumeWorkflow method which is called after user authentication
	req, err := m.executeWorkflow(ctx, request)

	if err != nil {
		return nil, fmt.Errorf("failed to execute workflow: %w", err)
	}

	return req, nil
}

func (m *WorkflowManager) executeWorkflow(
	ctx context.Context,
	request models.ElevateRequest,
) (*models.WorkflowRequest, error) {

	workflow, err := m.config.GetWorkflowFromElevationRequest(&request)

	if err != nil {
		return nil, fmt.Errorf("failed to create workflow: %w", err)
	}

	defaultAuth := workflow.GetAuthentication()
	workflowDsl := workflow.GetWorkflow()

	if workflowDsl == nil {
		return nil, fmt.Errorf(
			"workflow not found for role '%s' and provider '%s'",
			request.Role.Name,
			request.Provider,
		)
	}

	logrus.WithFields(logrus.Fields{
		"workflow_name": workflowDsl.Document.Name,
		"request":       request,
		"functions":     len(workflowDsl.Use.Functions),
	}).Info("Starting workflow execution")

	authProvider, foundAuthProvider := m.config.GetProviderByName(defaultAuth)

	if foundAuthProvider != nil {
		return nil, fmt.Errorf("authentication provider not found: %s", defaultAuth)
	}

	// Convert input to map
	internalContext := map[string]any{
		"role":     request.Role, // get role
		"provider": request.Provider,
		"reason":   request.Reason,
		"duration": request.Duration,
	}

	workflowTask, err := models.NewWorkflowContext(workflow)

	if err != nil {
		return nil, fmt.Errorf("failed to create workflow context: %w", err)
	}

	workflowTask.SetContext(internalContext)

	existingSession := request.Session

	if existingSession != nil {

		decodedSession, err := existingSession.GetDecodedSession(
			m.config.GetServices().GetEncryption())

		if err != nil {
			return nil, fmt.Errorf("failed to decode session: %w", err)
		}

		logrus.WithFields(logrus.Fields{
			"expiry":      existingSession.Expiry.UTC(),
			"user":        decodedSession.User.Email,
			"has_expired": existingSession.Expiry.UTC().Before(time.Now().UTC()),
		}).Info("Found existing session for user")

		if existingSession.Expiry.UTC().After(time.Now().UTC()) {

			err = authProvider.GetClient().ValidateSession(ctx, decodedSession)

			if err == nil {

				workflowTask.SetUser(decodedSession.User)

				redirectUrl := m.config.GetResumeCallbackUrl(workflowTask)

				logrus.WithField("redirect_url", redirectUrl).Info("Resuming workflow with existing session")

				// Session already ready to go
				return &models.WorkflowRequest{
					Task: workflowTask,
					Url:  redirectUrl,
				}, nil
			}

		} else {
			// The session has expired lets try and revalidate it
			// Redirect the user to the auth provider to re-authenticate

			logrus.Info("Existing session has expired, revalidating...")
		}
	}

	sessionResponse, err := authProvider.GetClient().AuthorizeSession(ctx, &models.AuthorizeUser{
		State:       workflowTask.GetEncodedTask(m.config.GetServices().GetEncryption()),
		RedirectUri: m.config.GetAuthCallbackUrl(workflow.Authentication),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to authorize user: %w", err)
	}

	logrus.WithField("redirect_url", sessionResponse.Url).Info("Redirecting user to authentication provider")

	return &models.WorkflowRequest{
		Task: workflowTask,
		Url:  sessionResponse.Url,
	}, nil

}

// ResumeWorkflow resumes workflow execution from client-provided state
func (m *WorkflowManager) ResumeWorkflow(
	result *models.WorkflowTask,
) (*models.WorkflowTask, error) {

	ctx := result.GetContext()

	// Check if workfow has already been registered on temporal
	serviceClient := m.config.GetServices()

	// If we have temporal configured, then we can resume the workflow
	// from the workflow ID or create one if the workflow ID does not exist
	if serviceClient.HasTemporal() {

		// Check the workflow task
		err := m.Hydrate(result)

		if err != nil {
			return nil, fmt.Errorf("failed to hydrate workflow task: %w", err)
		}

		temporalService := serviceClient.GetTemporal()
		temporalClient := temporalService.GetClient()

		_, err = temporalClient.DescribeWorkflow(ctx, result.WorkflowID, models.TemporalEmptyRunId)

		if err != nil {

			// Not found, so start a new workflow execution
			err := m.createTemporalWorkflow(result)

			if err != nil {
				return nil, fmt.Errorf("failed to create temporal workflow: %w", err)
			}

		}

		// Lets signal the workflow to continue
		err = temporalClient.SignalWorkflow(
			ctx, result.WorkflowID, models.TemporalEmptyRunId, models.TemporalResumeSignalName, result)

		if err != nil {
			return nil, fmt.Errorf("failed to signal workflow: %w", err)
		}

		return result, nil

	} else {

		return m.ResumeWorkflowTask(result)
	}

}

// ResumeWorkflowTask resumes a workflow task using the internal runner
// This maybe called as part of a temporal workflow or directly
func (m *WorkflowManager) ResumeWorkflowTask(
	result *models.WorkflowTask,
) (*models.WorkflowTask, error) {

	// Check the workflow task
	err := m.Hydrate(result)

	if err != nil {
		return nil, fmt.Errorf("failed to hydrate workflow task: %w", err)
	}

	// Set status to pending if not already set
	if !result.HasStatus() {
		result.SetStatus(swctx.PendingStatus)
	}

	logrus.WithFields(logrus.Fields{
		"workflow_id": result.WorkflowID,
	}).Info("Resuming workflow")

	// Create runner
	runner, err := m.createCustomRunner(result)

	if err != nil {
		return nil, fmt.Errorf("failed to create runner: %w", err)
	}

	// Resume from saved state
	_, err = runner.Run(result.GetInput())

	if err != nil {
		return nil, fmt.Errorf("failed to resume workflow: %w", err)
	}

	// Merge the output with the input based on any handlers

	return result, err
}

// createCustomRunner creates a workflow runner that can handle custom functions
func (m *WorkflowManager) createCustomRunner(workflow *models.WorkflowTask) (*runner.ResumableWorkflowRunner, error) {
	// Create our custom resumable runner instead of the default runner
	return runner.NewResumableRunner(m.config, m.functions, workflow), nil
}

// RegisterCustomFunction allows external code to register additional functions
func (m *WorkflowManager) RegisterCustomFunction(handler functions.Function) {
	m.functions.RegisterFunction(handler)
	logrus.WithField("function", handler.GetName()).Info("Registered external custom function")
}

// GetRegisteredFunctions returns all currently registered functions
func (m *WorkflowManager) GetRegisteredFunctions() []string {
	return m.functions.GetRegisteredFunctions()
}

func (m *WorkflowManager) createTemporalWorkflow(workflowTask *models.WorkflowTask) error {
	// Not found, so start a new workflow execution

	logrus.WithFields(logrus.Fields{
		"workflow_id": workflowTask.WorkflowID,
	}).Info("Starting new workflow execution")

	serviceClient := m.config.GetServices()

	temporalService := serviceClient.GetTemporal()
	temporalClient := temporalService.GetClient()

	workflowContext, err := workflowTask.GetContextAsElevationRequest()

	if err != nil {
		return fmt.Errorf("failed to get workflow context: %w", err)
	}

	userInfo := ""
	roleInfo := ""

	if workflowContext == nil {
		return fmt.Errorf("workflow context is nil")
	}

	if workflowContext.User != nil {
		userInfo = workflowContext.User.Email
	}

	if workflowContext.Role != nil {
		roleInfo = workflowContext.Role.Name
	}

	ctx := workflowTask.GetContext()

	// Create new workflow
	we, err := temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowTask.WorkflowID,
		TaskQueue: temporalService.GetTaskQueue(),
		TypedSearchAttributes: temporal.NewSearchAttributes(
			models.TypedSearchAttributeUser.ValueSet(userInfo),
			models.TypedSearchAttributeStatus.ValueSet(strings.ToUpper(string(swctx.PendingStatus))),
			models.TypedSearchAttributeApproved.ValueSet(false),
			models.TypedSearchAttributeRole.ValueSet(roleInfo),
		),
	}, models.TemporalExecuteElevationWorkflowName, workflowTask)

	if err != nil {
		return fmt.Errorf("failed to start workflow: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"workflow_id": we.GetID(),
		"run_id":      we.GetRunID(),
	}).Info("Started new workflow execution")

	return nil
}
