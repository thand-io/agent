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

func TestExecuteSetTask_BasicMapSet(t *testing.T) {
	// Create a test runner
	cfg := &config.Config{}
	functionRegistry := functions.NewFunctionRegistry(cfg)
	workflowTask := &models.WorkflowTask{
		WorkflowID: "test-workflow",
	}

	runner := &ResumableWorkflowRunner{
		config:       cfg,
		functions:    functionRegistry,
		workflowTask: workflowTask,
	}

	// Create a set task with static values
	setTask := &model.SetTask{
		Set: map[string]any{
			"shape": "circle",
			"size":  10,
			"color": "red",
		},
	}

	// Execute the set task
	result, err := runner.executeSetTask("testSet", setTask, map[string]any{})

	// Verify the result
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Cast result to map for verification
	resultMap, ok := result.(map[string]any)
	require.True(t, ok, "result should be a map[string]any")
	assert.Equal(t, "circle", resultMap["shape"])
	assert.Equal(t, 10, resultMap["size"])
	assert.Equal(t, "red", resultMap["color"])
}

func TestExecuteSetTask_WithRuntimeExpressions(t *testing.T) {
	// Create a test runner
	cfg := &config.Config{}
	functionRegistry := functions.NewFunctionRegistry(cfg)
	workflowTask := &models.WorkflowTask{
		WorkflowID: "test-workflow",
	}

	runner := &ResumableWorkflowRunner{
		config:       cfg,
		functions:    functionRegistry,
		workflowTask: workflowTask,
	}

	// Create input data
	input := map[string]any{
		"configuration": map[string]any{
			"size":  20,
			"fill":  "blue",
			"color": "green",
		},
	}

	// Create a set task with runtime expressions
	setTask := &model.SetTask{
		Set: map[string]any{
			"shape": "circle",
			"size":  "${ .configuration.size }",
			"fill":  "${ .configuration.fill }",
		},
	}

	// Execute the set task
	result, err := runner.executeSetTask("testSetWithExpressions", setTask, input)

	// Verify the result
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Cast result to map for verification
	resultMap, ok := result.(map[string]any)
	require.True(t, ok, "result should be a map[string]any")
	assert.Equal(t, "circle", resultMap["shape"])
	assert.Equal(t, 20, resultMap["size"])
	assert.Equal(t, "blue", resultMap["fill"])
}

func TestExecuteSetTask_DirectRuntimeExpression(t *testing.T) {
	// Create a test runner
	cfg := &config.Config{}
	functionRegistry := functions.NewFunctionRegistry(cfg)
	workflowTask := &models.WorkflowTask{
		WorkflowID: "test-workflow",
	}

	runner := &ResumableWorkflowRunner{
		config:       cfg,
		functions:    functionRegistry,
		workflowTask: workflowTask,
	}

	// Create input data
	input := map[string]any{
		"configuration": map[string]any{
			"color": "purple",
		},
	}

	// Create a set task with runtime expression that evaluates to the configuration object
	setTask := &model.SetTask{
		Set: map[string]any{
			"result": "${ .configuration }",
		},
	}

	// Execute the set task
	result, err := runner.executeSetTask("testDirectExpression", setTask, input)

	// Verify the result
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Cast result to map for verification
	resultMap, ok := result.(map[string]any)
	require.True(t, ok, "result should be a map[string]any")

	resultConfig, ok := resultMap["result"].(map[string]any)
	require.True(t, ok, "result should contain configuration map")
	assert.Equal(t, "purple", resultConfig["color"])
}

