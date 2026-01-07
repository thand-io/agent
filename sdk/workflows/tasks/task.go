package tasks

import (
	"github.com/serverlessworkflow/sdk-go/v3/model"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
)

type TaskCollection interface {
	RegisterTasks(registry *TaskRegistry)
}

// Task defines the interface that all Thand Tasks must implement
type Task interface {
	// GetName returns the unique name/identifier for this Task
	GetName() string

	// GetDescription returns a human-readable description of what this Task does
	GetDescription() string

	// GetVersion returns the version of this Task implementation
	GetVersion() string

	// Execute performs the main Function logic
	// Security: All inputs should be pre-validated by ValidateRequest
	Execute(
		workflowTask *sdkWorkflowsModel.WorkflowTask,
		task *model.TaskItem,
		input any,
	) (any, error)
}
