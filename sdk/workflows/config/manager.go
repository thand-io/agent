package config

import (
	"fmt"
	"strings"
	"sync"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/sdk/models"
	"github.com/thand-io/agent/sdk/workflows/functions"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"github.com/thand-io/agent/sdk/workflows/tasks"
)

type Config interface {
	HasTemporal() bool
	GetTemporal() models.TemporalService

	GetFunctionRegistry() *functions.FunctionRegistry
	RegisterFunction(f functions.Function) error
	GetFunction(name string) (functions.Function, bool)

	RegisterTask(t tasks.Task) error
	GetTaskRegistry() *tasks.TaskRegistry
	GetTask(taskItem *model.TaskItem) (tasks.Task, bool)

	RegisterWorkflow(name string, workflow model.Workflow) error

	// HydrateWorkflowTask used to populate the workflow definition in a workflow task
	// This is useful so that we don't have to embed the full workflow definition in every task instance
	HydrateWorkflowTask(workflowTask sdkWorkflowsModel.WorkflowTaskSupport) error
}

type configService struct {
	temporal  models.TemporalService
	functions *functions.FunctionRegistry
	workflows map[string]model.Workflow
	tasks     *tasks.TaskRegistry
	mu        sync.RWMutex
}

func NewConfigService() *configService {
	return &configService{
		workflows: make(map[string]model.Workflow),
		functions: functions.NewFunctionRegistry(),
		tasks:     tasks.NewTaskRegistry(),
	}
}

func (c *configService) RegisterFunction(f functions.Function) error {
	c.functions.RegisterFunction(f)
	return nil
}

func (c *configService) RegisterTask(t tasks.Task) error {
	c.tasks.RegisterTask(t)
	return nil
}

func (c *configService) RegisterWorkflow(name string, workflow model.Workflow) error {
	if _, exists := c.workflows[name]; exists {
		return fmt.Errorf("workflow with name %s already exists", name)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.workflows[name] = workflow
	return nil
}

func (c *configService) GetWorkflow(name string) (model.Workflow, bool) {

	c.mu.RLock()
	defer c.mu.RUnlock()
	workflow, exists := c.workflows[name]

	return workflow, exists
}

func (c *configService) WithTemporal(temporal models.TemporalService) *configService {
	c.temporal = temporal
	return c
}

func (c *configService) HasTemporal() bool {
	return c.temporal != nil
}

func (c *configService) GetTemporal() models.TemporalService {
	return c.temporal
}

func (c *configService) GetFunction(name string) (functions.Function, bool) {
	return c.functions.GetFunction(name)
}

func (c *configService) GetTask(taskItem *model.TaskItem) (tasks.Task, bool) {
	return c.tasks.GetTaskHandler(taskItem)
}

func (c *configService) GetTaskRegistry() *tasks.TaskRegistry {
	return c.tasks
}

func (c *configService) GetFunctionRegistry() *functions.FunctionRegistry {
	return c.functions
}

func (c *configService) HydrateWorkflowTask(workflowTask sdkWorkflowsModel.WorkflowTaskSupport) error {

	if workflowTask.GetWorkflowDef() == nil {

		workflowDsl, ok := c.GetWorkflow(strings.ToLower(workflowTask.GetName()))

		if !ok {
			return fmt.Errorf("workflow %s not found", workflowTask.GetName())
		}

		// TODO: Clone the workflow definition to avoid mutations
		workflowTask.SetWorkflowDef(&workflowDsl)

	}

	return nil
}