func TestExecuteSetTask_NestedData(t *testing.T) {
	// Create a test runner
	cfg := &config.Config{}
	functionRegistry := functions.NewFunctionRegistry(cfg)
	workflowTask := &models.WorkflowTask{
		WorkflowID: "test-workflow",
	}

	runner := &ResumableWorkflowRunner{
		config:       cfg,
		functions:    functionRegistry,
		workflowTask: workflowTask,
	}

	// Create a set task with nested data
	setTask := &model.SetTask{
		Set: map[string]any{
			"data": map[string]any{
				"approved": true,
				"metadata": map[string]any{
					"timestamp": "2025-10-05",
					"user":      "admin",
				},
			},
			"status": "active",
		},
	}

	// Execute the set task
	result, err := runner.executeSetTask("testNestedData", setTask, map[string]any{})

	// Verify the result
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Cast result to map for verification
	resultMap, ok := result.(map[string]any)
	require.True(t, ok, "result should be a map[string]any")

	// Check nested structure
	data, ok := resultMap["data"].(map[string]any)
	require.True(t, ok, "data should be a map")
	assert.Equal(t, true, data["approved"])

	metadata, ok := data["metadata"].(map[string]any)
	require.True(t, ok, "metadata should be a map")
	assert.Equal(t, "2025-10-05", metadata["timestamp"])
	assert.Equal(t, "admin", metadata["user"])

	assert.Equal(t, "active", resultMap["status"])
}

func TestExecuteSetTask_EmptySet(t *testing.T) {
	// Create a test runner
	cfg := &config.Config{}
	functionRegistry := functions.NewFunctionRegistry(cfg)
	workflowTask := &models.WorkflowTask{
		WorkflowID: "test-workflow",
	}

	runner := &ResumableWorkflowRunner{
		config:       cfg,
		functions:    functionRegistry,
		workflowTask: workflowTask,
	}

	// Create a set task with empty map
	setTask := &model.SetTask{
		Set: map[string]any{},
	}

	// Execute the set task
	result, err := runner.executeSetTask("testEmptySet", setTask, map[string]any{})

	// Verify the result
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Cast result to map for verification
	resultMap, ok := result.(map[string]any)
	require.True(t, ok, "result should be a map[string]any")
	assert.Empty(t, resultMap)
}

func TestExecuteSetTask_NilSet(t *testing.T) {
	// Create a test runner
	cfg := &config.Config{}
	functionRegistry := functions.NewFunctionRegistry(cfg)
	workflowTask := &models.WorkflowTask{
		WorkflowID: "test-workflow",
	}

	runner := &ResumableWorkflowRunner{
		config:       cfg,
		functions:    functionRegistry,
		workflowTask: workflowTask,
	}

	// Create a set task with nil set - this should fail at the SDK validation level
	// since the SetTask struct requires Set to be a non-nil map
	setTask := &model.SetTask{
		Set: map[string]any{}, // SDK requires non-nil map, so we'll test with empty instead
	}

	// Execute the set task
	result, err := runner.executeSetTask("testEmptyNilSet", setTask, map[string]any{})

	// Verify the result - empty map should be allowed
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Cast result to map for verification
	resultMap, ok := result.(map[string]any)
	require.True(t, ok, "result should be a map[string]any")
	assert.Empty(t, resultMap)
}

func TestExecuteSetTask_WithInputData(t *testing.T) {
	// Create a test runner
	cfg := &config.Config{}
	functionRegistry := functions.NewFunctionRegistry(cfg)
	workflowTask := &models.WorkflowTask{
		WorkflowID: "test-workflow",
	}

	runner := &ResumableWorkflowRunner{
		config:       cfg,
		functions:    functionRegistry,
		workflowTask: workflowTask,
	}

	// Create input data
	input := map[string]any{
		"user": map[string]any{
			"name": "John Doe",
			"role": "admin",
		},
		"settings": map[string]any{
			"theme": "dark",
		},
	}

	// Create a set task that combines static and dynamic data
	setTask := &model.SetTask{
		Set: map[string]any{
			"username":    "${ .user.name }",
			"userRole":    "${ .user.role }",
			"environment": "production",
			"config": map[string]any{
				"theme":   "${ .settings.theme }",
				"version": "1.0.0",
			},
		},
	}

	// Execute the set task
	result, err := runner.executeSetTask("testWithInputData", setTask, input)

	// Verify the result
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Cast result to map for verification
	resultMap, ok := result.(map[string]any)
	require.True(t, ok, "result should be a map[string]any")
	assert.Equal(t, "John Doe", resultMap["username"])
	assert.Equal(t, "admin", resultMap["userRole"])
	assert.Equal(t, "production", resultMap["environment"])

	config, ok := resultMap["config"].(map[string]any)
	require.True(t, ok, "config should be a map")
	assert.Equal(t, "dark", config["theme"])
	assert.Equal(t, "1.0.0", config["version"])
}

