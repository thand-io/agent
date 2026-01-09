package config

import (
	"fmt"
	"strings"

	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
)

type RunnerConfig interface {

	// Store the workflow manager config
	GetConfig() Config
	GetWorkflowTask() sdkWorkflowsModel.WorkflowTaskSupport

	// HydrateWorkflowTask used to populate the workflow definition in a workflow task
	// This is useful so that we don't have to embed the full workflow definition in every task instance
	HydrateWorkflowTask() error

	// Hooks for pre and post state transitions
	PreStateTransitionHook(
		workflowTask sdkWorkflowsModel.WorkflowTaskSupport,
	) error
	PostStateTransitionHook(
		workflowTask sdkWorkflowsModel.WorkflowTaskSupport,
	) error
}

type runnerConfig struct {
	config       Config
	workflowTask sdkWorkflowsModel.WorkflowTaskSupport
}

func NewRunnerConfig(cfg Config, workflowTask sdkWorkflowsModel.WorkflowTaskSupport) RunnerConfig {
	return &runnerConfig{
		config:       cfg,
		workflowTask: workflowTask,
	}
}

func (r *runnerConfig) GetConfig() Config {
	return r.config
}

func (r *runnerConfig) GetWorkflowTask() sdkWorkflowsModel.WorkflowTaskSupport {
	return r.workflowTask
}

func (c *runnerConfig) HydrateWorkflowTask() error {

	workflowTask := c.GetWorkflowTask()

	if workflowTask.GetWorkflowDef() == nil {

		workflowDsl, ok := c.config.GetWorkflow(strings.ToLower(workflowTask.GetName()))

		if !ok {
			return fmt.Errorf("workflow %s not found", workflowTask.GetName())
		}

		// TODO: Clone the workflow definition to avoid mutations
		workflowTask.SetWorkflowDef(&workflowDsl)

	}

	return nil
}

func (r *runnerConfig) PreStateTransitionHook(workflowTask sdkWorkflowsModel.WorkflowTaskSupport) error {
	// No-op by default
	return nil
}

func (r *runnerConfig) PostStateTransitionHook(workflowTask sdkWorkflowsModel.WorkflowTaskSupport) error {
	// No-op by default
	return nil
}
