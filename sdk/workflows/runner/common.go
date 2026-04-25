package runner

import (
	"errors"
	"time"

	swctx "github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

var ErrorAwaitSignal = errors.New(string(swctx.PendingStatus))

var DefaultRetryPolicy = &temporal.RetryPolicy{
	InitialInterval:    time.Second,
	BackoffCoefficient: 2.0,
	MaximumInterval:    time.Minute,
	MaximumAttempts:    5,
}

var CriticalPathRetryPolicy = &temporal.RetryPolicy{
	InitialInterval:    time.Second,
	BackoffCoefficient: 2.0,
	MaximumInterval:    time.Minute,
	MaximumAttempts:    0, // infinite retries for critical path tasks
}

// WorkflowTimeNow returns workflow.Now when running inside a Temporal workflow
// coroutine (ensuring deterministic replay), and falls back to time.Now otherwise.
func WorkflowTimeNow(wt sdkWorkflowsModel.WorkflowTaskSupport) time.Time {
	if wt.HasTemporalContext() {
		return workflow.Now(wt.GetTemporalContext()).UTC()
	}
	return time.Now().UTC()
}
