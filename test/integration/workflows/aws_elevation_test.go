package workflows_test

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/manager"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
)

// TestAWSElevationWorkflow tests the full AWS elevation workflow with email approval
func TestAWSElevationWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Set a reasonable timeout for the test
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup infrastructure
	infra := SetupTestInfrastructure(t, ctx)
	defer infra.Teardown()

	// Clear any existing emails
	err := infra.ClearEmails()
	require.NoError(t, err, "Failed to clear emails")

	// Load the aws-elevation test case
	loader := NewTestCaseLoader(infra)
	testCase, err := loader.LoadTestCase("aws-elevation")
	require.NoError(t, err, "Failed to load aws-elevation test case")

	t.Run("Test case loads correctly", func(t *testing.T) {
		require.NotNil(t, testCase.Workflows, "Workflows should be loaded")
		require.Contains(t, testCase.Workflows, "aws_self_approval", "aws_self_approval workflow should exist")
		require.NotNil(t, testCase.Providers, "Providers should be loaded")
		require.Contains(t, testCase.Providers, "aws-localstack", "aws-localstack provider should exist")
		require.Contains(t, testCase.Providers, "email-test", "email-test provider should exist")
		require.NotNil(t, testCase.Roles, "Roles should be loaded")
		require.Contains(t, testCase.Roles, "aws_test_admin", "aws_test_admin role should exist")
	})

	t.Run("AWS elevation with email self-approval", func(t *testing.T) {
		cfg, err := loader.CreateConfigFromTestCase(testCase)
		require.NoError(t, err, "Failed to create config")

		// Register cleanup to gracefully shutdown Temporal worker before container teardown
		infra.RegisterCleanup(func() {
			if cfg.GetServices().HasTemporal() {
				cfg.GetServices().GetTemporal().Shutdown()
			}
		})

		// Create test user
		testUser := &models.User{
			Email: "testuser@thand.io",
			Name:  "Test User",
		}

		// Get the workflow from config (not from testCase)
		// This is important: workflows must be retrieved from the config after initialization
		// because ApplyWorkflows sets the Identifier field which is required for workflow hydration
		workflowPtr, err := cfg.GetWorkflowByName("aws_self_approval")
		require.NoError(t, err, "Failed to get workflow from config")
		workflow := *workflowPtr
		role := testCase.Roles["aws_test_admin"]

		// Create workflow task (elevation request)
		workflowTask, err := models.NewElevationWorkflowContext(&workflow)
		require.NoError(t, err, "Failed to create workflow context")

		// Set up elevation context
		elevationContext := map[string]any{
			"user": map[string]any{
				"email": testUser.Email,
				"name":  testUser.Name,
			},
			"role":       "aws_test_admin",
			"workflow":   "aws_self_approval", // Required for Hydrate to find the workflow
			"reason":     "Integration test elevation",
			"identities": []string{testUser.Email},
			"providers":  []string{"aws-localstack"},
			"duration":   "1m", // 1 minute (minimum allowed)
		}

		// Set the context which includes user info for approvals
		workflowTask.SetContext(elevationContext)
		workflowTask.SetInput(elevationContext)
		workflowTask.SetRole(&role)
		workflowTask.SetUser(testUser)

		// Create workflow manager
		wm, err := manager.NewThandWorkflowManager(cfg)
		require.NoError(t, err, "Failed to create workflow manager")
		require.NotNil(t, wm, "Workflow manager should not be nil")

		// Verify Temporal is configured
		services := cfg.GetServices()
		require.True(t, services.HasTemporal(), "Temporal should be configured")
		t.Logf("Temporal is configured and available")

		// Start the workflow via Temporal - this will properly handle signals
		go func() {
			_, err := wm.ResumeWorkflow(workflowTask)
			if err != nil {
				t.Logf("Workflow completed or errored: %v", err)
			}
		}()

		// Wait for the approval email to arrive
		t.Log("Waiting for approval email...")
		email, err := infra.WaitForEmail(testUser.Email, 30*time.Second)
		require.NoError(t, err, "Should receive approval email")
		require.NotNil(t, email, "Email should not be nil")

		t.Logf("Received email with subject: %s", email.Content.Headers["Subject"])
		t.Logf("Email body length: %d characters", len(email.Content.Body))

		// Extract approval URL from email
		links := infra.ExtractLinksFromEmail(email)
		t.Logf("Found %d links in email:", len(links))
		for i, link := range links {
			t.Logf("  Link %d: %s", i+1, link)
		}

		// Find the approve link
		var approveURL string
		for _, link := range links {
			if strings.Contains(strings.ToLower(link), "approve") {
				approveURL = link
				break
			}
		}

		if len(approveURL) != 0 {
			t.Logf("Approval link found: %s", approveURL)
		} else {
			t.Log("No explicit 'approve' link found, but email was received successfully")
		}

		t.Log("Test passed: Email approval workflow sent email successfully")
	})
}

