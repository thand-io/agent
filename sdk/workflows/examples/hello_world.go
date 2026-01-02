package examples

import (
	"github.com/hashicorp/go-version"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/internal/models"
	manager "github.com/thand-io/agent/sdk/workflows/manager"
)

const WorkflowHelloWorldName = "hello_world"
const WorkflowHelloWorldVersion = "1.0.0"

func HelloWorld() any {

	cfg := NewConfig()
	wf := models.NewWorkflow(
		version.Must(version.NewVersion(WorkflowHelloWorldVersion)),
		"hello_world",
		"Hello World Workflow",
		model.NewWorkflowBuilder().
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
			Build(),
	)
	cfg.RegisterWorkflow(WorkflowHelloWorldName, wf)

	// Create workflow manager
	workflowManager := manager.NewWorkflowManager(cfg)

	// Get workflow by name, regisered in our config
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

func main() {
	HelloWorld()
}