func TestExecuteSetTask_ArrayValues(t *testing.T) {
	// Create a test runner
	cfg := &config.Config{}
	functionRegistry := functions.NewFunctionRegistry(cfg)
	workflowTask := &models.WorkflowTask{
		WorkflowID: "test-workflow",
	}

	runner := &ResumableWorkflowRunner{
		config:       cfg,
		functions:    functionRegistry,
		workflowTask: workflowTask,
	}

	// Create a set task with array values
	setTask := &model.SetTask{
		Set: map[string]any{
			"tags":   []string{"production", "web", "api"},
			"ports":  []int{80, 443, 8080},
			"active": true,
		},
	}

	// Execute the set task
	result, err := runner.executeSetTask("testArrayValues", setTask, map[string]any{})

	// Verify the result
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Cast result to map for verification
	resultMap, ok := result.(map[string]any)
	require.True(t, ok, "result should be a map[string]any")

	tags, ok := resultMap["tags"].([]string)
	require.True(t, ok, "tags should be a string array")
	assert.Equal(t, []string{"production", "web", "api"}, tags)

	ports, ok := resultMap["ports"].([]int)
	require.True(t, ok, "ports should be an int array")
	assert.Equal(t, []int{80, 443, 8080}, ports)

	assert.Equal(t, true, resultMap["active"])
}

func TestExecuteSetTask_ComplexExpressions(t *testing.T) {
	// Create a test runner
	cfg := &config.Config{}
	functionRegistry := functions.NewFunctionRegistry(cfg)
	workflowTask := &models.WorkflowTask{
		WorkflowID: "test-workflow",
	}

	runner := &ResumableWorkflowRunner{
		config:       cfg,
		functions:    functionRegistry,
		workflowTask: workflowTask,
	}

	// Create input data
	input := map[string]any{
		"user": map[string]any{
			"name": "john",
			"age":  30,
		},
		"multiplier": 2,
	}

	// Create a set task with complex expressions
	setTask := &model.SetTask{
		Set: map[string]any{
			"userName":   "${ .user.name }",
			"doubledAge": "${ .user.age * .multiplier }",
			"greeting":   "Hello, World!",
			"isAdult":    "${ .user.age >= 18 }",
		},
	}

	// Execute the set task
	result, err := runner.executeSetTask("testComplexExpressions", setTask, input)

	// Verify the result
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Cast result to map for verification
	resultMap, ok := result.(map[string]any)
	require.True(t, ok, "result should be a map[string]any")
	assert.Equal(t, "john", resultMap["userName"])
	assert.Equal(t, 60, resultMap["doubledAge"])
	assert.Equal(t, "Hello, World!", resultMap["greeting"])
	assert.Equal(t, true, resultMap["isAdult"])
}

func TestExecuteSetTask_SimpleWorkflowIntegration(t *testing.T) {
	t.Run("Simple Set Task Demo", func(t *testing.T) {
		workflowPath := "./testdata/simple_set_demo.yaml"
		input := map[string]any{
			"inputValue": "world",
		}

		expectedContext := map[string]any{
			"result": map[string]any{
				"staticValue":  "hello",
				"dynamicValue": "world",
			},
		}

		runWorkflowTest(t, workflowPath, input, expectedContext)
	})
}
