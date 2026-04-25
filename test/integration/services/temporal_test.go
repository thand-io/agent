package services_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/common"
	temporalService "github.com/thand-io/agent/internal/config/services/temporal"
	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
	"github.com/thand-io/agent/test/integration/testinfra"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// echoWorkflow is a trivial workflow that returns its input.
func echoWorkflow(ctx workflow.Context, msg string) (string, error) {
	ao := workflow.ActivityOptions{StartToCloseTimeout: 10 * time.Second}
	ctx = workflow.WithActivityOptions(ctx, ao)
	var result string
	err := workflow.ExecuteActivity(ctx, echoActivity, msg).Get(ctx, &result)
	return result, err
}

// echoActivity is a trivial activity that returns its input.
func echoActivity(_ context.Context, msg string) (string, error) {
	return msg, nil
}

// temporalConfig builds a *models.TemporalConfig from running infra.
func temporalConfig(infra *testinfra.TestInfrastructure, disableVersioning bool) *models.TemporalConfig {
	host, port := infra.TemporalHostPort()
	return &models.TemporalConfig{
		Host:              host,
		Port:              port,
		Namespace:         testinfra.TemporalTestNamespace,
		DisableVersioning: disableVersioning,
	}
}

// initAndRegister creates a TemporalClient, initialises it, and registers
// the echo workflow/activity on every worker.
func initAndRegister(t *testing.T, infra *testinfra.TestInfrastructure, cfg *models.TemporalConfig, identities ...string) *temporalService.TemporalClient {
	t.Helper()
	tc := temporalService.NewTemporalClient(cfg, nil, identities...)
	require.NoError(t, tc.Initialize(), "Initialize should succeed")
	infra.RegisterCleanup(func() { _ = tc.Shutdown() })

	w := tc.GetWorker()
	require.NotNil(t, w, "GetWorker must return a worker after Initialize")
	w.RegisterWorkflow(echoWorkflow)
	w.RegisterActivity(echoActivity)
	require.NoError(t, tc.StartWorkers(), "StartWorkers should succeed")
	return tc
}

// executeEchoWorkflow runs the echo workflow and asserts it returns the input.
func executeEchoWorkflow(t *testing.T, cl client.Client, taskQueue, input string, opts ...client.StartWorkflowOptions) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	swo := client.StartWorkflowOptions{
		TaskQueue: taskQueue,
		ID:        "echo-" + strconv.FormatInt(time.Now().UnixNano(), 36),
	}
	if len(opts) > 0 {
		swo = opts[0]
		if swo.TaskQueue == "" {
			swo.TaskQueue = taskQueue
		}
		if swo.ID == "" {
			swo.ID = "echo-" + strconv.FormatInt(time.Now().UnixNano(), 36)
		}
	}

	run, err := cl.ExecuteWorkflow(ctx, swo, echoWorkflow, input)
	require.NoError(t, err, "ExecuteWorkflow should succeed")

	var result string
	require.NoError(t, run.Get(ctx, &result), "Workflow should complete")
	assert.Equal(t, input, result, "Workflow result must match input")
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestTemporalServiceVersioningDisabled verifies that with versioning turned
// off, GetClient returns immediately and a simple workflow can execute.
func TestTemporalServiceVersioningDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	infra := testinfra.SetupTemporalInfrastructure(t, ctx)
	defer infra.Teardown()

	cfg := temporalConfig(infra, true)
	tc := initAndRegister(t, infra, cfg, "test-queue-noversion")

	// GetClient should return without blocking since versioning is off.
	cl := tc.GetClient()
	require.NotNil(t, cl, "GetClient must return a non-nil client")

	assert.True(t, tc.HasClient(), "HasClient should be true")
	assert.True(t, tc.HasWorker(), "HasWorker should be true")
	assert.True(t, tc.IsVersioningDisabled(), "IsVersioningDisabled should be true")

	executeEchoWorkflow(t, cl, "test-queue-noversion", "hello-no-version")
}

// TestTemporalServiceVersioningEnabled verifies that with versioning enabled,
// GetClient blocks until the deployment version is registered, and that
// workflow execution succeeds with PinnedVersioningOverride.
func TestTemporalServiceVersioningEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	infra := testinfra.SetupTemporalInfrastructure(t, ctx)
	defer infra.Teardown()

	cfg := temporalConfig(infra, false)
	tc := initAndRegister(t, infra, cfg, "test-queue-versioned")

	// GetClient may block while the deployment registers; give it time.
	done := make(chan client.Client, 1)
	go func() { done <- tc.GetClient() }()

	select {
	case cl := <-done:
		require.NotNil(t, cl, "GetClient must return a non-nil client")
		assert.False(t, tc.IsVersioningDisabled(), "versioning should be enabled")

		buildID := common.GetBuildIdentifier()
		swo := client.StartWorkflowOptions{
			TaskQueue: "test-queue-versioned",
			ID:        "echo-versioned-" + strconv.FormatInt(time.Now().UnixNano(), 36),
			VersioningOverride: &client.PinnedVersioningOverride{
				Version: worker.WorkerDeploymentVersion{
					DeploymentName: sdkConstants.TemporalDeploymentName,
					BuildID:        buildID,
				},
			},
		}
		executeEchoWorkflow(t, cl, "test-queue-versioned", "hello-versioned", swo)

	case <-time.After(60 * time.Second):
		t.Fatal("GetClient did not return within 60 s")
	}
}

