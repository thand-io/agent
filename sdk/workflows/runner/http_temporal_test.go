package runner

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/stretchr/testify/require"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
	"github.com/thand-io/agent/sdk/workflows/config"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

func TestExecuteHttpFunction_TemporalPathRetriesActivity(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	var attempts int32

	env.RegisterActivityWithOptions(
		func(ctx context.Context, httpCall model.HTTPArguments, finalURL string) (any, error) {
			_ = activity.GetInfo(ctx)

			if finalURL != "https://example.com/test" {
				return nil, errors.New("unexpected url")
			}

			currentAttempt := atomic.AddInt32(&attempts, 1)
			if currentAttempt < 3 {
				return nil, errors.New("transient http failure")
			}

			return map[string]any{"status": "ok"}, nil
		},
		activity.RegisterOptions{Name: sdkConstants.TemporalHttpActivityName},
	)

	wf := func(ctx workflow.Context) (any, error) {
		workflowTask := &sdkWorkflowsModel.WorkflowTask{}
		workflowTask.WithTemporalContext(ctx)
		workflowTask.SetTaskQueue("http-agent-queue")

		runner := NewResumableWorkflowRunner(
			config.NewRunnerConfig(config.NewConfigService(), workflowTask),
		)

		call := &model.CallHTTP{
			Call: "http",
			With: model.HTTPArguments{
				Method:   "GET",
				Endpoint: model.NewEndpoint("https://example.com/test"),
			},
		}

		return runner.executeHttpFunction("httpTask", call, map[string]any{})
	}

	env.ExecuteWorkflow(wf)

	require.True(t, env.IsWorkflowCompleted(), "workflow should complete")
	require.NoError(t, env.GetWorkflowError(), "workflow should succeed after retries")
	require.EqualValues(t, 3, atomic.LoadInt32(&attempts), "activity should be retried until success")
}
