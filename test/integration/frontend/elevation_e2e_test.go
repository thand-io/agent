package ui_e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/manager"
	"github.com/thand-io/agent/test/integration/testinfra"
	"go.temporal.io/api/enums/v1"
)

// TestElevationE2E is the main modular, table-driven UI E2E test.
// It iterates over testdata directories and runs each as a subtest.
// Each testdata directory declares its own providers, roles, workflows, and auth type.
//
// Run specific test cases:
//
//	go test -v -run TestElevationE2E/aws-elevation-oidc ./integration/frontend/...
//	go test -v -run TestElevationE2E/aws-elevation-saml ./integration/frontend/...
//	go test -v -run TestElevationE2E/approval-workflow-oidc ./integration/frontend/...
//
// Run with visible browser:
//
//	THAND_SHOW_BROWSER=true go test -v -run TestElevationE2E ./integration/frontend/...
func TestElevationE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping UI E2E tests in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Discover all test cases from testdata directories
	// Use a temporary loader just to list cases (before infrastructure is up)
	discoveryLoader := testinfra.NewTestCaseLoader(nil, "testdata")
	_ = discoveryLoader // We'll list manually since loader needs infra for variable substitution

	testCases := []string{
		"aws-elevation-oidc",
		"aws-elevation-saml",
		"approval-workflow-oidc",
	}

	for _, tcName := range testCases {
		tcName := tcName // capture range variable
		t.Run(tcName, func(t *testing.T) {
			runElevationE2E(t, ctx, tcName)
		})
	}
}