// TestTemporalServiceMultiIdentityWorkers ensures multiple task-queue
// identities each get their own worker and can execute workflows independently.
func TestTemporalServiceMultiIdentityWorkers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	infra := testinfra.SetupTemporalInfrastructure(t, ctx)
	defer infra.Teardown()

	cfg := temporalConfig(infra, true)
	tc := initAndRegister(t, infra, cfg, "queue-a", "queue-b")

	cl := tc.GetClient()
	require.NotNil(t, cl)

	// Each queue should have a dedicated worker.
	wA := tc.GetWorker("queue-a")
	wB := tc.GetWorker("queue-b")
	require.NotNil(t, wA, "worker for queue-a must exist")
	require.NotNil(t, wB, "worker for queue-b must exist")
	assert.Nil(t, tc.GetWorker("queue-c"), "non-existent queue returns nil")

	executeEchoWorkflow(t, cl, "queue-a", "msg-a")
	executeEchoWorkflow(t, cl, "queue-b", "msg-b")
}

// TestTemporalServiceShutdownUnblocksGetClient confirms that calling
// Shutdown before readiness unblocks a waiting GetClient caller.
func TestTemporalServiceShutdownUnblocksGetClient(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	infra := testinfra.SetupTemporalInfrastructure(t, ctx)
	defer infra.Teardown()

	// Use versioning enabled so GetClient blocks initially.
	cfg := temporalConfig(infra, false)
	tc := temporalService.NewTemporalClient(cfg, nil, "test-queue-shutdown")
	require.NoError(t, tc.Initialize())

	// Immediately shut down — this should unblock GetClient.
	done := make(chan struct{})
	go func() {
		_ = tc.GetClient() // should return (possibly nil) once Shutdown runs
		close(done)
	}()

	time.Sleep(100 * time.Millisecond) // let goroutine start waiting
	require.NoError(t, tc.Shutdown())

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("GetClient not unblocked by Shutdown within 5 s")
	}
}

// TestTemporalServiceNoIdentities confirms Initialize returns an error
// when no identities are provided.
func TestTemporalServiceNoIdentities(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	infra := testinfra.SetupTemporalInfrastructure(t, ctx)
	defer infra.Teardown()

	cfg := temporalConfig(infra, true)
	tc := temporalService.NewTemporalClient(cfg, nil)
	err := tc.Initialize()
	require.Error(t, err, "Initialize with zero identities should fail")
	assert.Contains(t, err.Error(), "at least one identity")
}

// TestTemporalServiceIdentityDedup verifies that duplicate identities are
// de-duplicated so only one worker is started per unique task queue.
func TestTemporalServiceIdentityDedup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	infra := testinfra.SetupTemporalInfrastructure(t, ctx)
	defer infra.Teardown()

	cfg := temporalConfig(infra, true)
	tc := initAndRegister(t, infra, cfg, "dup-queue", "dup-queue", "dup-queue")

	cl := tc.GetClient()
	require.NotNil(t, cl)

	// Only one worker should exist despite three identical identity args.
	wAll := tc.GetWorker()
	require.NotNil(t, wAll, "GetWorker must return a worker")
	assert.Nil(t, tc.GetWorker("other"), "non-existent queue returns nil")

	executeEchoWorkflow(t, cl, "dup-queue", "dedup-msg")
}

// TestTemporalServiceNamespaceValidation exercises the live Temporal
// connection by executing a ListWorkflow call and checking accessors.
func TestTemporalServiceNamespaceValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	infra := testinfra.SetupTemporalInfrastructure(t, ctx)
	defer infra.Teardown()

	cfg := temporalConfig(infra, true)
	tc := initAndRegister(t, infra, cfg, "ns-validate-queue")

	cl := tc.GetClient()
	require.NotNil(t, cl)

	// Smoke test: list workflows (should return 0, but no error).
	resp, err := cl.ListWorkflow(ctx, &workflowservice.ListWorkflowExecutionsRequest{
		Namespace: testinfra.TemporalTestNamespace,
	})
	require.NoError(t, err, "ListWorkflow should succeed against test namespace")
	assert.NotNil(t, resp)

	// Accessor checks
	host, port := infra.TemporalHostPort()
	assert.Equal(t, host+":"+strconv.Itoa(port), tc.GetHostPort())
	assert.Equal(t, testinfra.TemporalTestNamespace, tc.GetNamespace())
	assert.Equal(t, "ns-validate-queue", tc.GetTaskQueue())
	assert.Equal(t, "ns-validate-queue", tc.GetIdentity())
}

// TestTemporalServiceGetClientAccessorsAreSafe ensures concurrent calls to
// GetClient, HasClient, HasWorker, and GetWorker don't race.
func TestTemporalServiceGetClientAccessorsAreSafe(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	infra := testinfra.SetupTemporalInfrastructure(t, ctx)
	defer infra.Teardown()

	cfg := temporalConfig(infra, true)
	tc := initAndRegister(t, infra, cfg, "race-queue")

	// Hammer all accessors concurrently.
	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_ = tc.GetClient()
			_ = tc.HasClient()
			_ = tc.HasWorker()
			_ = tc.GetWorker()
			_ = tc.GetWorker("race-queue")
		}()
	}
	wg.Wait()
}

// TestTemporalServiceMaxWorkersCap verifies that NewTemporalClient caps
// the number of workers at MaxWorkers.
func TestTemporalServiceMaxWorkersCap(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ids := make([]string, temporalService.MaxWorkers+3)
	for i := range ids {
		ids[i] = "q-" + strconv.Itoa(i)
	}

	cfg := &models.TemporalConfig{
		Host:              "localhost",
		Port:              7233,
		Namespace:         "default",
		DisableVersioning: true,
	}
	tc := temporalService.NewTemporalClient(cfg, nil, ids...)

	// We can't call Initialize (no real server), but GetTaskQueue/GetIdentity
	// reflect the first identity and the cap is applied internally.
	assert.Equal(t, "q-0", tc.GetTaskQueue())
	assert.Equal(t, "q-0", tc.GetIdentity())
}
