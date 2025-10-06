package runner

import (
	"errors"
	"testing"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/functions"
)

func TestExecuteTryTask_SuccessfulTryBlock(t *testing.T) {
	t.Skip("Integration test - skipping for now due to complex setup requirements")
}

func TestExecuteTryTask_FailingTryBlockWithCatch(t *testing.T) {
	t.Skip("Integration test - skipping for now due to complex setup requirements")
}

func TestErrorMatchesFilter(t *testing.T) {
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

	tests := []struct {
		name     string
		error    error
		filter   *model.ErrorFilter
		expected bool
	}{
		{
			name: "match by type",
			error: &model.Error{
				Type:   model.NewUriTemplate(model.ErrorTypeRuntime),
				Status: 500,
			},
			filter: &model.ErrorFilter{
				Type: model.ErrorTypeRuntime,
			},
			expected: true,
		},
		{
			name: "match by status",
			error: &model.Error{
				Type:   model.NewUriTemplate(model.ErrorTypeRuntime),
				Status: 503,
			},
			filter: &model.ErrorFilter{
				Status: 503,
			},
			expected: true,
		},
		{
			name: "no match",
			error: &model.Error{
				Type:   model.NewUriTemplate(model.ErrorTypeRuntime),
				Status: 500,
			},
			filter: &model.ErrorFilter{
				Status: 404,
			},
			expected: false,
		},
		{
			name:  "regular error",
			error: errors.New("regular error"),
			filter: &model.ErrorFilter{
				Type: model.ErrorTypeRuntime,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runner.errorMatchesFilter(tt.error, tt.filter)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateRetryDelay(t *testing.T) {
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

	tests := []struct {
		name        string
		retryPolicy *model.RetryPolicy
		attempt     int
		minDelay    int64 // in milliseconds
		maxDelay    int64 // in milliseconds
	}{
		{
			name: "constant backoff",
			retryPolicy: &model.RetryPolicy{
				Delay: &model.Duration{
					// Assuming 1 second delay
				},
				Backoff: &model.RetryBackoff{
					Constant: &model.BackoffDefinition{},
				},
			},
			attempt:  3,
			minDelay: 950,  // Allow some tolerance
			maxDelay: 1050, // Allow some tolerance
		},
		{
			name: "exponential backoff",
			retryPolicy: &model.RetryPolicy{
				Delay: &model.Duration{
					// Assuming 1 second delay
				},
				Backoff: &model.RetryBackoff{
					Exponential: &model.BackoffDefinition{},
				},
			},
			attempt:  3,
			minDelay: 3900, // 4 seconds * attempt (approximately)
			maxDelay: 4100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := runner.calculateRetryDelay(tt.retryPolicy, tt.attempt)
			delayMs := delay.Milliseconds()

			assert.GreaterOrEqual(t, delayMs, tt.minDelay, "Delay should be at least minimum expected")
			assert.LessOrEqual(t, delayMs, tt.maxDelay, "Delay should not exceed maximum expected")
		})
	}
}
