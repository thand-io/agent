package config

import (
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/functions"
	"github.com/thand-io/agent/internal/workflows/tasks"

	workflowModels "github.com/thand-io/agent/internal/workflows/models"
)

type Config interface {
	HasTemporal() bool
	GetTemporal() models.TemporalImpl

	GetFunctionRegistry() *functions.FunctionRegistry
	GetTaskRegistry() *tasks.TaskRegistry

	//
	Hydrate(v *workflowModels.WorkflowTask) error
}

type configService struct {
	temporal  models.TemporalImpl
	functions *functions.FunctionRegistry
	tasks     *tasks.TaskRegistry
}

func NewConfigService(functions *functions.FunctionRegistry, tasks *tasks.TaskRegistry) *configService {
	return &configService{
		functions: functions,
		tasks:     tasks,
	}
}

func (c *configService) WithTemporal(temporal models.TemporalImpl) *configService {
	c.temporal = temporal
	return c
}

func (c *configService) HasTemporal() bool {
	return true
}

func (c *configService) GetTemporal() bool {
	return true
}

func (c *configService) GetFunctionRegistry() *functions.FunctionRegistry {
	return c.functions
}

func (c *configService) GetTaskRegistry() *tasks.TaskRegistry {
	return c.tasks
}
