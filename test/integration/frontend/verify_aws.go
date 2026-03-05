package ui_e2e

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/require"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
)

// CreateLocalStackIAMClient creates an IAM client configured for LocalStack.
func CreateLocalStackIAMClient(t *testing.T, ctx context.Context, endpoint string) *iam.Client {
	t.Helper()

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
		awsconfig.WithBaseEndpoint(endpoint),
	)
	require.NoError(t, err, "Failed to create AWS config for LocalStack")

	return iam.NewFromConfig(cfg)
}

// CreateLocalStackSTSClient creates an STS client configured for LocalStack.
func CreateLocalStackSTSClient(t *testing.T, ctx context.Context, endpoint string) *sts.Client {
	t.Helper()

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
		awsconfig.WithBaseEndpoint(endpoint),
	)
	require.NoError(t, err, "Failed to create AWS config for LocalStack")

	return sts.NewFromConfig(cfg)
}

// CreateTestIAMUser creates a test IAM user in LocalStack.
func CreateTestIAMUser(t *testing.T, ctx context.Context, iamClient *iam.Client, username string) {
	t.Helper()
	t.Logf("Creating test IAM user: %s", username)

	_, err := iamClient.CreateUser(ctx, &iam.CreateUserInput{
		UserName: aws.String(username),
	})
	if err != nil {
		// User may already exist from a previous test run
		var alreadyExists *iamtypes.EntityAlreadyExistsException
		if !errors.As(err, &alreadyExists) {
			require.NoError(t, err, "Failed to create test IAM user")
		}
		t.Logf("IAM user %s already exists, continuing", username)
	} else {
		t.Logf("Created IAM user: %s", username)
	}
}

// DeleteTestIAMUser deletes a test IAM user from LocalStack.
func DeleteTestIAMUser(t *testing.T, ctx context.Context, iamClient *iam.Client, username string) {
	t.Helper()
	t.Logf("Deleting test IAM user: %s", username)

	_, err := iamClient.DeleteUser(ctx, &iam.DeleteUserInput{
		UserName: aws.String(username),
	})
	if err != nil {
		t.Logf("Warning: Failed to delete IAM user %s: %v", username, err)
	}
}

// VerifyIAMRoleCreated checks that the IAM role was created and configured correctly.
// The role should be in an authorized state (user can assume it, no Deny policy).
func VerifyIAMRoleCreated(t *testing.T, ctx context.Context, iamClient *iam.Client, expectedRoleName, testUsername string) {
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

	policyDoc := *roleOutput.Role.AssumeRolePolicyDocument
	expectedUserArn := "arn:aws:iam::000000000000:user/" + testUsername

	// Verify role is in authorized state (not revoked)
	require.False(t, strings.Contains(policyDoc, "\"Effect\":\"Deny\""),
		"Role should NOT have Deny policy — role must be in authorized state")

	require.True(t, strings.Contains(policyDoc, expectedUserArn),
		"Assume role policy should include the test user ARN: %s", expectedUserArn)

	t.Logf("Assume role policy correctly includes user: %s", expectedUserArn)
}

// VerifyIAMRoleRevoked checks that the IAM role has been revoked (Deny policy in place).
func VerifyIAMRoleRevoked(t *testing.T, ctx context.Context, iamClient *iam.Client, expectedRoleName string) {
	t.Helper()
	t.Logf("Checking that IAM role '%s' has been revoked...", expectedRoleName)

	roleOutput, err := iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(expectedRoleName),
	})
	require.NoError(t, err, "IAM role should still exist")
	require.NotNil(t, roleOutput.Role, "Role should not be nil")

	require.NotNil(t, roleOutput.Role.AssumeRolePolicyDocument, "Role should have an assume role policy")
	policyDoc := *roleOutput.Role.AssumeRolePolicyDocument
	t.Logf("Assume Role Policy after revocation: %s", policyDoc)

	require.True(t, strings.Contains(policyDoc, "\"Effect\":\"Deny\""),
		"Role should have Deny policy after revocation")

	t.Logf("Role has been correctly revoked (Deny policy in place)")
}

// VerifyIAMRoleAbsent checks that the IAM role does not exist after a denial.
// After a workflow is denied, no IAM role should have been provisioned.
func VerifyIAMRoleAbsent(t *testing.T, ctx context.Context, iamClient *iam.Client, roleName string) {
	t.Helper()
	t.Logf("Verifying IAM role '%s' does not exist after denial...", roleName)

	_, err := iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})
	require.Error(t, err, "IAM role should not exist after denial, but role '%s' was found", roleName)
	var noSuchEntity *iamtypes.NoSuchEntityException
	require.True(t, errors.As(err, &noSuchEntity),
		"Expected NoSuchEntityException for role '%s', got: %v", roleName, err)
	t.Logf("Confirmed: IAM role '%s' does not exist (as expected after denial)", roleName)
}

