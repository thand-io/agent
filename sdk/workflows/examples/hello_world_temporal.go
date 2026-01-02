package examples

import (
	"github.com/thand-io/agent/internal/models"
	manager "github.com/thand-io/agent/sdk/workflows/manager"
)

func HelloWorldTemporal() any {

	cfg := NewConfig()

	// Setup local temporal

	err := cfg.RegisterWorkflow(WorkflowHelloWorldName, WorkflowHellowWorld)

	if err != nil {
		panic(err)
	}

	// Create workflow manager
	workflowManager := manager.NewWorkflowManager(cfg)

	// Get workflow by name, registered in our config
	workflow, err := cfg.GetWorkflowByName(WorkflowHelloWorldName)

	if err != nil {
		panic(err)
	}

	// Create new workflow task context
	workflowTask, err := models.NewWorkflowContext(workflow)

	if err != nil {
		panic(err)
	}

	// Dispatch workflow
	result, err := workflowManager.ResumeWorkflow(workflowTask)

	if err != nil {
		panic(err)
	}

	return result.GetOutput()
}
