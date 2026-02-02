package workflows_test

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/manager"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"go.temporal.io/api/enums/v1"
)

// TestAWSElevationApprovalsWorkflow tests the AWS elevation workflow with multiple approvers
func TestAWSElevationApprovalsWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup infrastructure
	infra := SetupTestInfrastructure(t, ctx)
	defer infra.Teardown()

	// Clear any existing emails
	err := infra.ClearEmails()
	require.NoError(t, err, "Failed to clear emails")

	// Load the aws-elevation-approvals test case
	loader := NewTestCaseLoader(infra)
	testCase, err := loader.LoadTestCase("aws-elevation-approvals")
	require.NoError(t, err, "Failed to load aws-elevation-approvals test case")

	cfg, err := loader.CreateConfigFromTestCase(testCase)
	require.NoError(t, err, "Failed to create config")

	// Register cleanup to gracefully shutdown Temporal worker before container teardown
	infra.RegisterCleanup(func() {
		if cfg.GetServices().HasTemporal() {
			cfg.GetServices().GetTemporal().Shutdown()
		}
	})

	// Create IAM client for LocalStack verification
	iamClient := createLocalStackIAMClient(t, ctx, infra.LocalStackEndpoint)

	// Create an IAM user in LocalStack for testing traditional IAM authorization
	testUsername := "testuser"
	t.Logf("Creating IAM user '%s' in LocalStack...", testUsername)
	_, err = iamClient.CreateUser(ctx, &iam.CreateUserInput{
		UserName: aws.String(testUsername),
	})
	require.NoError(t, err, "Failed to create IAM user in LocalStack")
	t.Logf("Created IAM user: %s", testUsername)

	// Create test user with Source="iam" to use traditional IAM instead of Identity Center
	testUser := &models.User{
		Email:    "testuser@thand.io",
		Name:     "Test User",
		Username: testUsername,
		Source:   "iam", // This tells the AWS provider to use traditional IAM
	}

	// Register approver identities in the provider
	// This is necessary because GetIdentity checks the provider for existence
	provider, err := cfg.GetProviderByName("aws-localstack")
	require.NoError(t, err, "Failed to get aws-localstack provider")

	provider.AddIdentities(
		models.Identity{
			ID:    testUser.Email,
			Label: testUser.Name,
			User:  testUser,
		},
		models.Identity{
			ID:    "approver1@thand.io",
			Label: "Approver 1",
			User: &models.User{
				ID:       "user-id-001", // Different ID
				Email:    "approver1@thand.io",
				Username: "approver1",
				Name:     "Approver 1",
			},
		},
		models.Identity{
			ID:    "approver2@thand.io",
			Label: "Approver 2",
			User: &models.User{
				ID:       "user-id-002", // Different ID
				Email:    "approver2@thand.io",
				Username: "approver2",
				Name:     "Approver 2",
			},
		},
	)

	// Get the workflow from config to ensure Identifier field is set
	workflowPtr, err := cfg.GetWorkflowByName("aws_multi_approval")
	require.NoError(t, err, "Failed to get workflow from config")
	workflow := *workflowPtr
	role := testCase.Roles["aws_test_admin"]

	// Calculate the expected role name that will be created by the AWS provider
	// The provider uses GetUniqueIdentifier which appends a hash to ensure uniqueness
	expectedRoleName := role.GetUniqueIdentifier(testUser)
	t.Logf("Expected IAM role name: %s", expectedRoleName)

	t.Run("Full elevation lifecycle with multiple approvers", func(t *testing.T) {
		// Create workflow task (elevation request)
		sdkWorkflowTask, err := models.NewElevationWorkflowContext(&workflow)
		require.NoError(t, err, "Failed to create workflow context")

		// Wrap in ElevateWorkflowTask to get SetRole/SetUser methods
		workflowTask := models.NewElevateWorkflowTask(sdkWorkflowTask)

		// Set up elevation context - include "workflow" key for Hydrate to find the workflow
		elevationContext := map[string]any{
			"user": map[string]any{
				"email":    testUser.Email,
				"name":     testUser.Name,
				"username": testUser.Username,
				"source":   testUser.Source,
			},
			"role":       "aws_test_admin",
			"workflow":   "aws_multi_approval", // Required for Hydrate to find the workflow
			"reason":     "Integration test elevation",
			"identities": []string{testUser.Email},
			"providers":  []string{"aws-localstack"},
			"duration":   "1m", // 1 minute (minimum allowed)
		}

		workflowTask.SetContext(elevationContext)
		workflowTask.SetInput(elevationContext)
		workflowTask.SetRole(&role)
		workflowTask.SetUser(testUser)

		// Create workflow manager
		wm, err := manager.NewThandWorkflowManager(cfg)
		require.NoError(t, err, "Failed to create workflow manager")

		// Get the workflow ID that was generated by NewElevationWorkflowContext
		workflowID := workflowTask.WorkflowID
		t.Logf("Workflow ID: %s", workflowID)

		// Start workflow in background using ResumeWorkflow.
		go func() {
			t.Log("Starting workflow execution via Temporal...")
			_, err := wm.ResumeWorkflow(workflowTask)
			if err != nil {
				t.Errorf("ResumeWorkflow failed: %v", err)
			}
		}()

		// Wait for approval email
		t.Log("Waiting for approval email...")
		email, err := infra.WaitForEmail(testUser.Email, 30*time.Second)
		require.NoError(t, err, "Should receive approval email")

		// Log email details
		subject := ""
		if subjects, ok := email.Content.Headers["Subject"]; ok && len(subjects) > 0 {
			subject = subjects[0]
		}
		t.Logf("Received email: %s", subject)

		// Extract and log links
		links := infra.ExtractLinksFromEmail(email)
		t.Logf("Found %d links in email body", len(links))

		// Find approve URL pattern in email
		approveURLRegex := regexp.MustCompile(`https?://[^\s<>"]*(?:approve|signal)[^\s<>"]*`)
		approveMatches := approveURLRegex.FindAllString(email.Content.Body, -1)
		t.Logf("Approval-related URLs found: %v", approveMatches)

		// Simulate FIRST approval
		t.Log("Simulating FIRST approval via Temporal signal...")

		// Create approval event from approver1 with different ID but same email
		approvalEvent1 := cloudevents.NewEvent()
		approvalEvent1.SetID(uuid.New().String())
		approvalEvent1.SetType("com.thand.approval")
		approvalEvent1.SetSource("urn:thand:test")
		approvalEvent1.SetData(cloudevents.ApplicationJSON, map[string]any{
			"approved": true,
		})
		// Approver 1 with ID "user-id-001" but we test they're equal via email
		approvalEvent1.SetExtension("user", "approver1@thand.io")

		// Signal the workflow with approval
		temporalClient := infra.TemporalClient
		err = temporalClient.SignalWorkflow(
			ctx,
			workflowID,
			sdkWorkflowsModel.TemporalEmptyRunId,
			sdkWorkflowsModel.TemporalEventSignalName,
			approvalEvent1,
		)
		require.NoError(t, err, "Signal error sending first approval signal")
		t.Log("Sent first approval signal to workflow")

		// Verify workflow is NOT done yet
		// ResumeWorkflow returns immediately after starting/signaling, so we check status in Temporal
		time.Sleep(1 * time.Second) // Give it a moment to process the signal
		desc, err := infra.TemporalClient.DescribeWorkflowExecution(ctx, workflowID, "")
		require.NoError(t, err, "Should be able to describe workflow")
		require.Equal(t, enums.WORKFLOW_EXECUTION_STATUS_RUNNING, desc.WorkflowExecutionInfo.Status, "Workflow should be running after first approval")
		t.Log("Workflow correctly continuing (RUNNING) after first approval")

		// Simulate SECOND approval
		t.Log("Simulating SECOND approval via Temporal signal...")

		// Create approval event from approver2 with different ID but same email
		approvalEvent2 := cloudevents.NewEvent()
		approvalEvent2.SetID(uuid.New().String())
		approvalEvent2.SetType("com.thand.approval")
		approvalEvent2.SetSource("urn:thand:test")
		approvalEvent2.SetData(cloudevents.ApplicationJSON, map[string]any{
			"approved": true,
		})
		// Approver 2 with ID "user-id-002" but we test they're equal via email
		approvalEvent2.SetExtension("user", "approver2@thand.io")

		// Signal the workflow with approval
		err = temporalClient.SignalWorkflow(
			ctx,
			workflowID,
			sdkWorkflowsModel.TemporalEmptyRunId,
			sdkWorkflowsModel.TemporalEventSignalName,
			approvalEvent2,
		)
		require.NoError(t, err, "Signal error sending second approval signal")
		t.Log("Sent second approval signal to workflow")

		// Wait a moment for the authorization activity to complete
		t.Log("Waiting for authorization activity to complete...")
		time.Sleep(2 * time.Second)

		// Step 3: Verify the IAM role was created and user can assume it
		t.Log("Step 3: Verifying IAM role was created with proper authorization...")
		verifyIAMRoleCreated(t, ctx, iamClient, expectedRoleName, testUsername)

		// Step 4: Verify the workflow has a timer (created after authorization)
		t.Log("Step 4: Verifying workflow has an active revocation timer...")
		verifyWorkflowHasTimer(t, ctx, temporalClient, workflowID)

		// Step 5: Cancel the workflow - this triggers cleanup which revokes the role
		t.Log("Step 5: Cancelling workflow to trigger cleanup activity...")
		err = temporalClient.CancelWorkflow(ctx, workflowID, sdkWorkflowsModel.TemporalEmptyRunId)
		require.NoError(t, err, "Should be able to cancel workflow")
		t.Log("Sent cancel request to workflow")

		// Step 6: Wait for the workflow to complete after cancellation
		t.Log("Step 6: Waiting for workflow to complete cancellation and cleanup...")
		waitForWorkflowCompletion(t, ctx, temporalClient, workflowID, 60*time.Second)

		// Step 7: Verify the role has been revoked (Deny policy in place)
		t.Log("Step 7: Verifying IAM role has been revoked after cleanup...")
		verifyIAMRoleRevoked(t, ctx, iamClient, expectedRoleName)

		t.Log("AWS elevation multi-approver integration test completed successfully!")
	})
}
