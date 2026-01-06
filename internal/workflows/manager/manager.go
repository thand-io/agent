package manager

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/config"
	models "github.com/thand-io/agent/internal/models"
	providerAws "github.com/thand-io/agent/internal/workflows/functions/providers/aws"
	providerGcp "github.com/thand-io/agent/internal/workflows/functions/providers/gcp"
	providerSlack "github.com/thand-io/agent/internal/workflows/functions/providers/slack"
	providerThand "github.com/thand-io/agent/internal/workflows/functions/providers/thand"
	taskThand "github.com/thand-io/agent/internal/workflows/tasks/providers/thand"
	sdkWorkflowsConfig "github.com/thand-io/agent/sdk/workflows/config"
	"github.com/thand-io/agent/sdk/workflows/functions"
	workflowSdk "github.com/thand-io/agent/sdk/workflows/manager"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"github.com/thand-io/agent/sdk/workflows/tasks"
)

// WorkflowManager manages workflow lifecycle and execution using the official SDK
type ThandWorkflowManager struct {
	config          *config.Config
	workflowManager *workflowSdk.WorkflowManager
}

// NewWorkflowManager creates a new workflow manager
func NewThandWorkflowManager(cfg *config.Config) (*ThandWorkflowManager, error) {

	workflowConfig := NewThandWorkflowConfig(cfg)

	// Register all custom tasks
	for _, task := range []tasks.TaskCollection{
		taskThand.NewThandCollection(cfg),
	} {
		task.RegisterTasks(workflowConfig.GetTaskRegistry())
	}

	// Register all built-in function providers
	for _, provider := range []functions.FunctionCollection{
		providerThand.NewThandCollection(cfg),
		providerSlack.NewSlackCollection(cfg),
		providerGcp.NewGCPCollection(cfg),
		providerAws.NewAWSCollection(cfg),
	} {
		provider.RegisterFunctions(workflowConfig.GetFunctionRegistry())
	}

	// Create an instance of the SDK workflow manager
	workflowManager, err := workflowSdk.NewWorkflowManager(workflowConfig)

	if err != nil {
		return nil, fmt.Errorf("failed to create workflow manager: %w", err)
	}

	thandWorkflowManager := &ThandWorkflowManager{
		config:          cfg,
		workflowManager: workflowManager,
	}

	// Register our custom temporal workflow
	err = thandWorkflowManager.registerThandWorkflows()

	if err != nil {
		logrus.WithError(err).Error("Failed to register thand workflows")
		return nil, fmt.Errorf("failed to register thand workflows: %w", err)
	}

	return thandWorkflowManager, nil
}

func (m *ThandWorkflowManager) GetWorkflowManager() *workflowSdk.WorkflowManager {
	return m.workflowManager
}

func (m *ThandWorkflowManager) GetThandConfig() *config.Config {
	return m.config
}

func (m *ThandWorkflowManager) GetConfig() sdkWorkflowsConfig.Config {
	return m.workflowManager.GetConfig()
}

func (m *ThandWorkflowManager) HasTemporal() bool {
	return m.workflowManager.HasTemporal()
}

func (m *ThandWorkflowManager) GetTemporal() models.TemporalImpl {
	return m.workflowManager.GetTemporal()
}

func (m *ThandWorkflowManager) GetTaskRegistry() *tasks.TaskRegistry {
	return m.workflowManager.GetTaskRegistry()
}

func (m *ThandWorkflowManager) GetFunctionRegistry() *functions.FunctionRegistry {
	return m.workflowManager.GetFunctionRegistry()
}

// CreateElevationWorkflow creates a workflow from a model.Workflow instance
func (m *ThandWorkflowManager) CreateElevationWorkflow(
	ctx context.Context,
	request models.ElevateRequest,
) (*models.WorkflowRequest, error) {
	// Create the workflow request which includes the redirect URL
	// and user session, the actual execution happens in the
	// ResumeWorkflow method which is called after user authentication
	req, err := m.executeElevationWorkflow(ctx, request)

	if err != nil {
		return nil, fmt.Errorf("failed to execute workflow: %w", err)
	}

	return req, nil
}

