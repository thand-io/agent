package manager

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	swctx "github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/config"
	models "github.com/thand-io/agent/internal/models"
	workflowConfig "github.com/thand-io/agent/internal/workflows/config"
	providerAws "github.com/thand-io/agent/internal/workflows/functions/providers/aws"
	providerGcp "github.com/thand-io/agent/internal/workflows/functions/providers/gcp"
	providerSlack "github.com/thand-io/agent/internal/workflows/functions/providers/slack"
	providerThand "github.com/thand-io/agent/internal/workflows/functions/providers/thand"
	taskThand "github.com/thand-io/agent/internal/workflows/tasks/providers/thand"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
	sdkWorkflowsConfig "github.com/thand-io/agent/sdk/workflows/config"
	"github.com/thand-io/agent/sdk/workflows/functions"
	workflowSdk "github.com/thand-io/agent/sdk/workflows/manager"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"github.com/thand-io/agent/sdk/workflows/tasks"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
)

// WorkflowManager manages workflow lifecycle and execution using the official SDK
type ThandWorkflowManager struct {
	config          *config.Config
	workflowManager *workflowSdk.WorkflowManager
}

// NewWorkflowManager creates a new workflow manager
func NewThandWorkflowManager(cfg *config.Config) (*ThandWorkflowManager, error) {

	workflowConfig := workflowConfig.NewThandWorkflowConfig(cfg)

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

	if cfg.GetServices().HasTemporal() {

		// Register our custom temporal workflow
		err = thandWorkflowManager.registerThandWorkflows()

		if err != nil {
			logrus.WithError(err).Error("Failed to register thand workflows")
			return nil, fmt.Errorf("failed to register thand workflows: %w", err)
		}
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
	return m.workflowManager.GetConfig().GetTaskRegistry()
}

func (m *ThandWorkflowManager) GetFunctionRegistry() *functions.FunctionRegistry {
	return m.workflowManager.GetConfig().GetFunctionRegistry()
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

	sanitizedRequest := request
	sanitizedRequest.Session = nil

	logrus.WithFields(logrus.Fields{
		"workflow_name": workflowDsl.Document.Name,
		"request":       sanitizedRequest,
	}).Info("Starting workflow execution")

	authProvider, foundAuthProvider := m.config.GetProviderByName(request.Authenticator)

	if foundAuthProvider != nil {
		return nil, fmt.Errorf("authentication provider not found: %s", request.Authenticator)
	}

	// Convert input to map
	internalContext := request.AsMap()

	workflowTask, err := models.NewElevationWorkflowContext(workflow)

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

			err = authProvider.ValidateSession(ctx, decodedSession.Session)

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

	sessionResponse, err := authProvider.AuthorizeSession(ctx, &models.AuthorizeUser{
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
	workflowTask *models.ElevateWorkflowTask,
) (*models.ElevateWorkflowTask, error) {

	ctx := workflowTask.GetContext()

	// Check if workfow has already been registered on temporal
	serviceClient := m.config.GetServices()

	// If we have temporal configured with a client, then we can resume the workflow
	// from the workflow ID or create one if the workflow ID does not exist
	if serviceClient.HasTemporal() && serviceClient.GetTemporal().HasClient() {

		temporalService := serviceClient.GetTemporal()
		temporalClient := temporalService.GetClient()

		_, err := temporalClient.DescribeWorkflow(
			ctx,
			workflowTask.GetWorkflowID(),
			sdkWorkflowsModel.TemporalEmptyRunId,
		)

		if err != nil {

			// Not found, so start a new workflow execution
			err := m.createTemporalWorkflow(workflowTask)

			if err != nil {
				return nil, fmt.Errorf("failed to create temporal workflow: %w", err)
			}

		}

		// Lets signal the workflow to continue
		err = temporalClient.SignalWorkflow(
			ctx,
			workflowTask.GetWorkflowID(),
			sdkWorkflowsModel.TemporalEmptyRunId,
			sdkWorkflowsModel.TemporalResumeSignalName,
			workflowTask,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to signal workflow: %w", err)
		}

		return workflowTask, nil

	} else {

		return m.ResumeWorkflowTask(workflowTask)
	}
}

func (m *ThandWorkflowManager) ResumeWorkflowTask(
	workflowTask *models.ElevateWorkflowTask,
) (*models.ElevateWorkflowTask, error) {

	result, err := m.workflowManager.ResumeWorkflowTask(workflowTask)

	if err != nil {
		return nil, fmt.Errorf("failed to resume workflow task: %w", err)
	}

	return models.NewElevateWorkflowTask(result), nil
}

func (m *ThandWorkflowManager) createTemporalWorkflow(workflowTask *models.ElevateWorkflowTask) error {
	// Not found, so start a new workflow execution

	logrus.WithFields(logrus.Fields{
		"workflow_id": workflowTask.WorkflowID,
	}).Info("Starting new workflow execution")

	serviceClient := m.config.GetServices()

	temporalService := serviceClient.GetTemporal()
	temporalClient := temporalService.GetClient()

	elevationRequest, err := workflowTask.GetContextAsElevationRequest()

	if err != nil {
		return fmt.Errorf("failed to get workflow context: %w", err)
	}

	userEmail := ""
	roleName := ""

	if elevationRequest == nil {
		return fmt.Errorf("workflow context is nil")
	}

	if elevationRequest.User != nil {
		userEmail = elevationRequest.User.Email
	}

	if elevationRequest.Role != nil {
		roleName = elevationRequest.Role.Name
	}

	// Convert duration to int64 seconds
	duration, err := common.ValidateDuration(elevationRequest.Duration)
	if err != nil {
		return fmt.Errorf("invalid duration: %w", err)
	}

	ctx := workflowTask.GetContext()

	// Build workflow options
	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowTask.WorkflowID,
		TaskQueue: temporalService.GetTaskQueue(),
		TypedSearchAttributes: temporal.NewSearchAttributes(
			models.TypedSearchAttributeUser.ValueSet(userEmail),
			models.TypedSearchAttributeRole.ValueSet(roleName),
			models.TypedSearchAttributeProviders.ValueSet(elevationRequest.Providers),
			models.TypedSearchAttributeWorkflow.ValueSet(elevationRequest.Workflow),
			models.TypedSearchAttributeStatus.ValueSet(strings.ToUpper(string(swctx.PendingStatus))),
			models.TypedSearchAttributeDuration.ValueSet(int64(duration.Seconds())),
			models.TypedSearchAttributeReason.ValueSet(elevationRequest.Reason),
			models.TypedSearchAttributeIdentities.ValueSet(elevationRequest.Identities),
		),
	}

	// Only add versioning override if versioning is enabled
	if !temporalService.IsVersioningDisabled() {
		workflowOptions.VersioningOverride = &client.PinnedVersioningOverride{
			Version: worker.WorkerDeploymentVersion{
				DeploymentName: sdkConstants.TemporalDeploymentName,
				BuildID:        common.GetBuildIdentifier(),
			},
		}
	}

	// Create new workflow
	we, err := temporalClient.ExecuteWorkflow(
		ctx,
		workflowOptions,
		models.TemporalExecuteElevationWorkflowName,
		workflowTask,
	)

	if err != nil {
		return fmt.Errorf("failed to start workflow: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"workflow_id": we.GetID(),
		"run_id":      we.GetRunID(),
	}).Info("Started new workflow execution")

	return nil
}
