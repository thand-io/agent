package runner

import (
	"testing"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/functions"
)

// TestProcessTaskOutput tests the processTaskOutput function which transforms
// raw task output using the output.as expression.
//
// Example transformation:
// Raw output: {"data": {"approved": true}}
// Transform expression: { "approvals": [{"approved": .data.approved}] }
// Transformed output: {"approvals": [{"approved": true}]}

func TestProcessTaskOutput_TransformToApprovalsList(t *testing.T) {
	// Create a test configuration
	cfg := config.DefaultConfig()

	// Create function registry
	functionRegistry := functions.NewFunctionRegistry(cfg)

	// Create a mock workflow
	workflow := &model.Workflow{
		Document: model.Document{
			Name:    "test-workflow",
			Version: "1.0.0",
		},
	}

	// Create a workflow context
	workflowCtx, err := models.NewWorkflowContext(&models.Workflow{
		Name:        workflow.Document.Name,
		Description: "Test workflow",
		Workflow:    workflow,
		Enabled:     true,
	})
	require.NoError(t, err)

	// Create the runner
	runner := NewResumableRunner(cfg, functionRegistry, workflowCtx)

	// Create task output configuration that transforms the raw output
	// Raw output: {"data": {"approved": true}}
	// Transformed output: {"approvals": [{"approved": true}]}
	taskOutput := &model.Output{
		As: model.NewObjectOrRuntimeExpr(`${ { "approvals": [{"approved": .data.approved}] } }`),
	}

	// Create task base with the output
	taskBase := &model.TaskBase{
		Output: taskOutput,
	}

	// Raw task output (what a Set task would produce)
	rawTaskOutput := map[string]any{
		"data": map[string]any{
			"approved": true,
		},
	}

	// Execute processTaskOutput
	result, err := runner.processTaskOutput(taskBase, rawTaskOutput, "test-task")
	assert.NoError(t, err)

	// Verify the transformation
	expected := map[string]any{
		"approvals": []any{
			map[string]any{"approved": true},
		},
	}
	assert.Equal(t, expected, result, "Should transform raw output to approvals list")
}

func TestProcessTaskOutput_TransformToApprovalsListWithExport(t *testing.T) {
	// Create a test configuration
	cfg := config.DefaultConfig()

	// Create function registry
	functionRegistry := functions.NewFunctionRegistry(cfg)

	// Create a mock workflow
	workflow := &model.Workflow{
		Document: model.Document{
			Name:    "test-workflow",
			Version: "1.0.0",
		},
	}

	// Create a workflow context
	workflowCtx, err := models.NewWorkflowContext(&models.Workflow{
		Name:        workflow.Document.Name,
		Description: "Test workflow",
		Workflow:    workflow,
		Enabled:     true,
	})
	require.NoError(t, err)

	// Create the runner
	runner := NewResumableRunner(cfg, functionRegistry, workflowCtx)

	// Create task output configuration that transforms the raw output
	// Raw output: {"data": {"approved": true}}
	// Transformed output: {"approvals": [{"approved": true}]}
	taskOutput := &model.Output{
		As: model.NewObjectOrRuntimeExpr(`${ { "approvals": [{"approved": .data.approved}] } }`),
	}

	// Create task export configuration that adds the approvals to context
	// This will merge the transformed output into the existing context
	taskExport := &model.Export{
		As: model.NewObjectOrRuntimeExpr(`${ $context + { "approvals": .approvals } }`),
	}

	// Create task base with both output and export
	taskBase := &model.TaskBase{
		Output: taskOutput,
		Export: taskExport,
	}

	// Set up initial context with some existing data
	runner.workflowTask.SetInstanceCtx(map[string]any{
		"user": "test.user",
		"role": "admin",
	})

	// Raw task output (what a Set task would produce)
	rawTaskOutput := map[string]any{
		"data": map[string]any{
			"approved": true,
		},
	}

	// Execute processTaskOutput
	result, err := runner.processTaskOutput(taskBase, rawTaskOutput, "test-task")
	assert.NoError(t, err)

	// Verify the transformation
	expected := map[string]any{
		"approvals": []any{
			map[string]any{"approved": true},
		},
	}
	assert.Equal(t, expected, result, "Should transform raw output to approvals list")

	// Execute processTaskExport to add transformed output to context
	err = runner.processTaskExport(taskBase, result, "test-task")
	assert.NoError(t, err)

	// Verify the context now contains the approvals along with existing data
	context := runner.workflowTask.GetInstanceCtx()
	expectedContext := map[string]any{
		"user": "test.user",
		"role": "admin",
		"approvals": []any{
			map[string]any{"approved": true},
		},
	}
	assert.Equal(t, expectedContext, context, "Context should contain approvals and existing data")
}