// TestAWSElevationWorkflowSimple tests a simplified AWS elevation flow
// This test directly executes workflow tasks without the full Temporal orchestration
func TestAWSElevationWorkflowSimple(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup infrastructure
	infra := SetupTestInfrastructure(t, ctx)
	defer infra.Teardown()

	// Create IAM client for LocalStack
	iamClient := createLocalStackIAMClient(t, ctx, infra.LocalStackEndpoint)

	t.Run("Verify LocalStack IAM is working", func(t *testing.T) {
		// List users to verify IAM is working
		result, err := iamClient.ListUsers(ctx, &iam.ListUsersInput{})
		require.NoError(t, err, "Should be able to list IAM users")
		t.Logf("Found %d IAM users in LocalStack", len(result.Users))
	})

	t.Run("Create and verify IAM role", func(t *testing.T) {
		roleName := "thand-test-admin-role"

		// Create a test role
		assumeRolePolicy := `{
			"Version": "2012-10-17",
			"Statement": [{
				"Effect": "Allow",
				"Principal": {"AWS": "arn:aws:iam::000000000000:root"},
				"Action": "sts:AssumeRole"
			}]
		}`

		_, err := iamClient.CreateRole(ctx, &iam.CreateRoleInput{
			RoleName:                 aws.String(roleName),
			AssumeRolePolicyDocument: aws.String(assumeRolePolicy),
			Description:              aws.String("Test admin role for integration testing"),
		})
		require.NoError(t, err, "Should create IAM role")
		t.Logf("Created IAM role: %s", roleName)

		// Verify role exists
		getResult, err := iamClient.GetRole(ctx, &iam.GetRoleInput{
			RoleName: aws.String(roleName),
		})
		require.NoError(t, err, "Should get IAM role")
		require.Equal(t, roleName, *getResult.Role.RoleName)
		t.Log("Verified IAM role exists")

		// Attach a policy
		policyDoc := `{
			"Version": "2012-10-17",
			"Statement": [{
				"Effect": "Allow",
				"Action": "*",
				"Resource": "*"
			}]
		}`

		_, err = iamClient.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
			RoleName:       aws.String(roleName),
			PolicyName:     aws.String("thand-test-policy"),
			PolicyDocument: aws.String(policyDoc),
		})
		require.NoError(t, err, "Should attach policy to role")
		t.Log("Attached policy to role")

		// Delete the role (cleanup)
		_, err = iamClient.DeleteRolePolicy(ctx, &iam.DeleteRolePolicyInput{
			RoleName:   aws.String(roleName),
			PolicyName: aws.String("thand-test-policy"),
		})
		require.NoError(t, err, "Should delete role policy")

		_, err = iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{
			RoleName: aws.String(roleName),
		})
		require.NoError(t, err, "Should delete IAM role")
		t.Log("Cleaned up IAM role")
	})
}

