package config

import (
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
)

type RunnerConfig interface {

	// Store the workflow manager config
	GetConfig() Config
	GetWorkflowTask() sdkWorkflowsModel.WorkflowTaskSupport

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

func (r *runnerConfig) PreStateTransitionHook(workflowTask sdkWorkflowsModel.WorkflowTaskSupport) error {
	// No-op by default
	return nil
}

func (r *runnerConfig) PostStateTransitionHook(workflowTask sdkWorkflowsModel.WorkflowTaskSupport) error {
	// No-op by default
	return nil
}
