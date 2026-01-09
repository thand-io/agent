package examples

import (
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/sdk/workflows/config"
	manager "github.com/thand-io/agent/sdk/workflows/manager"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
)

const WorkflowHelloWorldName = "hello_world"
const WorkflowHelloWorldVersion = "1.0.0"

var WorkflowHelloWorld = model.NewWorkflowBuilder().
	SetDocument(
		"1.0.0-alpha5",
		"examples",
		WorkflowHelloWorldName,
		WorkflowHelloWorldVersion,
	).
	AddTask("greet", &model.SetTask{
		Set: map[string]any{
			"greeting": "Hello, World!",
		},
	}).
	Build()

// HelloWorld is a simple example workflow that returns "Hello, World!" message.
// This runs without Temporal, using the local workflow manager.
// Thand can run workflows with or without Temporal, making it easy to develop and test workflows locally.
// However, not using Temporal means no durability or scalability guarantees.
// Critically this means that no state is saved between workflow steps, so long running workflows
// that require waiting for external events will not function correctly without Temporal.
// State is relayed via redirect URLs handed back to the user.
func HelloWorld() any {

	newConfig := config.NewConfigService()

	// Create workflow manager
	workflowManager, err := manager.NewWorkflowManager(newConfig)

	if err != nil {
		panic(err)
	}

	// Create new workflow task context
	workflowTask, err := sdkWorkflowsModel.NewWorkflowContext(WorkflowHelloWorld)

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

func main() {
	HelloWorld()
}