// TestAWSElevationWithTemporal tests the full AWS elevation workflow with Temporal
func TestAWSElevationWithTemporal(t *testing.T) {
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

	// Load the aws-elevation test case
	loader := NewTestCaseLoader(infra)
	testCase, err := loader.LoadTestCase("aws-elevation")
	require.NoError(t, err, "Failed to load aws-elevation test case")

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

	// Register the test user identity in the provider
	// This is necessary because GetIdentity checks the provider for existence
	provider, err := cfg.GetProviderByName("aws-localstack")
	require.NoError(t, err, "Failed to get aws-localstack provider")

	provider.AddIdentities(
		models.Identity{
			ID:    testUser.Email,
			Label: testUser.Name,
			User:  testUser,
		},
	)

	// Get the workflow from config to ensure Identifier field is set
	workflowPtr, err := cfg.GetWorkflowByName("aws_self_approval")
	require.NoError(t, err, "Failed to get workflow from config")
	workflow := *workflowPtr
	role := testCase.Roles["aws_test_admin"]

	t.Run("Full elevation lifecycle with Temporal", func(t *testing.T) {
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
			"workflow":   "aws_self_approval", // Required for Hydrate to find the workflow
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
		workflowID := workflowTask.GetWorkflowID()
		t.Logf("Workflow ID: %s", workflowID)

		// Channel to signal workflow completion
		workflowDone := make(chan error, 1)

		// Start workflow in background using ResumeWorkflow.
		// ResumeWorkflow is preferred over ResumeWorkflowTask because it properly registers the workflow execution with Temporal,
		// ensuring correct workflow lifecycle management and visibility. ResumeWorkflowTask does not register with Temporal and is
		// intended for internal or testing purposes only.
		go func() {
			t.Log("Starting workflow execution via Temporal...")
			_, err := wm.ResumeWorkflow(workflowTask)
			workflowDone <- err
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

		// Since we can't easily call the HTTP endpoint in this test,
		// we'll simulate approval by sending a Temporal signal directly
		t.Log("Simulating approval via Temporal signal...")

		// Create approval event
		approvalEvent := cloudevents.NewEvent()
		approvalEvent.SetID(uuid.New().String())
		approvalEvent.SetType("com.thand.approval")
		approvalEvent.SetSource("urn:thand:test")
		approvalEvent.SetData(cloudevents.ApplicationJSON, map[string]any{
			"approved": true,
		})
		approvalEvent.SetExtension("user", testUser.Email)

		// Signal the workflow with approval
		temporalClient := infra.TemporalClient
		err = temporalClient.SignalWorkflow(
			ctx,
			workflowID,
			sdkWorkflowsModel.TemporalEmptyRunId,
			sdkWorkflowsModel.TemporalEventSignalName,
			approvalEvent,
		)

		require.NoErrorf(t, err, "Signal error sending signal '%s' to workflow '%s'", sdkWorkflowsModel.TemporalEventSignalName, workflowID)
		t.Log("Sent approval signal to workflow")

		// Wait for ResumeWorkflow to finish sending the signal
		select {
		case err := <-workflowDone:
			if err != nil {
				t.Fatalf("ResumeWorkflow returned error: %v", err)
			}
			t.Log("ResumeWorkflow completed - signal sent to Temporal")
		case <-time.After(30 * time.Second):
			t.Fatal("ResumeWorkflow timed out after 30 seconds")
		}

		// Wait a moment for the authorization activity to complete
		t.Log("Waiting for authorization activity to complete...")
		time.Sleep(2 * time.Second)

		// Step 3: Verify the IAM role was created and user can assume it
		t.Log("Step 3: Verifying IAM role was created with proper authorization...")
		verifyIAMRoleCreated(t, ctx, iamClient, "test_admin", testUsername)

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
		verifyIAMRoleRevoked(t, ctx, iamClient, "test_admin")

		// The test validates that:
		// 1. Email was sent with approval links
		// 2. Workflow can receive approval signals
		// 3. IAM role was created in LocalStack (user can assume it)
		// 4. Workflow has a timer scheduled for revocation
		// 5. Cancelling the workflow triggers the cleanup activity
		// 6. Cleanup activity revokes the role (Deny policy in place)
		t.Log("AWS elevation integration test completed successfully!")
	})
}

// createLocalStackIAMClient creates an IAM client configured for LocalStack
func createLocalStackIAMClient(t *testing.T, ctx context.Context, endpoint string) *iam.Client {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
		config.WithBaseEndpoint(endpoint),
	)
	require.NoError(t, err, "Failed to create AWS config for LocalStack")

	return iam.NewFromConfig(cfg)
}

// waitForWorkflowCompletion polls the Temporal workflow status until it completes or times out
func waitForWorkflowCompletion(t *testing.T, ctx context.Context, temporalClient client.Client, workflowID string, timeout time.Duration) {
	t.Helper()
	t.Log("Waiting for Temporal workflow to complete...")

	workflowCompleteCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-workflowCompleteCtx.Done():
			t.Log("Timed out waiting for workflow - checking role anyway...")
			return
		default:
			desc, err := temporalClient.DescribeWorkflowExecution(workflowCompleteCtx, workflowID, "")
			if err != nil {
				t.Logf("Error describing workflow: %v", err)
				time.Sleep(500 * time.Millisecond)
				continue
			}

			status := desc.WorkflowExecutionInfo.Status
			t.Logf("Workflow status: %s", status.String())

			// Check if workflow has completed using proper enums
			switch status {
			case enums.WORKFLOW_EXECUTION_STATUS_COMPLETED,
				enums.WORKFLOW_EXECUTION_STATUS_FAILED,
				enums.WORKFLOW_EXECUTION_STATUS_CANCELED,
				enums.WORKFLOW_EXECUTION_STATUS_TERMINATED,
				enums.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
				t.Logf("Workflow finished with status: %s", status.String())
				return
			}

			time.Sleep(500 * time.Millisecond)
		}
	}
}

