package ui_e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	testcommon "github.com/thand-io/agent/test/integration/common"
)

// TestAWSElevationUI tests the complete AWS elevation workflow via the UI
func TestAWSElevationUI(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping UI integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Load the test case
	loader := &TestCaseLoader{basePath: "testdata"}
	testCase, err := loader.LoadTestCase("aws-elevation-ui")
	require.NoError(t, err, "Failed to load test case")

	// Setup infrastructure
	t.Log("Setting up test infrastructure...")
	infra := SetupUITestInfrastructure(t, ctx, testCase)
	defer infra.Teardown()

	// Update loader with infrastructure for variable interpolation
	loader.infra = infra

	// Create config from test case (this registers custom tasks)
	cfg, err := loader.CreateConfigFromTestCase(testCase)
	require.NoError(t, err, "Failed to create config")

	// Register cleanup to gracefully shutdown Temporal worker before container teardown
	infra.RegisterCleanup(func() {
		if cfg.GetServices().HasTemporal() {
			cfg.GetServices().GetTemporal().Shutdown()
		}
	})

	// Wait a bit for all services to stabilize
	time.Sleep(5 * time.Second)

	// Create chromedp browser
	browser := NewBrowser(t, infra.ThandEndpoint)
	defer browser.Close()

	t.Run("Complete AWS elevation workflow via UI", func(t *testing.T) {
		// Test credentials
		username := "testuser@thand.io"
		password := "testpass123"

		// Perform the complete elevation workflow
		workflowID, err := browser.CompleteElevationWorkflow(
			ctx,
			username,
			password,
			"aws_test_admin",
			"aws-localstack",
			"Testing AWS elevation via UI",
			"PT1H",
		)
		require.NoError(t, err, "Elevation workflow should complete successfully")
		require.NotEmpty(t, workflowID, "Workflow ID should be returned")

		t.Logf("Workflow started with ID: %s", workflowID)

		// Wait for approval email to arrive
		t.Log("Waiting for approval email...")
		time.Sleep(5 * time.Second)

		emails, err := infra.GetEmails()
		require.NoError(t, err, "Should be able to retrieve emails")
		require.NotEmpty(t, emails, "Should have received at least one email")

		// Find the approval email
		var approvalEmail *testcommon.MailHogMessage
		for i := range emails {
			if emails[i].To[0].Mailbox == "testuser" {
				approvalEmail = &emails[i]
				break
			}
		}
		require.NotNil(t, approvalEmail, "Should have received approval email")

		t.Logf("Received approval email: %s", approvalEmail.Content.Headers["Subject"][0])

		// Simulate clicking approve button via UI
		t.Log("Clicking approve button...")
		err = browser.ClickApproveButton(ctx, workflowID)
		require.NoError(t, err, "Should be able to click approve button")

		// Wait for workflow to complete
		t.Log("Waiting for workflow to complete...")
		status, err := browser.WaitForWorkflowCompletion(ctx, workflowID, 2*time.Minute)
		require.NoError(t, err, "Should be able to wait for workflow completion")
		require.Equal(t, "completed", status, "Workflow should complete successfully")

		t.Logf("Workflow %s completed with status: %s", workflowID, status)

		// Verify via Temporal client
		t.Log("Verifying workflow in Temporal...")
		exec := infra.TemporalClient.GetWorkflow(ctx, workflowID, "")
		var result interface{}
		err = exec.Get(ctx, &result)
		require.NoError(t, err, "Workflow should be accessible via Temporal client")

		t.Log("AWS elevation workflow via UI completed successfully!")
	})

	t.Run("Verify workflow is visible in executions list", func(t *testing.T) {
		// TODO: Add test to verify workflow appears in executions list
		// This would navigate to /executions and verify the workflow is listed
	})
}