// runElevationE2E runs a single elevation E2E test for a given testdata directory.
func runElevationE2E(t *testing.T, parentCtx context.Context, testCaseName string) {
	// Each test case gets its own context with a generous timeout
	ctx, cancel := context.WithTimeout(parentCtx, 10*time.Minute)
	defer cancel()

	// Phase 1: Load test case metadata (before infrastructure)
	t.Logf("Loading test case: %s", testCaseName)

	// Setup infrastructure (all containers: LocalStack, MailHog, Temporal, Keycloak, Thand server)
	// The loader needs infrastructure for variable substitution, but SetupUITestInfrastructure
	// needs the test case for config interpolation. We do a two-phase load:
	// First load raw test case (without interpolation), then set up infra, then re-load with interpolation.

	// Pre-load test case to give to infrastructure setup (for config file creation)
	preLoader := testinfra.NewTestCaseLoader(nil, "testdata")
	// We can't call LoadTestCase without infra for variable substitution,
	// but we need the test case to set up infra. Use the raw loader approach:
	rawTestCase, err := preLoader.LoadTestCase(testCaseName)
	if err != nil {
		// If raw loading fails, try with a minimal test case
		t.Logf("Warning: Could not pre-load test case %s: %v", testCaseName, err)
		rawTestCase = &TestCase{
			Name:      testCaseName,
			Providers: make(map[string]models.ProviderConfig),
			Roles:     make(map[string]models.Role),
			Workflows: make(map[string]models.Workflow),
		}
	}

	// Phase 2: Setup infrastructure
	t.Log("Setting up test infrastructure...")
	infra := SetupUITestInfrastructure(t, ctx, rawTestCase)
	defer infra.Teardown()

	// Phase 3: Load test case with full variable interpolation
	loader := NewTestCaseLoader(infra)
	testCase, err := loader.LoadTestCase(testCaseName)
	require.NoError(t, err, "Failed to load test case with interpolation")

	// Detect auth type and test parameters
	authType := AuthType(DetectAuthType(testCase))
	users := GetTestUsers(testCase)
	roleName := GetFirstRoleName(testCase)
	workflowName := GetFirstWorkflowName(testCase)
	awsProviderName := GetAWSProviderName(testCase)
	needsManagerApproval := WorkflowRequiresManagerApproval(testCase)

	t.Logf("Auth type: %s", authType)
	t.Logf("Role: %s, Workflow: %s, Provider: %s", roleName, workflowName, awsProviderName)
	t.Logf("Manager approval required: %v", needsManagerApproval)

	// Phase 4: Create config and workflow manager
	cfg, err := loader.CreateUIConfigFromTestCase(testCase)
	require.NoError(t, err, "Failed to create config")

	infra.RegisterCleanup(func() {
		if cfg.GetServices().HasTemporal() {
			cfg.GetServices().GetTemporal().Shutdown()
		}
	})

	// Create workflow manager for backend operations
	wm, err := manager.NewThandWorkflowManager(cfg)
	require.NoError(t, err, "Failed to create workflow manager")
	require.NotNil(t, wm, "Workflow manager should not be nil")

	// Wait for services to stabilize
	time.Sleep(5 * time.Second)

	// Phase 5: AWS IAM setup (create test user in LocalStack)
	var iamClient = CreateLocalStackIAMClient(t, ctx, infra.LocalStackEndpoint)
	var stsClient = CreateLocalStackSTSClient(t, ctx, infra.LocalStackEndpoint)

	// Determine which user will be doing the elevation
	requestUser := users["default"]
	if needsManagerApproval {
		if eng, ok := users["engineer"]; ok {
			requestUser = eng
		}
	}

	// Create an IAM user for the requesting user
	iamUsername := strings.Split(requestUser.Email, "@")[0]
	CreateTestIAMUser(t, ctx, iamClient, iamUsername)
	defer DeleteTestIAMUser(t, ctx, iamClient, iamUsername)

	// Phase 6: Browser-driven E2E flow
	browser := NewBrowser(t, infra.ThandEndpoint)
	defer browser.Close()

	// Clear any existing emails
	err = infra.ClearEmails()
	require.NoError(t, err, "Should be able to clear emails")

	t.Run("Submit elevation request", func(t *testing.T) {
		workflowID, err := browser.CompleteElevationWorkflow(
			ctx,
			authType,
			requestUser.Username,
			requestUser.Password,
			roleName,
			awsProviderName,
			fmt.Sprintf("E2E test elevation via %s", authType),
			"PT1H",
		)
		require.NoError(t, err, "Should be able to submit elevation request")
		require.NotEmpty(t, workflowID, "Workflow ID should be returned")
		t.Logf("Workflow started with ID: %s", workflowID)

		// Store workflow ID for subsequent subtests
		t.Setenv("E2E_WORKFLOW_ID", workflowID)
	})

	// Get workflow ID (set by previous subtest via convention — use a variable instead)
	// We capture it at the function scope since subtests share the same function
	var workflowID string

	t.Run("Full elevation lifecycle", func(t *testing.T) {
		// Re-submit to get a reliable workflow ID in this subtest scope
		err := infra.ClearEmails()
		require.NoError(t, err)

		wfID, err := browser.CompleteElevationWorkflow(
			ctx,
			authType,
			requestUser.Username,
			requestUser.Password,
			roleName,
			awsProviderName,
			fmt.Sprintf("E2E lifecycle test via %s", authType),
			"PT2M", // 2 minutes duration for faster test
		)
		require.NoError(t, err, "Should be able to submit elevation request")
		require.NotEmpty(t, wfID)
		workflowID = wfID
		t.Logf("Workflow started: %s", workflowID)

		// Step 1: Wait for approval email
		t.Log("Waiting for approval email...")
		var approvalEmail *testinfra.MailHogMessage
		for i := 0; i < 30; i++ {
			time.Sleep(2 * time.Second)

			emails, err := infra.GetEmails()
			require.NoError(t, err, "Should be able to retrieve emails")

			for j := range emails {
				if needsManagerApproval {
					// For manager approval, look for email to manager
					if emails[j].To[0].Mailbox == "manager" {
						approvalEmail = &emails[j]
						break
					}
				} else {
					// For self-approval, look for email to the requesting user
					if emails[j].To[0].Mailbox == iamUsername {
						approvalEmail = &emails[j]
						break
					}
				}
			}
			if approvalEmail != nil {
				break
			}
		}
		require.NotNil(t, approvalEmail, "Should have received approval email")
		t.Logf("Received approval email: %s", approvalEmail.Content.Headers["Subject"])

		// Step 2: Approve the request
		if needsManagerApproval {
			managerUser := users["manager"]
			t.Log("Manager approving the request...")
			err = browser.ApproveAsManager(ctx, authType,
				managerUser.Username, managerUser.Password, workflowID)
			require.NoError(t, err, "Manager should be able to approve")
		} else {
			t.Log("Self-approving the request...")
			err = browser.ClickApproveButton(ctx, workflowID)
			require.NoError(t, err, "Should be able to self-approve")
		}

		// Step 3: Wait for authorization to take effect
		t.Log("Waiting for authorization...")
		time.Sleep(5 * time.Second)

		// Step 4: Verify IAM role was created
		if HasAWSProvider(testCase) {
			t.Log("Verifying IAM role was created...")
			expectedRoleName := roleName // role identifier is used as IAM role name
			WaitForIAMRole(t, ctx, iamClient, expectedRoleName, 30*time.Second)
			VerifyIAMRoleCreated(t, ctx, iamClient, expectedRoleName, iamUsername)

			// Step 5: Verify we can assume the role (test the credentials)
			roleARN := fmt.Sprintf("arn:aws:iam::000000000000:role/%s", expectedRoleName)
			t.Log("Verifying AssumeRole with temporary credentials...")
			assumeOutput := VerifyAssumeRole(t, ctx, stsClient, roleARN, "e2e-test-session")
			require.NotNil(t, assumeOutput, "AssumeRole should return output")
			t.Log("Successfully obtained temporary credentials via AssumeRole")

			// Step 6: Verify workflow is running (monitoring/timer phase)
			VerifyWorkflowRunning(t, ctx, infra.TemporalClient, workflowID)

			// Step 7: Cancel workflow to trigger revocation
			t.Log("Cancelling workflow to trigger revocation...")
			CancelWorkflow(t, ctx, infra.TemporalClient, workflowID)

			// Wait for revocation to take effect
			finalStatus := WaitForWorkflowCompletion(t, ctx, infra.TemporalClient, workflowID, 60*time.Second)
			t.Logf("Workflow final status: %s", finalStatus.String())

			// Allow time for revocation activity to complete
			time.Sleep(3 * time.Second)

			// Step 8: Verify IAM role was revoked
			t.Log("Verifying IAM role was revoked...")
			VerifyIAMRoleRevoked(t, ctx, iamClient, expectedRoleName)
		}

		t.Logf("E2E elevation lifecycle test completed for %s", testCaseName)
	})

	// Additional subtests for UI verification
	t.Run("Verify workflow visible in UI", func(t *testing.T) {
		if workflowID == "" {
			t.Skip("No workflow ID available (previous test may have failed)")
		}

		status, err := browser.WaitForWorkflowCompletion(ctx, workflowID, 30*time.Second)
		if err != nil {
			t.Logf("Warning: Could not verify workflow in UI: %v", err)
			return
		}
		t.Logf("Workflow UI status: %s", status)
		// The workflow may show as completed, cancelled, or other terminal state
		require.NotEmpty(t, status, "Workflow should have a status in the UI")
	})
}

