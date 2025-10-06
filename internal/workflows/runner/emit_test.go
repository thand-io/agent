package runner

import (
	"testing"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/stretchr/testify/assert"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/functions"
)

func TestExecuteEmitTask_NonTemporal(t *testing.T) {
	// Create a test runner without temporal context
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

	// Create an emit task
	emit := &model.EmitTask{
		Emit: model.EmitTaskConfiguration{
			Event: model.EmitEventDefinition{
				With: &model.EventProperties{
					Source: &model.URITemplateOrRuntimeExpr{
						Value: "https://example.com",
					},
					Type: "com.example.test",
				},
			},
		},
	}

	// Execute the emit task
	result, err := runner.executeEmitTask("testEmit", emit, map[string]any{"test": "data"})

	// Should return error for non-temporal workflow
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Equal(t, ErrorEmitUnsupported, err)
}

func TestCreateCloudEventFromEmit(t *testing.T) {
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

	// Create an emit task with required fields
	emit := &model.EmitTask{
		Emit: model.EmitTaskConfiguration{
			Event: model.EmitEventDefinition{
				With: &model.EventProperties{
					ID:   "test-event-123",
					Type: "com.example.test.v1",
					Source: &model.URITemplateOrRuntimeExpr{
						Value: "https://example.com/test",
					},
					Subject:         "test-subject",
					DataContentType: "application/json",
					Additional: map[string]any{
						"data": map[string]string{
							"message": "Hello World",
						},
						"customExtension": "customValue",
					},
				},
			},
		},
	}

	input := map[string]any{"inputKey": "inputValue"}

	// Create cloud event
	event, err := runner.createCloudEventFromEmit(emit, input)

	// Verify no error
	assert.NoError(t, err)
	assert.NotNil(t, event)

	// Verify required fields
	assert.Equal(t, "test-event-123", event.ID())
	assert.Equal(t, "com.example.test.v1", event.Type())
	assert.Equal(t, "https://example.com/test", event.Source())
	assert.Equal(t, "test-subject", event.Subject())
	assert.Equal(t, "application/json", event.DataContentType())

	// Verify custom extension (keys are normalized to lowercase by cloudevents)
	extensions := event.Extensions()
	t.Logf("Extensions: %+v", extensions)
	assert.Equal(t, "customValue", event.Extensions()["customextension"])

	// Verify data
	var eventData map[string]string
	err = event.DataAs(&eventData)
	assert.NoError(t, err)
	assert.Equal(t, "Hello World", eventData["message"])
}

func TestCreateCloudEventFromEmit_MissingRequiredFields(t *testing.T) {
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

	// Test missing source
	emit := &model.EmitTask{
		Emit: model.EmitTaskConfiguration{
			Event: model.EmitEventDefinition{
				With: &model.EventProperties{
					Type: "com.example.test",
				},
			},
		},
	}

	_, err := runner.createCloudEventFromEmit(emit, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "source is required")

	// Test missing type
	emit.Emit.Event.With.Source = &model.URITemplateOrRuntimeExpr{
		Value: "https://example.com",
	}
	emit.Emit.Event.With.Type = ""

	_, err = runner.createCloudEventFromEmit(emit, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "type is required")
}
