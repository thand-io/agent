package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/serverlessworkflow/sdk-go/v3/parser"
	"github.com/stretchr/testify/assert"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/functions"
)

func NewDefaultRunner(workflow *model.Workflow) (*ResumableWorkflowRunner, error) {

	config := config.DefaultConfig()

	// create functions registry
	functions := functions.NewFunctionRegistry(config)

	wkflw, err := models.NewWorkflowContext(&models.Workflow{
		Name:        workflow.Document.Name,
		Description: workflow.Document.Summary,
		Workflow:    workflow,
		Enabled:     true,
	})

	if err != nil {
		return nil, err
	}

	return NewResumableRunner(config, functions, wkflw), nil
}

// runWorkflowTest is a reusable test function for workflows
func runWorkflowTest(t *testing.T, workflowPath string, input, expectedOutput map[string]any) {
	// Run the workflow
	output, err := runWorkflow(t, workflowPath, input, expectedOutput)
	assert.NoError(t, err)

	assertWorkflowRun(t, expectedOutput, output)
}

func runWorkflowWithErr(t *testing.T, workflowPath string, input, expectedOutput map[string]any, assertErr func(error)) {
	output, err := runWorkflow(t, workflowPath, input, expectedOutput)
	assert.Error(t, err)
	assertErr(err)
	assertWorkflowRun(t, expectedOutput, output)
}

func runWorkflow(t *testing.T, workflowPath string, input, expectedOutput map[string]any) (output any, err error) {
	// Read the workflow YAML from the testdata directory
	yamlBytes, err := os.ReadFile(filepath.Clean(workflowPath))
	assert.NoError(t, err, "Failed to read workflow YAML file")

	// Parse the YAML workflow
	workflow, err := parser.FromYAMLSource(yamlBytes)
	assert.NoError(t, err, "Failed to parse workflow YAML")

	// Initialize the workflow runner
	runner, err := NewDefaultRunner(workflow)
	assert.NoError(t, err)

	// Run the workflow
	output, err = runner.Run(input)
	return output, err
}

func assertWorkflowRun(t *testing.T, expectedOutput map[string]any, output any) {
	if expectedOutput == nil {
		assert.Nil(t, output, "Expected nil Workflow run output")
	} else {
		assert.Equal(t, expectedOutput, output, "Workflow output mismatch")
	}
}