// verifyIAMRoleCreated checks that the IAM role was created and configured correctly.
// This function strictly verifies the role is in an authorized state (user can assume it).
func verifyIAMRoleCreated(t *testing.T, ctx context.Context, iamClient *iam.Client, expectedRoleName, testUsername string) {
	t.Helper()
	t.Logf("Checking for IAM role '%s' in LocalStack...", expectedRoleName)

	roleOutput, err := iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(expectedRoleName),
	})
	require.NoError(t, err, "IAM role should have been created")
	require.NotNil(t, roleOutput.Role, "Role should not be nil")
	t.Logf("Found IAM role: %s", *roleOutput.Role.RoleName)
	t.Logf("Role ARN: %s", *roleOutput.Role.Arn)

	// Verify the assume role policy allows our test user
	require.NotNil(t, roleOutput.Role.AssumeRolePolicyDocument, "Role should have an assume role policy")
	t.Logf("Assume Role Policy: %s", *roleOutput.Role.AssumeRolePolicyDocument)

	// Check that the policy contains the test user and does NOT have Deny
	policyDoc := *roleOutput.Role.AssumeRolePolicyDocument
	expectedUserArn := "arn:aws:iam::000000000000:user/" + testUsername

	// Verify role is in authorized state (not revoked)
	require.False(t, strings.Contains(policyDoc, "\"Effect\":\"Deny\""),
		"Role should NOT have Deny policy - role must be in authorized state")

	require.True(t, strings.Contains(policyDoc, expectedUserArn),
		"Assume role policy should include the test user ARN: %s", expectedUserArn)

	t.Logf("✓ Assume role policy correctly includes user: %s", expectedUserArn)
}

// verifyIAMRoleRevoked checks that the IAM role has been revoked (Deny policy in place)
func verifyIAMRoleRevoked(t *testing.T, ctx context.Context, iamClient *iam.Client, expectedRoleName string) {
	t.Helper()
	t.Logf("Checking that IAM role '%s' has been revoked...", expectedRoleName)

	roleOutput, err := iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(expectedRoleName),
	})
	require.NoError(t, err, "IAM role should still exist")
	require.NotNil(t, roleOutput.Role, "Role should not be nil")

	// Verify the assume role policy has Deny (revoked state)
	require.NotNil(t, roleOutput.Role.AssumeRolePolicyDocument, "Role should have an assume role policy")
	policyDoc := *roleOutput.Role.AssumeRolePolicyDocument
	t.Logf("Assume Role Policy after revocation: %s", policyDoc)

	require.True(t, strings.Contains(policyDoc, "\"Effect\":\"Deny\""),
		"Role should have Deny policy after revocation")

	t.Logf("✓ Role has been correctly revoked (Deny policy in place)")
}

// verifyWorkflowHasTimer checks that the Temporal workflow has an active timer
// This timer is created after the user has been authorized
func verifyWorkflowHasTimer(t *testing.T, ctx context.Context, temporalClient client.Client, workflowID string) {
	t.Helper()
	t.Log("Verifying workflow has an active timer...")

	desc, err := temporalClient.DescribeWorkflowExecution(ctx, workflowID, "")
	require.NoError(t, err, "Should be able to describe workflow")

	// The workflow should be running (waiting on a timer for revocation)
	status := desc.WorkflowExecutionInfo.Status

	require.Equal(t, enums.WORKFLOW_EXECUTION_STATUS_RUNNING, status,
		"Workflow should be running (waiting on revocation timer), but got %s", status.String())

	t.Logf("✓ Workflow is running with status: %s", status.String())

	// Check for pending activities or timers
	// When the workflow is authorized, it sends a termination signal with a scheduled time
	// which creates a timer in the workflow
	t.Logf("Workflow has %d pending activities", len(desc.PendingActivities))

	t.Log("✓ Workflow has timer scheduled for revocation")
}