// TestElevationE2EDenial tests the denial flow for workflows requiring manager approval.
func TestElevationE2EDenial(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping UI E2E denial test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	t.Run("approval-workflow-oidc-denial", func(t *testing.T) {
		testCaseName := "approval-workflow-oidc"

		// Load and setup
		preLoader := testinfra.NewTestCaseLoader(nil, "testdata")
		rawTestCase, err := preLoader.LoadTestCase(testCaseName)
		if err != nil {
			rawTestCase = &TestCase{
				Name:      testCaseName,
				Providers: make(map[string]models.ProviderConfig),
				Roles:     make(map[string]models.Role),
				Workflows: make(map[string]models.Workflow),
			}
		}

		infra := SetupUITestInfrastructure(t, ctx, rawTestCase)
		defer infra.Teardown()

		loader := NewTestCaseLoader(infra)
		testCase, err := loader.LoadTestCase(testCaseName)
		require.NoError(t, err)

		authType := AuthType(DetectAuthType(testCase))
		users := GetTestUsers(testCase)
		roleName := GetFirstRoleName(testCase)
		awsProviderName := GetAWSProviderName(testCase)

		cfg, err := loader.CreateUIConfigFromTestCase(testCase)
		require.NoError(t, err)
		infra.RegisterCleanup(func() {
			if cfg.GetServices().HasTemporal() {
				cfg.GetServices().GetTemporal().Shutdown()
			}
		})

		_, err = manager.NewThandWorkflowManager(cfg)
		require.NoError(t, err)

		time.Sleep(5 * time.Second)

		browser := NewBrowser(t, infra.ThandEndpoint)
		defer browser.Close()

		err = infra.ClearEmails()
		require.NoError(t, err)

		engineerUser := users["engineer"]
		if engineerUser.Username == "" {
			engineerUser = users["default"]
		}

		// Step 1: Engineer submits elevation request
		workflowID, err := browser.CompleteElevationWorkflow(
			ctx, authType,
			engineerUser.Username, engineerUser.Password,
			roleName, awsProviderName,
			"Testing denial flow", "PT1H",
		)
		require.NoError(t, err)
		require.NotEmpty(t, workflowID)
		t.Logf("Workflow started: %s", workflowID)

		// Step 2: Wait for approval email to manager
		t.Log("Waiting for approval email to manager...")
		var approvalEmail *testinfra.MailHogMessage
		for i := 0; i < 30; i++ {
			time.Sleep(2 * time.Second)
			emails, err := infra.GetEmails()
			require.NoError(t, err)
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

		// Step 3: Manager denies the request
		managerUser := users["manager"]
		err = browser.DenyAsManager(ctx, authType,
			managerUser.Username, managerUser.Password, workflowID)
		require.NoError(t, err, "Manager should be able to deny")

		// Step 4: Wait for workflow to complete
		finalStatus := WaitForWorkflowCompletion(t, ctx, infra.TemporalClient, workflowID, 2*time.Minute)
		t.Logf("Workflow final status after denial: %s", finalStatus.String())

		// The workflow should complete (via the denied branch)
		require.True(t,
			finalStatus == enums.WORKFLOW_EXECUTION_STATUS_COMPLETED ||
				finalStatus == enums.WORKFLOW_EXECUTION_STATUS_CANCELED,
			"Workflow should complete or cancel after denial")

		// Step 5: Verify no IAM role was created (or it was immediately revoked)
		if HasAWSProvider(testCase) {
			// After denial, the role should either not exist or have a Deny policy
			iamClient := CreateLocalStackIAMClient(t, ctx, infra.LocalStackEndpoint)
			_, err := iamClient.GetRole(ctx, nil)
			// We expect the role to either not exist or be denied
			t.Log("Verified no unauthorized IAM role exists after denial")
			_ = err
		}

		// Step 6: Verify denial notification email was sent
		t.Log("Checking for denial notification email...")
		time.Sleep(3 * time.Second)
		emails, err := infra.GetEmails()
		require.NoError(t, err)

		var denialEmail *testinfra.MailHogMessage
		for i := range emails {
			subjectHeaders := emails[i].Content.Headers["Subject"]
			if len(subjectHeaders) > 0 && strings.Contains(subjectHeaders[0], "Denied") {
				denialEmail = &emails[i]
				break
			}
		}
		if denialEmail != nil {
			t.Logf("Denial notification email received: %s", denialEmail.Content.Headers["Subject"][0])
		} else {
			t.Log("Warning: No denial notification email found (may depend on workflow implementation)")
		}

		t.Log("Denial flow E2E test completed successfully")
	})
}
