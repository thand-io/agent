package workflows_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/manager"
)

// createConfigWithCleanup creates a config from a test case and registers cleanup
func createConfigWithCleanup(t *testing.T, loader *TestCaseLoader, testCase *TestCase, infra *TestInfrastructure) *config.Config {
	t.Helper()
	cfg, err := loader.CreateConfigFromTestCase(testCase)
	require.NoError(t, err, "Failed to create config")

	// Register cleanup to gracefully shutdown Temporal worker before container teardown
	infra.RegisterCleanup(func() {
		if cfg.GetServices().HasTemporal() {
			cfg.GetServices().GetTemporal().Shutdown()
		}
	})

	return cfg
}

// TestHelloWorldWorkflow tests the simplest workflow execution
func TestHelloWorldWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Set a reasonable timeout for the test
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup infrastructure
	infra := SetupTestInfrastructure(t, ctx)
	defer infra.Teardown()

	// Load the hello-world test case
	loader := NewTestCaseLoader(infra)
	testCase, err := loader.LoadTestCase("hello-world")
	require.NoError(t, err, "Failed to load hello-world test case")

	t.Run("Workflow loads correctly", func(t *testing.T) {
		require.NotNil(t, testCase.Workflows, "Workflows should be loaded")
		require.Contains(t, testCase.Workflows, "hello_world", "hello_world workflow should exist")

		workflow := testCase.Workflows["hello_world"]
		require.True(t, workflow.Enabled, "Workflow should be enabled")
		require.NotNil(t, workflow.GetWorkflow(), "Workflow DSL should be present")
	})

	t.Run("Providers load correctly", func(t *testing.T) {
		require.NotNil(t, testCase.Providers, "Providers should be loaded")
		require.Contains(t, testCase.Providers, "aws-test", "aws-test provider should exist")
		require.Contains(t, testCase.Providers, "email-test", "email-test provider should exist")

		awsProvider := testCase.Providers["aws-test"]
		require.Equal(t, "aws", awsProvider.Provider, "AWS provider type should be 'aws'")

		// Verify LocalStack endpoint was substituted
		require.NotNil(t, awsProvider.Config, "AWS config should not be nil")

		endpoint, ok := awsProvider.Config.AsMap()["endpoint"].(string)
		require.True(t, ok, "Endpoint should be a string")
		require.Contains(t, endpoint, "http://", "Endpoint should be an HTTP URL")
		t.Logf("AWS provider endpoint: %s", endpoint)
	})

	t.Run("Roles load correctly", func(t *testing.T) {
		require.NotNil(t, testCase.Roles, "Roles should be loaded")
		require.Contains(t, testCase.Roles, "test-role", "test-role should exist")

		role := testCase.Roles["test-role"]
		require.True(t, role.Enabled, "Role should be enabled")
		require.Contains(t, role.Workflows, "hello_world", "Role should allow hello_world workflow")
	})

	t.Run("Config creates successfully", func(t *testing.T) {
		cfg := createConfigWithCleanup(t, loader, testCase, infra)
		require.NotNil(t, cfg, "Config should not be nil")

		// Verify providers are set
		require.Len(t, cfg.Providers.Definitions, 2, "Should have 2 providers")

		// Verify workflows are set
		require.Len(t, cfg.Workflows.Definitions, 1, "Should have 1 workflow")
	})

	t.Run("Workflow manager initializes", func(t *testing.T) {
		cfg := createConfigWithCleanup(t, loader, testCase, infra)

		// Create workflow manager (without Temporal for now - just testing the runner)
		wm, err := manager.NewThandWorkflowManager(cfg)
		require.NoError(t, err, "Failed to create workflow manager")
		require.NotNil(t, wm, "Workflow manager should not be nil")

		// Get registered functions
		functions := wm.GetWorkflowManager().GetRegisteredFunctions()
		t.Logf("Registered functions: %v", functions)
		require.NotEmpty(t, functions, "Should have registered functions")
	})

	t.Run("Workflow executes with simple input", func(t *testing.T) {
		cfg := createConfigWithCleanup(t, loader, testCase, infra)

		workflow := testCase.Workflows["hello_world"]

		// Create a workflow task from the workflow
		workflowTask, err := models.NewElevationWorkflowContext(&workflow)
		require.NoError(t, err, "Failed to create workflow context")

		// Set input
		input := map[string]any{
			"name": "Integration Test",
		}
		workflowTask.SetInput(input)

		// Create workflow manager
		wm, err := manager.NewThandWorkflowManager(cfg)
		require.NoError(t, err, "Failed to create workflow manager")

		// Execute workflow using ResumeWorkflowTask (bypasses Temporal for direct execution)
		result, err := wm.ResumeWorkflowTask(models.NewElevateWorkflowTask(workflowTask))
		require.NoError(t, err, "Workflow execution should succeed")
		require.NotNil(t, result, "Result should not be nil")

		// Check output
		output := result.GetOutput()
		t.Logf("Workflow output: %+v", output)

		outputMap, ok := output.(map[string]any)
		require.True(t, ok, "Output should be a map")

		greeting, ok := outputMap["greeting"]
		require.True(t, ok, "Output should contain 'greeting'")
		require.Equal(t, "Hello, Integration Test!", greeting, "Greeting should match expected value")
	})

	t.Run("Workflow uses default name when not provided", func(t *testing.T) {
		cfg := createConfigWithCleanup(t, loader, testCase, infra)

		workflow := testCase.Workflows["hello_world"]

		workflowTask, err := models.NewElevationWorkflowContext(&workflow)
		require.NoError(t, err, "Failed to create workflow context")

		// Set empty input - should use default "World"
		workflowTask.SetInput(map[string]any{})

		wm, err := manager.NewThandWorkflowManager(cfg)
		require.NoError(t, err, "Failed to create workflow manager")

		result, err := wm.ResumeWorkflowTask(models.NewElevateWorkflowTask(workflowTask))
		require.NoError(t, err, "Workflow execution should succeed")

		output := result.GetOutput()
		outputMap, ok := output.(map[string]any)
		require.True(t, ok, "Output should be a map")

		greeting := outputMap["greeting"]
		require.Equal(t, "Hello, World!", greeting, "Greeting should use default 'World'")
	})
}
