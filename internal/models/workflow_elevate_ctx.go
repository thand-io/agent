package models

import (
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
)

func NewElevationWorkflowContext(workflow *Workflow) (*ElevateWorkflowTask, error) {
	newWorkflow, err := sdkWorkflowsModel.NewWorkflowContext(workflow.Workflow)
	if err != nil {
		return nil, err
	}

	// Set the workflow name
	newWorkflow.WorkflowName = workflow.Name

	return NewElevateWorkflowTask(newWorkflow), nil
}