func TestProcessTaskOutput_WithoutOutput(t *testing.T) {
	// Create a test configuration
	cfg := config.DefaultConfig()

	// Create function registry
	functionRegistry := functions.NewFunctionRegistry(cfg)

	// Create a mock workflow
	workflow := &model.Workflow{
		Document: model.Document{
			Name:    "test-workflow",
			Version: "1.0.0",
		},
	}

	// Create a workflow context
	workflowCtx, err := models.NewWorkflowContext(&models.Workflow{
		Name:        workflow.Document.Name,
		Description: "Test workflow",
		Workflow:    workflow,
		Enabled:     true,
	})
	require.NoError(t, err)

	// Create the runner
	runner := NewResumableRunner(cfg, functionRegistry, workflowCtx)

	// Create task base without output
	taskBase := &model.TaskBase{
		Output: nil,
	}

	// Set up task output data
	rawTaskOutput := map[string]any{
		"data": map[string]any{
			"approved": true,
		},
	}

	// Execute processTaskOutput - should return the raw output unchanged
	result, err := runner.processTaskOutput(taskBase, rawTaskOutput, "test-task")
	assert.NoError(t, err)
	assert.Equal(t, rawTaskOutput, result)
}

func TestProcessTaskOutput_TransformStructure(t *testing.T) {
	// Create a test configuration
	cfg := config.DefaultConfig()

	// Create function registry
	functionRegistry := functions.NewFunctionRegistry(cfg)

	// Create a mock workflow
	workflow := &model.Workflow{
		Document: model.Document{
			Name:    "test-workflow",
			Version: "1.0.0",
		},
	}

	// Create a workflow context
	workflowCtx, err := models.NewWorkflowContext(&models.Workflow{
		Name:        workflow.Document.Name,
		Description: "Test workflow",
		Workflow:    workflow,
		Enabled:     true,
	})
	require.NoError(t, err)

	// Create the runner
	runner := NewResumableRunner(cfg, functionRegistry, workflowCtx)

	// Create task output configuration that transforms the raw output structure
	taskOutput := &model.Output{
		As: model.NewObjectOrRuntimeExpr("${ { approved: .data.approved, user: .data.user } }"),
	}

	// Create task base with the output
	taskBase := &model.TaskBase{
		Output: taskOutput,
	}

	// Set up raw task output data - this represents the cloudevents.Event
	rawTaskOutput := map[string]any{
		"data": map[string]any{
			"approved":  false,
			"user":      "jane.doe",
			"timestamp": "2023-01-01T00:00:00Z",
			"metadata":  map[string]any{"source": "approval-service"},
		},
	}

	// Execute processTaskOutput
	result, err := runner.processTaskOutput(taskBase, rawTaskOutput, "test-task")
	assert.NoError(t, err)

	// Verify the result - should be the transformed structure
	expected := map[string]any{
		"approved": false,
		"user":     "jane.doe",
	}
	assert.Equal(t, expected, result, "Should transform the output structure")
}

func TestProcessTaskOutput_IdentityTransform(t *testing.T) {
	// Create a test configuration
	cfg := config.DefaultConfig()

	// Create function registry
	functionRegistry := functions.NewFunctionRegistry(cfg)

	// Create a mock workflow
	workflow := &model.Workflow{
		Document: model.Document{
			Name:    "test-workflow",
			Version: "1.0.0",
		},
	}

	// Create a workflow context
	workflowCtx, err := models.NewWorkflowContext(&models.Workflow{
		Name:        workflow.Document.Name,
		Description: "Test workflow",
		Workflow:    workflow,
		Enabled:     true,
	})
	require.NoError(t, err)

	// Create the runner
	runner := NewResumableRunner(cfg, functionRegistry, workflowCtx)

	// Create task output configuration using identity transform (the default)
	taskOutput := &model.Output{
		As: model.NewObjectOrRuntimeExpr("${ . }"),
	}

	// Create task base with the output
	taskBase := &model.TaskBase{
		Output: taskOutput,
	}

	// Set up raw task output data - this represents the cloudevents.Event
	rawTaskOutput := map[string]any{
		"data": map[string]any{
			"approved": true,
			"user":     "john.doe",
		},
		"type":   "approval-request",
		"source": "approval-service",
	}

	// Execute processTaskOutput
	result, err := runner.processTaskOutput(taskBase, rawTaskOutput, "test-task")
	assert.NoError(t, err)

	// Verify the result - should be identical to the input (identity transform)
	assert.Equal(t, rawTaskOutput, result, "Identity transform should return unchanged output")
}
