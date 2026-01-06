package manager

import (
	"context"
	"fmt"
	"strings"

	swctx "github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	models "github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/sdk/workflows/config"
	"github.com/thand-io/agent/sdk/workflows/functions"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"github.com/thand-io/agent/sdk/workflows/tasks"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
)

// WorkflowManager manages workflow lifecycle and execution using the official SDK
type WorkflowManager struct {
	config config.Config
}

// NewWorkflowManager creates a new workflow manager
func NewWorkflowManager(cfg config.Config) (*WorkflowManager, error) {
	workflowManager := &WorkflowManager{
		config: cfg,
	}
	err := workflowManager.registerActivities()
	if err != nil {
		logrus.WithError(err).Error("Failed to register activities")
		return nil, err
	}
	err = workflowManager.registerWorkflows()
	if err != nil {
		logrus.WithError(err).Error("Failed to register workflows")
		return nil, err
	}
	return workflowManager, nil
}

func (m *WorkflowManager) GetConfig() config.Config {
	return m.config
}

func (m *WorkflowManager) HasTemporal() bool {
	return m.config.HasTemporal()
}

func (m *WorkflowManager) GetTemporal() models.TemporalImpl {
	return m.config.GetTemporal()
}

func (m *WorkflowManager) RegisterFunction(handler functions.Function) {
	m.config.GetFunctionRegistry().RegisterFunction(handler)
}

func (m *WorkflowManager) GetFunction(name string) (functions.Function, bool) {
	return m.config.GetFunctionRegistry().GetFunction(name)
}

func (m *WorkflowManager) GetFunctionRegistry() *functions.FunctionRegistry {
	return m.config.GetFunctionRegistry()
}

func (m *WorkflowManager) GetTaskRegistry() *tasks.TaskRegistry {
	return m.config.GetTaskRegistry()
}

// ResumeWorkflow resumes workflow execution from client-provided state
func (m *WorkflowManager) ResumeWorkflow(
	result sdkWorkflowsModel.WorkflowTask,
) (sdkWorkflowsModel.WorkflowTask, error) {

	ctx := result.GetContext()

	// If we have temporal configured with a client, then we can resume the workflow
	// from the workflow ID or create one if the workflow ID does not exist
	if m.HasTemporal() && m.GetTemporal().HasClient() {

		return m.resumeTemporalWorkflowTask(ctx, result)

	} else {

		return m.ResumeWorkflowTask(result)
	}

}

func (m *WorkflowManager) ResumeWorkflowTask(
	workflowTask sdkWorkflowsModel.WorkflowTask,
) (sdkWorkflowsModel.WorkflowTask, error) {
	return ResumeWorkflowTask(
		m.config,
		workflowTask,
	)
}

func (m *WorkflowManager) resumeTemporalWorkflowTask(
	ctx context.Context,
	result sdkWorkflowsModel.WorkflowTask,
) (sdkWorkflowsModel.WorkflowTask, error) {

	// Check the workflow task
	if result.HasState() {
		result.ClearTaskContext()
	}

	temporalService := m.GetTemporal()
	temporalClient := temporalService.GetClient()

	_, err := temporalClient.DescribeWorkflow(
		ctx, result.GetWorkflowID(), sdkWorkflowsModel.TemporalEmptyRunId)

	if err != nil {

		// Not found, so start a new workflow execution
		err := m.createTemporalWorkflow(result)

		if err != nil {
			return nil, fmt.Errorf("failed to create temporal workflow: %w", err)
		}

	}

	// Lets signal the workflow to continue
	err = temporalClient.SignalWorkflow(
		ctx, result.GetWorkflowID(), sdkWorkflowsModel.TemporalEmptyRunId,
		sdkWorkflowsModel.TemporalResumeSignalName, result)

	if err != nil {
		return nil, fmt.Errorf("failed to signal workflow: %w", err)
	}

	return result, nil
}

// RegisterCustomFunction allows external code to register additional functions
func (m *WorkflowManager) RegisterCustomFunction(handler functions.Function) {
	m.RegisterFunction(handler)
	logrus.WithField("function", handler.GetName()).Info("Registered external custom function")
}

// GetRegisteredFunctions returns all currently registered functions
func (m *WorkflowManager) GetRegisteredFunctions() []string {
	return m.config.GetFunctionRegistry().GetRegisteredFunctions()
}

func (m *WorkflowManager) createTemporalWorkflow(workflowTask sdkWorkflowsModel.WorkflowTask) error {
	// Not found, so start a new workflow execution

	logrus.WithFields(logrus.Fields{
		"workflow_id": workflowTask.GetWorkflowID(),
	}).Info("Starting new workflow execution")

	temporalService := m.GetTemporal()
	temporalClient := temporalService.GetClient()

	ctx := workflowTask.GetContext()

	// Build workflow options
	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowTask.GetWorkflowID(),
		TaskQueue: temporalService.GetTaskQueue(),
		TypedSearchAttributes: temporal.NewSearchAttributes(
			models.TypedSearchAttributeStatus.ValueSet(strings.ToUpper(string(swctx.PendingStatus))),
		),
	}

	// Only add versioning override if versioning is enabled
	if !temporalService.IsVersioningDisabled() {
		workflowOptions.VersioningOverride = &client.PinnedVersioningOverride{
			Version: worker.WorkerDeploymentVersion{
				DeploymentName: models.TemporalDeploymentName,
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