func (m *ThandWorkflowManager) executeElevationWorkflow(
	ctx context.Context,
	request models.ElevateRequest,
) (*models.WorkflowRequest, error) {

	workflow, err := m.config.GetWorkflowFromElevationRequest(&request)

	if err != nil {
		return nil, fmt.Errorf("failed to create workflow: %w", err)
	}

	if len(request.Duration) == 0 {
		return nil, fmt.Errorf("no duration specified in request")
	}

	if len(request.Providers) == 0 {
		return nil, fmt.Errorf("no providers specified in request")
	}

	if len(request.Identities) == 0 {
		return nil, fmt.Errorf("no identities specified in request")
	}

	if len(request.Authenticator) == 0 {

		// Get the user information from the context
		// and use the first auth provider from the role
		if request.Session != nil {
			decodedSession, err := request.Session.GetDecodedSession(
				m.config.GetServices().GetEncryption())

			if err != nil {
				return nil, fmt.Errorf("failed to decode session from request: %w", err)
			}

			providerAuth := decodedSession.Provider

			// Check that the session is valid for one of the role's authenticators
			if request.Role != nil && len(request.Role.Authenticators) > 0 {
				if !slices.Contains(request.Role.Authenticators, providerAuth) {
					return nil, fmt.Errorf("authenticator %s is not allowed for the specified role", providerAuth)
				}
			}

			request.Authenticator = providerAuth

		}
	}

	if len(request.Authenticator) == 0 {
		return nil, fmt.Errorf("no authenticator specified or found in session")
	}

	workflowDsl := workflow.GetWorkflow()

	if workflowDsl == nil {
		return nil, fmt.Errorf(
			"workflow not found for role '%s' and provider '%s'",
			request.Role.Name,
			request.Providers,
		)
	}

	logrus.WithFields(logrus.Fields{
		"workflow_name": workflowDsl.Document.Name,
		"request":       request,
	}).Info("Starting workflow execution")

	authProvider, foundAuthProvider := m.config.GetProviderByName(request.Authenticator)

	if foundAuthProvider != nil {
		return nil, fmt.Errorf("authentication provider not found: %s", request.Authenticator)
	}

	// Convert input to map
	internalContext := request.AsMap()

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

			err = authProvider.GetClient().ValidateSession(ctx, decodedSession.Session)

			if err == nil {

				workflowTask.SetUser(decodedSession.User)

				// Now that we have a user we need to evaluate our composite role
				newRole, err := m.config.GetCompositeRole(&models.Identity{
					ID:    decodedSession.User.GetIdentity(),
					Label: decodedSession.User.GetName(),
					User:  decodedSession.User,
				}, workflowTask.GetRole())

				if err != nil {
					return nil, fmt.Errorf("failed to evaluate composite role for elevation request: %w", err)
				}

				workflowTask.SetRole(newRole)

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
		RedirectUri: m.config.GetAuthCallbackUrl(request.Authenticator),
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

func (m *ThandWorkflowManager) ResumeWorkflow(
	workflowTask *models.ThandWorkflowTask,
) (sdkWorkflowsModel.WorkflowTask, error) {

	err := m.Hydrate(workflowTask)

	if err != nil {
		return nil, fmt.Errorf("failed to hydrate resumed workflow task: %w", err)
	}

	result, err := m.workflowManager.ResumeWorkflow(workflowTask)
	return result, err
}

func (m *ThandWorkflowManager) ResumeWorkflowTask(
	workflowTask *models.ThandWorkflowTask,
) (sdkWorkflowsModel.WorkflowTask, error) {

	err := m.Hydrate(workflowTask)

	if err != nil {
		return nil, fmt.Errorf("failed to hydrate resumed workflow task: %w", err)
	}

	result, err := m.workflowManager.ResumeWorkflowTask(workflowTask)
	return result, err
}

// updateTemporalSearchAttributes updates the workflow search attributes
/*
func (wr *ThandWorkflowManager) updateTemporalSearchAttributes(
	currentTask *model.TaskItem,
	status swctx.StatusPhase,
) error {

	if !wr.workflowTask.HasTemporalContext() {
		return nil
	}

	workflowTask := wr.GetWorkflowTask()
	log := workflowTask.GetLogger()

	ctx := workflowTask.GetTemporalContext()

	updates := []temporal.SearchAttributeUpdate{
		models.TypedSearchAttributeStatus.ValueSet(string(status)),
	}

	isApproved := workflowTask.IsApproved()

	if isApproved != nil {
		updates = append(updates,
			models.TypedSearchAttributeApproved.ValueSet(*isApproved),
		)
	}

	if currentTask != nil && len(currentTask.Key) > 0 {
		updates = append(updates,
			models.TypedSearchAttributeTask.ValueSet(currentTask.Key),
		)
	}

	elevationRequest, err := workflowTask.GetContextAsElevationRequest()

	if err != nil {

		log.WithError(err).Warn("No valid elevation context found, skipping search attribute update.")

	} else {
		if elevationRequest.User != nil && len(elevationRequest.User.Email) > 0 {
			updates = append(updates,
				models.TypedSearchAttributeUser.ValueSet(elevationRequest.User.Email),
			)
		}

		if len(elevationRequest.Role.Name) > 0 {
			updates = append(updates,
				models.TypedSearchAttributeRole.ValueSet(elevationRequest.Role.Name),
			)
		}

		if len(elevationRequest.Workflow) > 0 {
			updates = append(updates,
				models.TypedSearchAttributeWorkflow.ValueSet(elevationRequest.Workflow),
			)
		}

		if len(elevationRequest.Providers) > 0 {
			updates = append(updates,
				models.TypedSearchAttributeProviders.ValueSet(elevationRequest.Providers),
			)
		}

		if len(elevationRequest.Identities) > 0 {
			updates = append(updates,
				models.TypedSearchAttributeIdentities.ValueSet(elevationRequest.Identities),
			)
		}

	}

	log.WithFields(logrus.Fields{
		"workflowID": workflowTask.WorkflowID,
	}).Info("Updating temporal search attributes")

	return workflow.UpsertTypedSearchAttributes(ctx, updates...)
}
*/
