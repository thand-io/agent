package manager

import (
	"fmt"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/internal/config"
	models "github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/sdk/workflows/functions"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"github.com/thand-io/agent/sdk/workflows/tasks"
)

type thandWorkflowConfig struct {
	config    *config.Config
	functions *functions.FunctionRegistry
	tasks     *tasks.TaskRegistry
}

func NewThandWorkflowConfig(
	cfg *config.Config,
) *thandWorkflowConfig {
	return &thandWorkflowConfig{
		config:    cfg,
		functions: functions.NewFunctionRegistry(),
		tasks:     tasks.NewTaskRegistry(),
	}
}

func (c *thandWorkflowConfig) GetConfig() *config.Config {
	return c.config
}

func (c *thandWorkflowConfig) HasTemporal() bool {
	return c.config.GetServices().HasTemporal()
}

func (c *thandWorkflowConfig) GetTemporal() models.TemporalImpl {
	return c.config.GetServices().GetTemporal()
}

func (c *thandWorkflowConfig) GetFunctionRegistry() *functions.FunctionRegistry {
	return c.functions
}

func (c *thandWorkflowConfig) GetTaskRegistry() *tasks.TaskRegistry {
	return c.tasks
}

func (c *thandWorkflowConfig) RegisterFunction(f functions.Function) error {
	c.functions.RegisterFunction(f)
	return nil
}

func (c *thandWorkflowConfig) GetFunction(name string) (functions.Function, bool) {
	return c.functions.GetFunction(name)
}

func (c *thandWorkflowConfig) RegisterTask(t tasks.Task) error {
	c.tasks.RegisterTask(t)
	return nil
}

func (c *thandWorkflowConfig) GetTask(taskItem *model.TaskItem) (tasks.Task, bool) {
	return c.tasks.GetTaskHandler(taskItem)
}

func (c *thandWorkflowConfig) RegisterWorkflow(name string, workflow model.Workflow) error {
	return fmt.Errorf("registering workflows is not supported in ThandWorkflowConfig")
}

// HydrateWorkflowTask ensures that the workflow task has its workflow definition loaded
// and its state initialised.
func (c *thandWorkflowConfig) HydrateWorkflowTask(
	workflowTask sdkWorkflowsModel.WorkflowTaskSupport,
) error {

	if workflowTask.GetWorkflowDef() == nil {

		workflowDsl, err := c.GetConfig().GetWorkflowByName(workflowTask.GetName())

		if err != nil {
			return fmt.Errorf("failed to load workflow: %w", err)
		}

		workflowCopy := workflowDsl.GetWorkflowClone()

		if workflowCopy == nil {
			return fmt.Errorf("failed to clone workflow definition")
		}

		workflowTask.SetWorkflowDef(workflowCopy)

	}

	// Create a new task state if it does not exist
	// This is important as we might be in the middle of a workflow and
	// the state might not have been initialised yet
	if !workflowTask.HasState() {
		workflowTask.ClearTaskContext()
	}

	return nil
}
