package config

import (
	"fmt"
	"strings"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	models "github.com/thand-io/agent/internal/models"
	sdkWorkflowsConfig "github.com/thand-io/agent/sdk/workflows/config"
	"github.com/thand-io/agent/sdk/workflows/functions"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"github.com/thand-io/agent/sdk/workflows/tasks"
)

type thandWorkflowConfig struct {
	config    models.ConfigImpl
	functions *functions.FunctionRegistry
	tasks     *tasks.TaskRegistry
}

func NewThandWorkflowConfig(
	cfg models.ConfigImpl,
) *thandWorkflowConfig {
	return &thandWorkflowConfig{
		config:    cfg,
		functions: functions.NewFunctionRegistry(),
		tasks:     tasks.NewTaskRegistry(),
	}
}

func (r *thandWorkflowConfig) CreateRunner(sdkM sdkWorkflowsModel.WorkflowTaskSupport) sdkWorkflowsConfig.RunnerConfig {
	return NewthandRunner(r, sdkM)
}

func (c *thandWorkflowConfig) GetConfig() models.ConfigImpl {
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

// GetWorkflow retrieves a workflow by name
// Its important that we return a copy of the workflow definition to avoid
// unintended mutations
func (c *thandWorkflowConfig) GetWorkflow(name string) (model.Workflow, bool) {

	workflowDsl, err := c.config.GetWorkflowByName(strings.ToLower(name))

	if err != nil {
		return model.Workflow{}, false
	}

	workflowCopy := workflowDsl.GetWorkflowClone()

	if workflowCopy == nil {
		return model.Workflow{}, false
	}

	return *workflowCopy, true
}

func (c *thandWorkflowConfig) RegisterWorkflow(name string, workflow model.Workflow) error {
	return fmt.Errorf("registering workflows is not supported in ThandWorkflowConfig")
}
