package config

import (
	"fmt"

	"github.com/thand-io/agent/internal/workflows/common"
	sdkWorkflowsConfig "github.com/thand-io/agent/sdk/workflows/config"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
)

type thandRunner struct {
	config       sdkWorkflowsConfig.Config
	workflowTask sdkWorkflowsModel.WorkflowTaskSupport
}

func NewthandRunner(cfg sdkWorkflowsConfig.Config, workflowTask sdkWorkflowsModel.WorkflowTaskSupport) *thandRunner {
	return &thandRunner{
		config:       cfg,
		workflowTask: workflowTask,
	}
}

func (r *thandRunner) GetConfig() sdkWorkflowsConfig.Config {
	return r.config
}

func (r *thandRunner) GetWorkflowTask() sdkWorkflowsModel.WorkflowTaskSupport {
	return r.workflowTask
}

// HydrateWorkflowTask ensures that the workflow task has its workflow definition loaded
// and its state initialised.
func (c *thandRunner) HydrateWorkflowTask() error {

	workflowTask := c.GetWorkflowTask()

	if workflowTask.GetWorkflowDef() == nil {

		workflowDsl, ok := c.GetConfig().GetWorkflow(workflowTask.GetName())

		if !ok {
			return fmt.Errorf("failed to load workflow")
		}

		workflowTask.SetWorkflowDef(&workflowDsl)

	}

	// Create a new task state if it does not exist
	// This is important as we might be in the middle of a workflow and
	// the state might not have been initialised yet
	if !workflowTask.HasState() {
		workflowTask.ClearTaskContext()
	}

	return nil
}

func (r *thandRunner) PreStateTransitionHook(
	workflowTask sdkWorkflowsModel.WorkflowTaskSupport,
) error {
	// No-op by default
	return common.UpdateSearchAttributes(workflowTask)
}

func (r *thandRunner) PostStateTransitionHook(
	workflowTask sdkWorkflowsModel.WorkflowTaskSupport,
) error {
	// No-op by default
	return common.UpdateSearchAttributes(workflowTask)
}
