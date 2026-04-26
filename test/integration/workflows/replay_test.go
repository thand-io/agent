package workflows_test

// replay_test.go verifies that the elevation workflow code is replay-safe
// (deterministic) by replaying captured event histories through the current code.
//
// # Capturing histories
//
// Any integration test that runs an elevation workflow can call
// replayElevationWorkflow after the workflow completes to both check
// determinism inline AND optionally persist the history to
// testdata/<testCaseName>/history.json for future regression protection.
//
// To capture committed history files, run the integration suite with:
//
//	SAVE_REPLAY_HISTORY=1 cd test && go test -v -timeout 15m ./integration/workflows/...
//
// # What these tests catch
//
// Replaying a history against modified workflow code detects:
//   - reordered activity / local-activity / timer / signal-receive calls
//   - added or removed workflow.Go goroutines
//   - changed activity type names (e.g. due to a rename or refactor)
//   - any other change that makes the workflow emit Commands in a different
//     order than what was recorded

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	internalManager "github.com/thand-io/agent/internal/workflows/manager"
	"github.com/thand-io/agent/test/integration/testinfra"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

// replayElevationWorkflow saves the completed history for workflowID to a
// temporary file and immediately replays it using the provided manager.
// Call this at the end of any integration test that runs an elevation workflow
// to catch determinism regressions as early as possible.
//
// When the environment variable SAVE_REPLAY_HISTORY=1 is set the history is
// also written to testdata/<testCaseName>/history.json so it can be committed
// and picked up by TestReplayElevationWorkflow in subsequent CI runs.
func replayElevationWorkflow(
	t *testing.T,
	wm *internalManager.ThandWorkflowManager,
	infra *testinfra.TestInfrastructure,
	ctx context.Context,
	workflowID string,
	testCaseName string,
) {
	t.Helper()

	tmpFile := filepath.Join(t.TempDir(), "history.json")
	infra.SaveWorkflowHistory(t, ctx, workflowID, "", tmpFile)

	if os.Getenv("SAVE_REPLAY_HISTORY") == "1" && testCaseName != "" {
		committed := filepath.Join("testdata", testCaseName, "history.json")
		infra.SaveWorkflowHistory(t, ctx, workflowID, "", committed)
		t.Logf("Persisted history to %s for future regression protection", committed)
	}

	if _, statErr := os.Stat(tmpFile); os.IsNotExist(statErr) {
		t.Logf("No history saved for %s — skipping inline replay", workflowID)
		return
	}

	replayer := worker.NewWorkflowReplayer()
	replayer.RegisterWorkflowWithOptions(
		wm.ElevationWorkflowHandlerForReplay(),
		workflow.RegisterOptions{
			Name: models.TemporalExecuteElevationWorkflowName,
		},
	)

	err := replayer.ReplayWorkflowHistoryFromJSONFile(nil, tmpFile)
	require.NoError(t, err,
		"Determinism replay failed for workflow %s. Check for reordered "+
			"activities, added/removed goroutines, or changed activity type names.",
		workflowID)
	t.Logf("Workflow %s passed determinism replay check", workflowID)
}

// TestReplayElevationWorkflow replays committed history files found at
// testdata/*/history.json against the current elevation workflow code.
//
// The test is skipped when no committed history files are present (i.e. on a
// fresh clone). Capture histories by running:
//
//	SAVE_REPLAY_HISTORY=1 cd test && go test -v -timeout 15m ./integration/workflows/...
//
// Once histories are committed, this test will run on every PR and catch
// determinism regressions without requiring a live workflow run.
func TestReplayElevationWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping replay test in short mode")
	}

	pattern := filepath.Join("testdata", "*", "history.json")
	historyFiles, err := filepath.Glob(pattern)
	require.NoError(t, err, "error searching for history files")

	if len(historyFiles) == 0 {
		t.Skip("No committed history files found under testdata/*/history.json. " +
			"Run the integration suite with SAVE_REPLAY_HISTORY=1 to capture them.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// TestReplayElevationWorkflow needs a live Temporal service to initialise the
	// workflow manager.  The replay itself is entirely local: no workflow is
	// dispatched and no activities execute against the running server.
	infra := testinfra.SetupTestInfrastructure(t, ctx)
	defer infra.Teardown()

	loader := testinfra.NewTestCaseLoader(infra, "testdata")
	testCase, loadErr := loader.LoadTestCase("aws-elevation")
	require.NoError(t, loadErr, "failed to load aws-elevation test case for manager construction")

	cfg, cfgErr := loader.CreateConfigFromTestCase(testCase)
	require.NoError(t, cfgErr)

	infra.RegisterCleanup(func() {
		if cfg.GetServices().HasTemporal() {
			cfg.GetServices().GetTemporal().Shutdown()
		}
	})

	wm, managerErr := internalManager.NewThandWorkflowManager(cfg)
	require.NoError(t, managerErr)

	replayer := worker.NewWorkflowReplayer()
	replayer.RegisterWorkflowWithOptions(
		wm.ElevationWorkflowHandlerForReplay(),
		workflow.RegisterOptions{
			Name: models.TemporalExecuteElevationWorkflowName,
		},
	)

	for _, histFile := range historyFiles {
		histFile := histFile
		testName := filepath.Base(filepath.Dir(histFile))

		t.Run(testName, func(t *testing.T) {
			err := replayer.ReplayWorkflowHistoryFromJSONFile(nil, histFile)
			require.NoError(t, err,
				"Replay of %s diverged from recorded history — the elevation "+
					"workflow has a determinism violation. Check for reordered "+
					"activities, added/removed goroutines, or changed activity "+
					"type names.", histFile)
		})
	}
}
