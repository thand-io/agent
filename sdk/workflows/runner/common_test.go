package runner_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"github.com/thand-io/agent/sdk/workflows/runner"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

func workflowTimeNowUsesTemporalNow(ctx workflow.Context) (bool, error) {
	wt := &sdkWorkflowsModel.WorkflowTask{}
	wt.WithTemporalContext(ctx)

	got := runner.WorkflowTimeNow(wt)
	expected := workflow.Now(ctx).UTC()

	return got.Equal(expected), nil
}

func TestWorkflowTimeNow_UsesWorkflowNowWithTemporalContext(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	env.ExecuteWorkflow(workflowTimeNowUsesTemporalNow)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var usesTemporalNow bool
	require.NoError(t, env.GetWorkflowResult(&usesTemporalNow))
	require.True(t, usesTemporalNow, "expected WorkflowTimeNow to use workflow.Now when Temporal context is available")
}

func TestWorkflowTimeNow_FallsBackToSystemClockWithoutTemporalContext(t *testing.T) {
	wt := &sdkWorkflowsModel.WorkflowTask{}

	before := time.Now().UTC()
	got := runner.WorkflowTimeNow(wt)
	after := time.Now().UTC()

	require.False(t, got.Before(before), "expected WorkflowTimeNow to be >= pre-call timestamp")
	require.False(t, got.After(after), "expected WorkflowTimeNow to be <= post-call timestamp")
}
