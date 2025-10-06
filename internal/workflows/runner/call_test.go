package runner

import (
	"testing"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/functions"
)

// MockFunction implements the Function interface for testing
type MockFunction struct {
	name        string
	description string
	version     string
	lastCall    *model.CallFunction
	lastInput   any
	lastTask    *models.WorkflowTask
}

func NewMockFunction(name string) *MockFunction {
	return &MockFunction{
		name:        name,
		description: "Mock function for testing",
		version:     "1.0.0",
	}
}

func (m *MockFunction) GetName() string        { return m.name }
func (m *MockFunction) GetDescription() string { return m.description }
func (m *MockFunction) GetVersion() string     { return m.version }

func (m *MockFunction) GetRequiredParameters() []string {
	return []string{}
}

func (m *MockFunction) GetOptionalParameters() map[string]any {
	return map[string]any{}
}

func (m *MockFunction) ValidateRequest(
	workflowTask *models.WorkflowTask,
	call *model.CallFunction,
	input any,
) error {
	// Store the call for inspection
	m.lastCall = call
	m.lastInput = input
	m.lastTask = workflowTask
	return nil
}

func (m *MockFunction) Execute(
	workflowTask *models.WorkflowTask,
	call *model.CallFunction,
	input any,
) (any, error) {
	// Store the call for inspection
	m.lastCall = call
	m.lastInput = input
	m.lastTask = workflowTask
	return map[string]any{"success": true}, nil
}

func TestExecuteCallFunction_MessageInterpolation(t *testing.T) {
	// Create a test configuration
	cfg := &config.Config{}

	// Create function registry and register a mock function
	registry := functions.NewFunctionRegistry(cfg)
	mockFunction := NewMockFunction("test.mock")
	registry.RegisterFunction(mockFunction)

	// Create a workflow task with user context
	workflowTask := &models.WorkflowTask{
		WorkflowID: "test-workflow",
		Context: map[string]any{
			"user": map[string]any{
				"name":  "john.doe",
				"email": "john.doe@example.com",
			},
		},
		Input:  make(map[string]any),
		Output: make(map[string]any),
	}

	// Create a runner
	runner := &ResumableWorkflowRunner{
		workflowTask: workflowTask,
		functions:    registry,
		config:       cfg,
	}

	// Create a call function with the message expression that needs interpolation
	callFunc := &model.CallFunction{
		Call: "test.mock",
		With: map[string]any{
			"provider":  "slack",
			"to":        "C09DDUAVBK4",
			"message":   `${ "The user \($context.user.name) is requesting access." }`,
			"approvals": true,
		},
	}

	// Execute the function
	input := map[string]any{"someInput": "value"}
	result, err := runner.executeCallFunction("testTask", callFunc, input)

	if err != nil {
		t.Fatalf("executeCallFunction failed: %v", err)
	}

	// Verify the result
	if result == nil {
		t.Fatalf("executeCallFunction returned nil result")
	}

	// Check that the mock function was called
	if mockFunction.lastCall == nil {
		t.Fatalf("Mock function was not called")
	}

	// Verify that the message was interpolated correctly
	withParams := mockFunction.lastCall.With
	if withParams == nil {
		t.Fatalf("With parameters are nil")
	}

	messageValue, exists := withParams["message"]
	if !exists {
		t.Fatalf("Message parameter not found in with parameters")
	}

	actualMessage, ok := messageValue.(string)
	if !ok {
		t.Fatalf("Message parameter is not a string, got: %T", messageValue)
	}

	expectedMessage := "The user john.doe is requesting access."
	if actualMessage != expectedMessage {
		t.Errorf("Message interpolation failed in executeCallFunction. Got: %s, Expected: %s", actualMessage, expectedMessage)
	}

	// Verify other parameters were passed through correctly
	if provider := withParams["provider"]; provider != "slack" {
		t.Errorf("Provider parameter incorrect. Got: %v, Expected: slack", provider)
	}

	if to := withParams["to"]; to != "C09DDUAVBK4" {
		t.Errorf("To parameter incorrect. Got: %v, Expected: C09DDUAVBK4", to)
	}

	if approvals := withParams["approvals"]; approvals != true {
		t.Errorf("Approvals parameter incorrect. Got: %v, Expected: true", approvals)
	}

	t.Logf("executeCallFunction interpolation test successful: %s", actualMessage)
}

func TestExecuteCallFunction_MultipleExpressions(t *testing.T) {
	// Create a test configuration
	cfg := &config.Config{}

	// Create function registry and register a mock function
	registry := functions.NewFunctionRegistry(cfg)
	mockFunction := NewMockFunction("test.mock")
	registry.RegisterFunction(mockFunction)

	// Create a workflow task with user context
	workflowTask := &models.WorkflowTask{
		WorkflowID: "test-workflow",
		Context: map[string]any{
			"user": map[string]any{
				"name":  "jane.smith",
				"email": "jane.smith@company.com",
			},
			"role": map[string]any{
				"name": "admin",
			},
		},
		Input:  make(map[string]any),
		Output: make(map[string]any),
	}

	// Create a runner
	runner := &ResumableWorkflowRunner{
		workflowTask: workflowTask,
		functions:    registry,
		config:       cfg,
	}

	// Create a call function with multiple expressions
	callFunc := &model.CallFunction{
		Call: "test.mock",
		With: map[string]any{
			"greeting": `${ "Hello \($context.user.name)" }`,
			"role_msg": `${ "You have \($context.role.name) access" }`,
			"email":    `${ $context.user.email }`,
			"static":   "This should not change",
		},
	}

	// Execute the function
	input := map[string]any{}
	_, err := runner.executeCallFunction("testTask", callFunc, input)

	if err != nil {
		t.Fatalf("executeCallFunction failed: %v", err)
	}

	// Verify interpolations
	withParams := mockFunction.lastCall.With

	expectedValues := map[string]string{
		"greeting": "Hello jane.smith",
		"role_msg": "You have admin access",
		"email":    "jane.smith@company.com",
		"static":   "This should not change",
	}

	for key, expected := range expectedValues {
		actual, exists := withParams[key]
		if !exists {
			t.Errorf("Parameter %s not found", key)
			continue
		}

		actualStr, ok := actual.(string)
		if !ok {
			t.Errorf("Parameter %s is not a string, got: %T", key, actual)
			continue
		}

		if actualStr != expected {
			t.Errorf("Parameter %s interpolation failed. Got: %s, Expected: %s", key, actualStr, expected)
		}
	}

	t.Logf("Multiple expressions test successful")
}
