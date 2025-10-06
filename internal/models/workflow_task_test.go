package models

import (
	"encoding/json"
	"testing"

	"github.com/serverlessworkflow/sdk-go/v3/model"
)

// TestWorkflowTask_MessageInterpolation tests various message interpolation scenarios
//
// IMPORTANT: Regarding expression formats without encapsulating parentheses:
//
// The format "The user ${.context.user.email} has requested access" (without proper ${ } wrapping)
// is NOT supported by the Serverless Workflow specification.
//
// The specification requires expressions to be in "strict mode" by default, which means:
// 1. All expressions MUST be fully enclosed within ${ } brackets
// 2. The content inside ${ } must be valid jq syntax
//
// Valid formats:
//
//	✅ ${ "The user \(.context.user.email) has requested access" }
//	✅ ${ "The user \(.user.email) has requested access" }
//	✅ ${ .context.user.email }
//
// Invalid formats that will NOT be interpolated:
//
//	❌ "The user ${.context.user.email} has requested access"
//	❌ The user ${.context.user.email} has requested access
//	❌ ${.context.user.email}
//
// If an invalid format is used, the string will be passed through unchanged.
func TestWorkflowTask_MessageInterpolation(t *testing.T) {
	tests := []struct {
		name           string
		setupUser      *User
		setupContext   map[string]any
		messageExpr    string
		expectedResult string
		wantErr        bool
	}{
		{
			name: "interpolate user name in message",
			setupUser: &User{
				Name:  "john.doe",
				Email: "john.doe@example.com",
			},
			setupContext: map[string]any{
				"user": map[string]any{
					"name":  "john.doe",
					"email": "john.doe@example.com",
				},
			},
			messageExpr:    `${ "The user \(.user.name) is requesting access." }`,
			expectedResult: "The user john.doe is requesting access.",
			wantErr:        false,
		},
		{
			name: "interpolate user name with context variable",
			setupUser: &User{
				Name:  "jane.smith",
				Email: "jane.smith@company.com",
			},
			setupContext: map[string]any{
				"user": map[string]any{
					"name":  "jane.smith",
					"email": "jane.smith@company.com",
				},
			},
			messageExpr:    `${ "The user \($context.user.name) is requesting access." }`,
			expectedResult: "The user jane.smith is requesting access.",
			wantErr:        false,
		},
		{
			name: "interpolate with additional context",
			setupUser: &User{
				Name:  "alice.williams",
				Email: "alice@test.com",
			},
			setupContext: map[string]any{
				"user": map[string]any{
					"name":  "alice.williams",
					"email": "alice@test.com",
				},
				"role": "admin",
			},
			messageExpr:    `${ "User \($context.user.name) is requesting \($context.role) access." }`,
			expectedResult: "User alice.williams is requesting admin access.",
			wantErr:        false,
		},
		{
			name: "simple string interpolation",
			setupUser: &User{
				Name:  "bob.jones",
				Email: "bob@example.org",
			},
			setupContext: map[string]any{
				"user": map[string]any{
					"name":  "bob.jones",
					"email": "bob@example.org",
				},
			},
			messageExpr:    `${ .user.name + " needs access" }`,
			expectedResult: "bob.jones needs access",
			wantErr:        false,
		},
		{
			name: "complex message with conditional",
			setupUser: &User{
				Name:  "charlie.brown",
				Email: "charlie@test.io",
			},
			setupContext: map[string]any{
				"user": map[string]any{
					"name":  "charlie.brown",
					"email": "charlie@test.io",
				},
				"urgent": true,
			},
			messageExpr:    `${ "Access request from \($context.user.name)" + (if $context.urgent then " (URGENT)" else "" end) }`,
			expectedResult: "Access request from charlie.brown (URGENT)",
			wantErr:        false,
		},
		{
			name: "non-expression string should pass through",
			setupUser: &User{
				Name:  "david.miller",
				Email: "david@company.com",
			},
			setupContext: map[string]any{
				"user": map[string]any{
					"name":  "david.miller",
					"email": "david@company.com",
				},
			},
			messageExpr:    "Static message without interpolation",
			expectedResult: "Static message without interpolation",
			wantErr:        false,
		},
		{
			name:      "missing user context should handle gracefully",
			setupUser: nil,
			setupContext: map[string]any{
				"role": "user",
			},
			messageExpr:    `${ "User \($context.user.name // "unknown") requesting access" }`,
			expectedResult: "User unknown requesting access",
			wantErr:        false,
		},
		{
			name: "interpolate with direct context access - correct format",
			setupUser: &User{
				Name:  "test.user",
				Email: "test.user@example.com",
			},
			setupContext: map[string]any{
				"user": map[string]any{
					"name":  "test.user",
					"email": "test.user@example.com",
				},
			},
			messageExpr:    `${ "The user \(.user.email) has requested access" }`,
			expectedResult: "The user test.user@example.com has requested access",
			wantErr:        false,
		},
		{
			name: "invalid format without proper encapsulation should not interpolate",
			setupUser: &User{
				Name:  "test.user",
				Email: "test.user@example.com",
			},
			setupContext: map[string]any{
				"user": map[string]any{
					"name":  "test.user",
					"email": "test.user@example.com",
				},
			},
			messageExpr:    `The user ${.context.user.email} has requested access`,
			expectedResult: "The user ${.context.user.email} has requested access", // Should pass through unchanged
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new workflow task
			workflowTask := &WorkflowTask{
				WorkflowID: "test-workflow",
				Context:    tt.setupContext,
				Input:      make(map[string]any),
				Output:     make(map[string]any),
			}

			// Set up user if provided
			if tt.setupUser != nil {
				workflowTask.SetUser(tt.setupUser)
			}

			// Create a mock call function with the message
			callWith := map[string]any{
				"provider":  "slack",
				"to":        "#test-channel",
				"message":   tt.messageExpr,
				"approvals": true,
			}

			// Test the expression evaluation on the message field
			result, err := workflowTask.TraverseAndEvaluate(callWith, workflowTask.GetInstanceCtx())

			if (err != nil) != tt.wantErr {
				t.Errorf("TraverseAndEvaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return // Expected error case
			}

			// Extract the evaluated message
			resultMap, ok := result.(map[string]any)
			if !ok {
				t.Errorf("Expected result to be a map, got %T", result)
				return
			}

			actualMessage, exists := resultMap["message"]
			if !exists {
				t.Errorf("Expected 'message' field in result")
				return
			}

			actualMessageStr, ok := actualMessage.(string)
			if !ok {
				t.Errorf("Expected message to be string, got %T", actualMessage)
				return
			}

			if actualMessageStr != tt.expectedResult {
				t.Errorf("TraverseAndEvaluate() message = %v, want %v", actualMessageStr, tt.expectedResult)
			}
		})
	}
}