// VerifyAssumeRole calls STS AssumeRole and verifies temporary credentials are returned.
func VerifyAssumeRole(t *testing.T, ctx context.Context, stsClient *sts.Client, roleARN, sessionName string) *sts.AssumeRoleOutput {
	t.Helper()
	t.Logf("Assuming role: %s", roleARN)

	output, err := stsClient.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleARN),
		RoleSessionName: aws.String(sessionName),
	})
	require.NoError(t, err, "Should be able to assume the role")
	require.NotNil(t, output.Credentials, "AssumeRole should return credentials")
	require.NotEmpty(t, *output.Credentials.AccessKeyId, "Should have an access key ID")
	require.NotEmpty(t, *output.Credentials.SecretAccessKey, "Should have a secret access key")
	require.NotEmpty(t, *output.Credentials.SessionToken, "Should have a session token")

	t.Logf("Successfully assumed role, access key: %s...", (*output.Credentials.AccessKeyId)[:8])
	return output
}

// WaitForWorkflowCompletion polls the Temporal workflow status until it reaches a terminal state.
func WaitForWorkflowCompletion(t *testing.T, ctx context.Context, temporalClient client.Client, workflowID string, timeout time.Duration) enums.WorkflowExecutionStatus {
	t.Helper()
	t.Log("Waiting for Temporal workflow to complete...")

	workflowCompleteCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-workflowCompleteCtx.Done():
			t.Log("Timed out waiting for workflow completion")
			return enums.WORKFLOW_EXECUTION_STATUS_RUNNING
		default:
			desc, err := temporalClient.DescribeWorkflowExecution(workflowCompleteCtx, workflowID, "")
			if err != nil {
				t.Logf("Error describing workflow: %v", err)
				time.Sleep(500 * time.Millisecond)
				continue
			}

			status := desc.WorkflowExecutionInfo.Status
			t.Logf("Workflow status: %s", status.String())

			switch status {
			case enums.WORKFLOW_EXECUTION_STATUS_COMPLETED,
				enums.WORKFLOW_EXECUTION_STATUS_FAILED,
				enums.WORKFLOW_EXECUTION_STATUS_CANCELED,
				enums.WORKFLOW_EXECUTION_STATUS_TERMINATED,
				enums.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
				t.Logf("Workflow finished with status: %s", status.String())
				return status
			}

			time.Sleep(500 * time.Millisecond)
		}
	}
}

// VerifyWorkflowRunning checks that the Temporal workflow is currently running
// (e.g. waiting on a revocation timer after authorization).
func VerifyWorkflowRunning(t *testing.T, ctx context.Context, temporalClient client.Client, workflowID string) {
	t.Helper()
	t.Log("Verifying workflow is running...")

	desc, err := temporalClient.DescribeWorkflowExecution(ctx, workflowID, "")
	require.NoError(t, err, "Should be able to describe workflow")

	status := desc.WorkflowExecutionInfo.Status
	require.Equal(t, enums.WORKFLOW_EXECUTION_STATUS_RUNNING, status,
		"Workflow should be running (waiting on revocation timer), but got %s", status.String())

	t.Logf("Workflow is running with status: %s", status.String())
}

// CancelWorkflow signals a Temporal workflow to cancel.
func CancelWorkflow(t *testing.T, ctx context.Context, temporalClient client.Client, workflowID string) {
	t.Helper()
	t.Logf("Cancelling workflow: %s", workflowID)

	err := temporalClient.CancelWorkflow(ctx, workflowID, "")
	require.NoError(t, err, "Should be able to cancel workflow")

	t.Log("Workflow cancellation signal sent")
}

// WaitForIAMRole polls for an IAM role to appear with a timeout.
// Returns once the role exists or times out.
func WaitForIAMRole(t *testing.T, ctx context.Context, iamClient *iam.Client, roleName string, timeout time.Duration) {
	t.Helper()
	t.Logf("Waiting for IAM role '%s' to appear...", roleName)

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, err := iamClient.GetRole(ctx, &iam.GetRoleInput{
			RoleName: aws.String(roleName),
		})
		if err == nil {
			t.Logf("IAM role '%s' found", roleName)
			return
		}
		time.Sleep(1 * time.Second)
	}

	t.Logf("Warning: Timed out waiting for IAM role '%s'", roleName)
}
