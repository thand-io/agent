package config

import (
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/workflow"
)

// This file create longs run workflows for a given system id

type ServerWorkflowStart struct {
	Identities []string
}

type ServerWorkflowShutdown struct{}

func CreateServerWorkflow(conifgImpl models.ConfigImpl, start ServerWorkflowStart) func(workflow.Context, ServerWorkflowStart) (*ServerWorkflowShutdown, error) {
	return func(ctx workflow.Context, req ServerWorkflowStart) (*ServerWorkflowShutdown, error) {
		return nil, nil
	}
}

type AgentWorkflowStart struct {
	Identities []string
}

type AgentWorkflowShutdown struct{}

func CreateAgentWorkflow(conifgImpl models.ConfigImpl, start AgentWorkflowStart) func(workflow.Context, AgentWorkflowStart) (*AgentWorkflowShutdown, error) {
	return func(ctx workflow.Context, req AgentWorkflowStart) (*AgentWorkflowShutdown, error) {
		return nil, nil
	}
}
