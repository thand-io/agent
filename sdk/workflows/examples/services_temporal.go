package examples

import (
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/worker"
)

// InMemoryTemporalService provides an in-memory Temporal test environment for unit testing.
// It implements the TemporalService interface using Temporal's test suite.
type InMemoryTemporalService struct {
	env       *testsuite.TestWorkflowEnvironment
	testSuite *testsuite.WorkflowTestSuite
	client    client.Client
	worker    worker.Worker
}

// Initialize sets up the in-memory Temporal test environment.
func (t *InMemoryTemporalService) Initialize() error {
	// The test environment is already initialized in SetupInMemoryTemporal
	return nil
}

// Shutdown cleans up the in-memory Temporal test environment.
func (t *InMemoryTemporalService) Shutdown() error {
	// Test environment doesn't require explicit shutdown
	return nil
}

// GetClient returns the mock Temporal client for the test environment.
func (t *InMemoryTemporalService) GetClient() client.Client {
	return t.client
}

// HasClient returns true if a client is configured.
func (t *InMemoryTemporalService) HasClient() bool {
	return t.client != nil
}

// GetWorker returns the mock Temporal worker for the test environment.
func (t *InMemoryTemporalService) GetWorker() worker.Worker {
	return t.worker
}

// HasWorker returns true if a worker is configured.
func (t *InMemoryTemporalService) HasWorker() bool {
	return t.worker != nil
}

// GetHostPort returns the host:port for the Temporal server (empty for in-memory).
func (t *InMemoryTemporalService) GetHostPort() string {
	return "in-memory"
}

// GetNamespace returns the Temporal namespace (default for testing).
func (t *InMemoryTemporalService) GetNamespace() string {
	return "default"
}

// GetTaskQueue returns the task queue name for this test environment.
func (t *InMemoryTemporalService) GetTaskQueue() string {
	return "test-task-queue"
}

// IsVersioningDisabled returns true since versioning is not used in tests.
func (t *InMemoryTemporalService) IsVersioningDisabled() bool {
	return true
}

// GetTestEnvironment returns the underlying test workflow environment for test assertions.
func (t *InMemoryTemporalService) GetTestEnvironment() *testsuite.TestWorkflowEnvironment {
	return t.env
}

// SetupInMemoryTemporal configures an in-memory Temporal service for testing.
// This creates a test workflow environment that can be used to test Temporal workflows
// without requiring a running Temporal server.
func (s *Services) SetupInMemoryTemporal() {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	s.temporal = &InMemoryTemporalService{
		env:       env,
		testSuite: testSuite,
	}
}
