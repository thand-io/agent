package config

import (
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/sdk/workflows/functions"
	"github.com/thand-io/agent/sdk/workflows/tasks"
)

type Config interface {
	HasTemporal() bool
	GetTemporal() models.TemporalImpl

	GetFunctionRegistry() *functions.FunctionRegistry
	GetTaskRegistry() *tasks.TaskRegistry
}

type configService struct {
	temporal  models.TemporalImpl
	functions *functions.FunctionRegistry
	tasks     *tasks.TaskRegistry
}

func NewConfigService() *configService {
	return &configService{
		functions: functions.NewFunctionRegistry(),
		tasks:     tasks.NewTaskRegistry(),
	}
}

func (c *configService) RegisterFunction(f functions.Function) {
	c.functions.RegisterFunction(f)
}

func (c *configService) RegisterTask(t tasks.Task) {
	c.tasks.RegisterTask(t)
}

func (c *configService) WithTemporal(temporal models.TemporalImpl) *configService {
	c.temporal = temporal
	return c
}

func (c *configService) HasTemporal() bool {
	return true
}

func (c *configService) GetTemporal() models.TemporalImpl {
	return c.temporal
}

func (c *configService) GetFunctionRegistry() *functions.FunctionRegistry {
	return c.functions
}

func (c *configService) GetTaskRegistry() *tasks.TaskRegistry {
	return c.tasks
}
