package models

import (
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
)

func NewElevationWorkflowContext(workflow *Workflow) (*ElevateWorkflowTask, error) {
	newWorkflow, err := sdkWorkflowsModel.NewWorkflowContext(workflow.Workflow)
	if err != nil {
		return nil, err
	}

	// Set the workflow identifier, this will be used to hydrate the
	// workflow as it transitions through different states
	newWorkflow.WorkflowName = workflow.GetIdentifier()

	return NewElevateWorkflowTask(newWorkflow), nil
}
