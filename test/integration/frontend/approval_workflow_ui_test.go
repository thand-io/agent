package ui_e2e

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	testcommon "github.com/thand-io/agent/test/integration/common"
)

// TestApprovalWorkflowUI tests a workflow requiring external approval via the UI
func TestApprovalWorkflowUI(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping UI integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Load the test case
	loader := &TestCaseLoader{basePath: "testdata"}
	testCase, err := loader.LoadTestCase("approval-workflow-ui")
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
	// Wait for services to stabilize
	time.Sleep(5 * time.Second)

	// Create chromedp browser
	browser := NewBrowser(t, infra.ThandEndpoint)
	defer browser.Close()

	t.Run("Complete approval workflow via UI", func(t *testing.T) {
		// Engineer credentials
		engineerUsername := "engineer@thand.io"
		engineerPassword := "testpass123"

		// Clear any existing emails
		err := infra.ClearEmails()
		require.NoError(t, err, "Should be able to clear emails")

		// Step 1: Engineer submits elevation request
		t.Log("Engineer submitting elevation request...")
		workflowID, err := browser.CompleteElevationWorkflow(
			ctx,
			engineerUsername,
			engineerPassword,
			"aws_production_admin",
			"aws-localstack",
			"Need production access to debug critical issue",
			"PT2H",
		)
		require.NoError(t, err, "Engineer should be able to submit elevation request")
		require.NotEmpty(t, workflowID, "Workflow ID should be returned")

		t.Logf("Workflow started with ID: %s", workflowID)

		// Step 2: Wait for approval email to manager
		t.Log("Waiting for approval email to manager...")
		var approvalEmail *testcommon.MailHogMessage
		for i := 0; i < 30; i++ {
			time.Sleep(2 * time.Second)

			emails, err := infra.GetEmails()
			require.NoError(t, err, "Should be able to retrieve emails")

			for j := range emails {
				if emails[j].To[0].Mailbox == "manager" {
					approvalEmail = &emails[j]
					break
				}
			}

			if approvalEmail != nil {
				break
			}
		}
		require.NotNil(t, approvalEmail, "Manager should have received approval email")

		t.Logf("Manager received approval email: %s", approvalEmail.Content.Headers["Subject"][0])

		// Step 3: Extract approval link from email
		t.Log("Extracting approval link from email...")
		approvalLinkRegex := regexp.MustCompile(`https?://[^\s<>"]+/executions/[^\s<>"]+`)
		links := approvalLinkRegex.FindAllString(approvalEmail.Content.Body, -1)
		require.NotEmpty(t, links, "Email should contain approval link")

		approvalLink := links[0]
		t.Logf("Found approval link: %s", approvalLink)

		// Step 4: Manager logs in and approves (simulate different user)
		t.Log("Manager logging in to approve request...")
		managerUsername := "manager@thand.io"
		managerPassword := "managerpass123"

		// Login as manager
		err = browser.LoginWithOIDC(ctx, managerUsername, managerPassword)
		require.NoError(t, err, "Manager should be able to log in")

		// Navigate to the approval page and approve
		t.Log("Manager approving the request...")
		err = browser.ClickApproveButton(ctx, workflowID)
		require.NoError(t, err, "Manager should be able to approve the request")

		// Step 5: Wait for workflow to complete
		t.Log("Waiting for workflow to complete after approval...")
		status, err := browser.WaitForWorkflowCompletion(ctx, workflowID, 2*time.Minute)
		require.NoError(t, err, "Should be able to wait for workflow completion")
		require.Equal(t, "completed", status, "Workflow should complete successfully after approval")

		t.Logf("Workflow %s completed with status: %s", workflowID, status)

		// Step 6: Verify workflow via Temporal client
		t.Log("Verifying workflow in Temporal...")
		exec := infra.TemporalClient.GetWorkflow(ctx, workflowID, "")
		var result interface{}
		err = exec.Get(ctx, &result)
		require.NoError(t, err, "Workflow should be accessible via Temporal client")

		t.Log("Approval workflow via UI completed successfully!")
	})

	t.Run("Verify workflow appears in engineer's execution history", func(t *testing.T) {
		// TODO: Add test to verify workflow appears in engineer's execution history
		// This would navigate to /executions and verify the workflow is listed for the engineer
	})

	t.Run("Test workflow denial flow", func(t *testing.T) {
		// Clear emails
		err := infra.ClearEmails()
		require.NoError(t, err, "Should be able to clear emails")

		// Engineer submits request
		engineerUsername := "engineer@thand.io"
		engineerPassword := "testpass123"

		t.Log("Engineer submitting another elevation request...")
		workflowID, err := browser.CompleteElevationWorkflow(
			ctx,
			engineerUsername,
			engineerPassword,
			"aws_production_admin",
			"aws-localstack",
			"Testing denial flow",
			"PT1H",
		)
		require.NoError(t, err, "Engineer should be able to submit elevation request")
		require.NotEmpty(t, workflowID, "Workflow ID should be returned")

		// Wait for approval email
		t.Log("Waiting for approval email...")
		time.Sleep(5 * time.Second)

		// Manager logs in and denies (we'd need a ClickDenyButton function)
		// For now, just verify the workflow was created
		t.Logf("Workflow %s created and awaiting approval", workflowID)

		// TODO: Implement denial flow
		// - Add ClickDenyButton method to playwright.go
		// - Manager clicks deny
		// - Verify engineer receives denial email
		// - Verify workflow status is "denied"
	})
}