func TestWorkflowTask_ThandNotifyCallFunctionInterpolation(t *testing.T) {
	tests := []struct {
		name         string
		setupUser    *User
		setupContext map[string]any
		callFunction *model.CallFunction
		expectedWith map[string]any
		wantErr      bool
	}{
		{
			name: "thand.notify with user interpolation",
			setupUser: &User{
				Name:  "test.user",
				Email: "test@example.com",
			},
			setupContext: map[string]any{
				"user": map[string]any{
					"name":  "test.user",
					"email": "test@example.com",
				},
			},
			callFunction: &model.CallFunction{
				Call: "thand.notify",
				With: map[string]any{
					"provider":  "slack",
					"to":        "C09DDUAVBK4",
					"message":   `${ "The user \($context.user.name) is requesting access." }`,
					"approvals": true,
				},
			},
			expectedWith: map[string]any{
				"provider":  "slack",
				"to":        "C09DDUAVBK4",
				"message":   "The user test.user is requesting access.",
				"approvals": true,
			},
			wantErr: false,
		},
		{
			name: "thand.notify matching workflow example format",
			setupUser: &User{
				Name:  "admin.user",
				Email: "admin@company.com",
			},
			setupContext: map[string]any{
				"user": map[string]any{
					"name":  "admin.user",
					"email": "admin@company.com",
				},
				"duration": "1h",
			},
			callFunction: &model.CallFunction{
				Call: "thand.notify",
				With: map[string]any{
					"provider":  "slack",
					"to":        "C09DDUAVBK4",
					"message":   `${ "The user \($context.user.name) is requesting access." }`,
					"approvals": true,
				},
			},
			expectedWith: map[string]any{
				"provider":  "slack",
				"to":        "C09DDUAVBK4",
				"message":   "The user admin.user is requesting access.",
				"approvals": true,
			},
			wantErr: false,
		},
		{
			name: "thand.notify with complex message",
			setupUser: &User{
				Name:  "developer",
				Email: "dev@startup.io",
			},
			setupContext: map[string]any{
				"user": map[string]any{
					"name":  "developer",
					"email": "dev@startup.io",
				},
				"role": map[string]any{
					"name": "AWS-Production-ReadOnly",
				},
				"duration": "2h",
			},
			callFunction: &model.CallFunction{
				Call: "thand.notify",
				With: map[string]any{
					"provider":  "slack",
					"to":        "#access-requests",
					"message":   `${ "User \($context.user.name) is requesting \($context.role.name) access for \($context.duration)" }`,
					"approvals": true,
				},
			},
			expectedWith: map[string]any{
				"provider":  "slack",
				"to":        "#access-requests",
				"message":   "User developer is requesting AWS-Production-ReadOnly access for 2h",
				"approvals": true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a workflow task
			workflowTask := &WorkflowTask{
				WorkflowID: "test-workflow",
				Context:    tt.setupContext,
				Input:      make(map[string]any),
				Output:     make(map[string]any),
			}

			// Set up user if provided
			if tt.setupUser != nil {
				workflowTask.SetUser(tt.setupUser)
			}

			// Evaluate the call function's "with" parameters
			result, err := workflowTask.TraverseAndEvaluate(tt.callFunction.With, workflowTask.GetInstanceCtx())

			if (err != nil) != tt.wantErr {
				t.Errorf("TraverseAndEvaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return // Expected error case
			}

			// Convert result to map for comparison
			resultMap, ok := result.(map[string]any)
			if !ok {
				t.Errorf("Expected result to be a map, got %T", result)
				return
			}

			// Compare each expected field
			for key, expectedValue := range tt.expectedWith {
				actualValue, exists := resultMap[key]
				if !exists {
					t.Errorf("Expected key '%s' not found in result", key)
					continue
				}

				if actualValue != expectedValue {
					t.Errorf("For key '%s': got %v, want %v", key, actualValue, expectedValue)
				}
			}

			// Ensure no extra keys in result
			for key := range resultMap {
				if _, exists := tt.expectedWith[key]; !exists {
					t.Errorf("Unexpected key '%s' found in result with value %v", key, resultMap[key])
				}
			}
		})
	}
}

func TestWorkflowTask_ExpressionEvaluationEdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		setupContext map[string]any
		expression   string
		expected     any
		wantErr      bool
	}{
		{
			name: "nested user object access",
			setupContext: map[string]any{
				"user": map[string]any{
					"profile": map[string]any{
						"firstName": "John",
						"lastName":  "Doe",
					},
				},
			},
			expression: `${ "\($context.user.profile.firstName) \($context.user.profile.lastName)" }`,
			expected:   "John Doe",
			wantErr:    false,
		},
		{
			name: "array access in user context",
			setupContext: map[string]any{
				"user": map[string]any{
					"roles": []any{"admin", "user", "developer"},
				},
			},
			expression: `${ "Primary role: \($context.user.roles[0])" }`,
			expected:   "Primary role: admin",
			wantErr:    false,
		},
		{
			name: "null coalescing for missing user",
			setupContext: map[string]any{
				"role": "guest",
			},
			expression: `${ "User: \($context.user.name // "anonymous")" }`,
			expected:   "User: anonymous",
			wantErr:    false,
		},
		{
			name: "conditional expression with user context",
			setupContext: map[string]any{
				"user": map[string]any{
					"name":    "admin",
					"isAdmin": true,
				},
			},
			expression: `${ if $context.user.isAdmin then "Admin user \($context.user.name)" else "Regular user \($context.user.name)" end }`,
			expected:   "Admin user admin",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflowTask := &WorkflowTask{
				WorkflowID: "test-workflow",
				Context:    tt.setupContext,
				Input:      make(map[string]any),
				Output:     make(map[string]any),
			}

			result, err := workflowTask.TraverseAndEvaluate(tt.expression, workflowTask.GetInstanceCtx())

			if (err != nil) != tt.wantErr {
				t.Errorf("TraverseAndEvaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			if result != tt.expected {
				t.Errorf("TraverseAndEvaluate() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestWorkflowTask_WorkflowExampleCompatibility tests the exact format from the workflow example
func TestWorkflowTask_WorkflowExampleCompatibility(t *testing.T) {
	// This test verifies the exact message format from the attached workflow example
	workflowTask := &WorkflowTask{
		WorkflowID: "slack_approval",
		Context: map[string]any{
			"user": map[string]any{
				"name": "john.doe",
			},
		},
		Input:  make(map[string]any),
		Output: make(map[string]any),
	}

	// Test the exact expression from the workflow example
	notifyCall := map[string]any{
		"provider":  "slack",
		"to":        "C09DDUAVBK4",
		"message":   `${ "The user \($context.user.name) is requesting access." }`,
		"approvals": true,
	}

	result, err := workflowTask.TraverseAndEvaluate(notifyCall, workflowTask.GetInstanceCtx())
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resultMap := result.(map[string]any)
	actualMessage := resultMap["message"].(string)
	expectedMessage := "The user john.doe is requesting access."

	if actualMessage != expectedMessage {
		t.Errorf("Message interpolation failed. Got: %s, Expected: %s", actualMessage, expectedMessage)
	}

	// Also verify the JSON serialization works correctly
	jsonData, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal result to JSON: %v", err)
	}

	var unmarshalledResult map[string]any
	err = json.Unmarshal(jsonData, &unmarshalledResult)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if unmarshalledResult["message"] != expectedMessage {
		t.Errorf("JSON round-trip failed. Got: %s, Expected: %s", unmarshalledResult["message"], expectedMessage)
	}
}