// TestWorkflowRunner_Run_YAML validates multiple workflows
func TestWorkflowRunner_Run_YAML(t *testing.T) {
	// Workflow 1: Chained Set Tasks
	t.Run("Chained Set Tasks", func(t *testing.T) {
		workflowPath := "./testdata/chained_set_tasks.yaml"
		input := map[string]any{}
		expectedOutput := map[string]any{
			"tripled": float64(60),
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})

	// Workflow 2: Concatenating Strings
	t.Run("Concatenating Strings", func(t *testing.T) {
		workflowPath := "./testdata/concatenating_strings.yaml"
		input := map[string]any{}
		expectedOutput := map[string]any{
			"fullName": "John Doe",
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})

	// Workflow 3: Conditional Logic
	t.Run("Conditional Logic", func(t *testing.T) {
		workflowPath := "./testdata/conditional_logic.yaml"
		input := map[string]any{}
		expectedOutput := map[string]any{
			"weather": "hot",
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})

	t.Run("Conditional Logic", func(t *testing.T) {
		workflowPath := "./testdata/sequential_set_colors.yaml"
		// Define the input and expected output
		input := map[string]any{}
		expectedOutput := map[string]any{
			"resultColors": []any{"red", "green", "blue"},
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})
	t.Run("input From", func(t *testing.T) {
		workflowPath := "./testdata/sequential_set_colors_output_as.yaml"
		// Define the input and expected output
		expectedOutput := map[string]any{
			"result": []any{"red", "green", "blue"},
		}
		runWorkflowTest(t, workflowPath, nil, expectedOutput)
	})
	t.Run("input From", func(t *testing.T) {
		workflowPath := "./testdata/conditional_logic_input_from.yaml"
		// Define the input and expected output
		input := map[string]any{
			"localWeather": map[string]any{
				"temperature": 34,
			},
		}
		expectedOutput := map[string]any{
			"weather": "hot",
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})

	// Star Wars Homeworld workflow test
	t.Run("Star Wars Homeworld", func(t *testing.T) {
		workflowPath := "./testdata/star-wars-homeworld.yaml"
		// Use Luke Skywalker's character ID (1) as test input
		input := map[string]any{
			"id": 1,
		}

		// Run the workflow and check for successful execution
		// Note: This test requires internet connectivity and SWAPI availability
		yamlBytes, err := os.ReadFile(filepath.Clean(workflowPath))
		assert.NoError(t, err, "Failed to read workflow YAML file")

		workflow, err := parser.FromYAMLSource(yamlBytes)
		assert.NoError(t, err, "Failed to parse workflow YAML")

		runner, err := NewDefaultRunner(workflow)
		assert.NoError(t, err)

		output, err := runner.Run(input)

		// The test should succeed if the API is available
		if err != nil {
			// If there's an error, it might be due to network connectivity
			// or API unavailability - log it but don't fail the test hard
			t.Logf("Star Wars API might be unavailable: %v", err)
			t.Skip("Skipping Star Wars homeworld test due to API unavailability")
			return
		}

		// Verify that we got some output
		assert.NotNil(t, output, "Expected non-nil output from Star Wars homeworld workflow")

		// If output is a map, verify it contains expected structure
		if outputMap, ok := output.(map[string]any); ok {
			// The workflow should return the homeworld data
			assert.Contains(t, outputMap, "name", "Expected name in HTTP response")
			assert.Contains(t, outputMap, "created", "Expected created in HTTP response")

		}
	})
}

func TestWorkflowRunner_Run_YAML_SetTasksWithExportApprovals(t *testing.T) {
	t.Run("Set Tasks with Export Approvals", func(t *testing.T) {
		workflowPath := "./testdata/set_tasks_with_export_approvals.yaml"
		input := map[string]any{}
		expectedOutput := map[string]any{
			"approvals": []any{
				map[string]any{"approved": true},
				map[string]any{"approved": true},
			},
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})
}

func TestWorkflowRunner_Run_YAML_WithSchemaValidation(t *testing.T) {
	// Workflow 1: Workflow input Schema Validation
	t.Run("Workflow input Schema Validation - Valid input", func(t *testing.T) {
		workflowPath := "./testdata/workflow_input_schema.yaml"
		input := map[string]any{
			"key": "value",
		}
		expectedOutput := map[string]any{
			"outputKey": "value",
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})

	t.Run("Workflow input Schema Validation - Invalid input", func(t *testing.T) {
		workflowPath := "./testdata/workflow_input_schema.yaml"
		input := map[string]any{
			"wrongKey": "value",
		}
		yamlBytes, err := os.ReadFile(filepath.Clean(workflowPath))
		assert.NoError(t, err, "Failed to read workflow YAML file")
		workflow, err := parser.FromYAMLSource(yamlBytes)
		assert.NoError(t, err, "Failed to parse workflow YAML")
		runner, err := NewDefaultRunner(workflow)
		assert.NoError(t, err)
		_, err = runner.Run(input)
		assert.Error(t, err, "Expected validation error for invalid input")
		assert.Contains(t, err.Error(), "JSON schema validation failed")
	})

	// Workflow 2: Task input Schema Validation
	t.Run("Task input Schema Validation", func(t *testing.T) {
		workflowPath := "./testdata/task_input_schema.yaml"
		input := map[string]any{
			"taskInputKey": 42,
		}
		expectedOutput := map[string]any{
			"taskOutputKey": 84,
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})

	t.Run("Task input Schema Validation - Invalid input", func(t *testing.T) {
		workflowPath := "./testdata/task_input_schema.yaml"
		input := map[string]any{
			"taskInputKey": "invalidValue",
		}
		yamlBytes, err := os.ReadFile(filepath.Clean(workflowPath))
		assert.NoError(t, err, "Failed to read workflow YAML file")
		workflow, err := parser.FromYAMLSource(yamlBytes)
		assert.NoError(t, err, "Failed to parse workflow YAML")
		runner, err := NewDefaultRunner(workflow)
		assert.NoError(t, err)
		_, err = runner.Run(input)
		assert.Error(t, err, "Expected validation error for invalid task input")
		assert.Contains(t, err.Error(), "JSON schema validation failed")
	})

	// Workflow 3: Task output Schema Validation
	t.Run("Task output Schema Validation", func(t *testing.T) {
		workflowPath := "./testdata/task_output_schema.yaml"
		input := map[string]any{
			"taskInputKey": "value",
		}
		expectedOutput := map[string]any{
			"finalOutputKey": "resultValue",
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})

	t.Run("Task output Schema Validation - Invalid output", func(t *testing.T) {
		workflowPath := "./testdata/task_output_schema_with_dynamic_value.yaml"
		input := map[string]any{
			"taskInputKey": 123, // Invalid value (not a string)
		}
		yamlBytes, err := os.ReadFile(filepath.Clean(workflowPath))
		assert.NoError(t, err, "Failed to read workflow YAML file")
		workflow, err := parser.FromYAMLSource(yamlBytes)
		assert.NoError(t, err, "Failed to parse workflow YAML")
		runner, err := NewDefaultRunner(workflow)
		assert.NoError(t, err)
		_, err = runner.Run(input)
		assert.Error(t, err, "Expected validation error for invalid task output")
		assert.Contains(t, err.Error(), "JSON schema validation failed")
	})

	t.Run("Task output Schema Validation - Valid output", func(t *testing.T) {
		workflowPath := "./testdata/task_output_schema_with_dynamic_value.yaml"
		input := map[string]any{
			"taskInputKey": "validValue", // Valid value
		}
		expectedOutput := map[string]any{
			"finalOutputKey": "validValue",
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})

	// Workflow 4: Task Export Schema Validation
	t.Run("Task Export Schema Validation", func(t *testing.T) {
		workflowPath := "./testdata/task_export_schema.yaml"
		input := map[string]any{
			"key": "value",
		}
		expectedOutput := map[string]any{
			"exportedKey": "value",
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})
}

func TestWorkflowRunner_Run_YAML_ControlFlow(t *testing.T) {
	t.Run("Set Tasks with Then Directive", func(t *testing.T) {
		workflowPath := "./testdata/set_tasks_with_then.yaml"
		input := map[string]any{}
		expectedOutput := map[string]any{
			"result": float64(90),
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})

	t.Run("Set Tasks with Termination", func(t *testing.T) {
		workflowPath := "./testdata/set_tasks_with_termination.yaml"
		input := map[string]any{}
		expectedOutput := map[string]any{
			"finalValue": float64(20),
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})

	t.Run("Set Tasks with Invalid Then Reference", func(t *testing.T) {
		workflowPath := "./testdata/set_tasks_invalid_then.yaml"
		input := map[string]any{}
		expectedOutput := map[string]any{
			"partialResult": float64(15),
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})
}

func TestWorkflowRunner_Run_YAML_RaiseTasks(t *testing.T) {
	// TODO: add $workflow context to the expr processing
	t.Run("Raise Inline Error", func(t *testing.T) {
		runWorkflowWithErr(t, "./testdata/raise_inline.yaml", nil, nil, func(err error) {
			assert.Equal(t, model.ErrorTypeValidation, model.AsError(err).Type.String())
			assert.Equal(t, "Invalid input provided to workflow raise-inline", model.AsError(err).Detail.String())
		})
	})

	t.Run("Raise Referenced Error", func(t *testing.T) {
		runWorkflowWithErr(t, "./testdata/raise_reusable.yaml", nil, nil,
			func(err error) {
				assert.Equal(t, model.ErrorTypeAuthentication, model.AsError(err).Type.String())
			})
	})

	t.Run("Raise Error with Dynamic Detail", func(t *testing.T) {
		input := map[string]any{
			"reason": "User token expired",
		}
		runWorkflowWithErr(t, "./testdata/raise_error_with_input.yaml", input, nil,
			func(err error) {
				assert.Equal(t, model.ErrorTypeAuthentication, model.AsError(err).Type.String())
				assert.Equal(t, "User authentication failed: User token expired", model.AsError(err).Detail.String())
			})
	})

	t.Run("Raise Undefined Error Reference", func(t *testing.T) {
		runWorkflowWithErr(t, "./testdata/raise_undefined_reference.yaml", nil, nil,
			func(err error) {
				assert.Equal(t, model.ErrorTypeValidation, model.AsError(err).Type.String())
			})
	})
}

func TestWorkflowRunner_Run_YAML_RaiseTasks_ControlFlow(t *testing.T) {
	t.Run("Raise Error with Conditional Logic", func(t *testing.T) {
		input := map[string]any{
			"user": map[string]any{
				"age": 16,
			},
		}
		runWorkflowWithErr(t, "./testdata/raise_conditional.yaml", input, nil,
			func(err error) {
				assert.Equal(t, model.ErrorTypeAuthorization, model.AsError(err).Type.String())
				assert.Equal(t, "User is under the required age", model.AsError(err).Detail.String())
			})
	})
}

func TestForTaskRunner_Run(t *testing.T) {
	t.Run("Simple For with Colors", func(t *testing.T) {
		workflowPath := "./testdata/for_colors.yaml"
		input := map[string]any{
			"colors": []string{"red", "green", "blue"},
		}
		expectedOutput := map[string]any{
			"processed": map[string]any{
				"colors":  []any{"red", "green", "blue"},
				"indexes": []any{0, 1, 2},
			},
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})

	t.Run("SUM Numbers", func(t *testing.T) {
		workflowPath := "./testdata/for_sum_numbers.yaml"
		input := map[string]any{
			"numbers": []int32{2, 3, 4},
		}
		expectedOutput := map[string]any{
			"result": any(9),
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})

	t.Run("For Nested Loops", func(t *testing.T) {
		workflowPath := "./testdata/for_nested_loops.yaml"
		input := map[string]any{
			"fruits": []any{"apple", "banana"},
			"colors": []any{"red", "green"},
		}
		expectedOutput := map[string]any{
			"matrix": []any{
				[]any{"apple", "red"},
				[]any{"apple", "green"},
				[]any{"banana", "red"},
				[]any{"banana", "green"},
			},
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})

}

func TestSwitchTaskRunner_Run(t *testing.T) {
	t.Run("Color is red", func(t *testing.T) {
		workflowPath := "./testdata/switch_match.yaml"
		input := map[string]any{
			"color": "red",
		}
		expectedOutput := map[string]any{
			"colors": []any{"red"},
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})

	t.Run("Color is green", func(t *testing.T) {
		workflowPath := "./testdata/switch_match.yaml"
		input := map[string]any{
			"color": "green",
		}
		expectedOutput := map[string]any{
			"colors": []any{"green"},
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})

	t.Run("Color is blue", func(t *testing.T) {
		workflowPath := "./testdata/switch_match.yaml"
		input := map[string]any{
			"color": "blue",
		}
		expectedOutput := map[string]any{
			"colors": []any{"blue"},
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})
}

func TestSwitchTaskRunner_DefaultCase(t *testing.T) {
	t.Run("Color is unknown, should match default", func(t *testing.T) {
		workflowPath := "./testdata/switch_with_default.yaml"
		input := map[string]any{
			"color": "yellow",
		}
		expectedOutput := map[string]any{
			"colors": []any{"default"},
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})
}

func TestForkSimple_NoCompete(t *testing.T) {
	t.Run("Create a color array", func(t *testing.T) {
		workflowPath := "./testdata/fork_simple.yaml"
		input := map[string]any{}
		expectedOutput := map[string]any{
			"colors": []any{"red", "blue"},
		}
		runWorkflowTest(t, workflowPath, input, expectedOutput)
	})
}
