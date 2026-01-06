package manager

import (
	"github.com/thand-io/agent/internal/config"
	models "github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/sdk/workflows/functions"
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
