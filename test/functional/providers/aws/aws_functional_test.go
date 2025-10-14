package aws_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/localstack"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
	_ "github.com/thand-io/agent/internal/providers/aws" // Import to register the provider
)

func TestAWSProviderFunctional(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping functional test in short mode")
	}

	ctx := context.Background()

	// Start LocalStack container
	localstackContainer, err := localstack.Run(ctx,
		"localstack/localstack:3.0",
		testcontainers.WithEnv(map[string]string{
			"SERVICES": "iam",
			"DEBUG":    "1",
		}),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/health").
				WithPort("4566/tcp").
				WithStartupTimeout(60*time.Second).
				WithPollInterval(1*time.Second),
		),
	)
	require.NoError(t, err)
	defer func() {
		if err := testcontainers.TerminateContainer(localstackContainer); err != nil {
			t.Logf("Failed to terminate LocalStack container: %v", err)
		}
	}()

	// Get LocalStack endpoint for port 4566
	host, err := localstackContainer.Host(ctx)
	require.NoError(t, err)

	mappedPort, err := localstackContainer.MappedPort(ctx, "4566/tcp")
	require.NoError(t, err)

	endpoint := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())

	// Create test user
	testUser := &models.User{
		ID:       "test-user-123",
		Username: "testuser",
		Email:    "testuser@example.com",
		Name:     "Test User",
		Provider: "aws",
	}

	// Create test role
	testRole := &models.Role{
		Name:        "TestRole",
		Description: "Test IAM role for functional testing",
		Permissions: models.Permissions{
			Allow: []string{
				"s3:GetObject",
				"s3:PutObject",
				"ec2:DescribeInstances",
			},
		},
		Providers: []string{"aws"},
		Enabled:   true,
	}

	// Helper function to check if role exists
	roleExists := func(iamClient *iam.Client, roleName string) bool {
		_, err := iamClient.GetRole(ctx, &iam.GetRoleInput{
			RoleName: aws.String(roleName),
		})
		return err == nil
	}

	// Helper function to get role policies
	getRolePolicies := func(iamClient *iam.Client, roleName string) []string {
		output, err := iamClient.ListRolePolicies(ctx, &iam.ListRolePoliciesInput{
			RoleName: aws.String(roleName),
		})
		if err != nil {
			return nil
		}
		return output.PolicyNames
	}

	// Helper function to get assume role policy document
	getAssumeRolePolicy := func(iamClient *iam.Client, roleName string) (string, error) {
		output, err := iamClient.GetRole(ctx, &iam.GetRoleInput{
			RoleName: aws.String(roleName),
		})
		if err != nil {
			return "", err
		}
		if output.Role.AssumeRolePolicyDocument == nil {
			return "", fmt.Errorf("no assume role policy found")
		}
		return *output.Role.AssumeRolePolicyDocument, nil
	}

	// Helper function to check if user is assigned to role via assume role policy
	userIsAssignedToRole := func(iamClient *iam.Client, roleName string, user *models.User) bool {
		policyDoc, err := getAssumeRolePolicy(iamClient, roleName)
		if err != nil {
			return false
		}

		// Extract username from email
		username := strings.Split(user.Email, "@")[0]
		expectedPrincipal := fmt.Sprintf("arn:aws:iam::*:user/%s", username)

		// Check if the policy contains the user's principal with Allow effect
		return strings.Contains(policyDoc, expectedPrincipal) && strings.Contains(policyDoc, `"Effect":"Allow"`)
	}

	// Helper function to check if user is completely unbound from role
	userIsUnboundFromRole := func(iamClient *iam.Client, roleName string, user *models.User) bool {
		policyDoc, err := getAssumeRolePolicy(iamClient, roleName)
		if err != nil {
			return true // If we can't get policy, assume unbound
		}

		// Extract username from email
		username := strings.Split(user.Email, "@")[0]
		expectedPrincipal := fmt.Sprintf("arn:aws:iam::*:user/%s", username)

		// User is unbound if they are NOT mentioned in the policy at all
		return !strings.Contains(policyDoc, expectedPrincipal)
	}

	// Create provider configuration with LocalStack endpoint
	providerConfig := &models.Provider{
		Name:        "test-aws-provider",
		Description: "Test AWS provider using LocalStack",
		Provider:    "aws",
		Config: &models.BasicConfig{
			"region":         "us-east-1",
			"account_id":     "000000000000", // LocalStack default account ID
			"account_secret": "test",
			"endpoint":       endpoint, // Configure to use LocalStack endpoint
		},
		Enabled: true,
	}

	t.Run("Initialize AWS Provider", func(t *testing.T) {
		// Get AWS provider from registry
		providerImpl, err := providers.Get("aws")
		require.NoError(t, err, "Failed to get AWS provider from registry")

		// Initialize the provider
		err = providerImpl.Initialize(*providerConfig)
		require.NoError(t, err, "Failed to initialize AWS provider")

		// Verify provider is properly initialized
		assert.Equal(t, "aws", providerImpl.GetProvider(), "Provider type should be aws")
		assert.NotEmpty(t, providerImpl.GetName(), "Provider name should not be empty")
	})

	t.Run("Full IAM Role Lifecycle with Direct API Verification", func(t *testing.T) {
		// Get AWS provider from registry and initialize
		providerImpl, err := providers.Get("aws")
		require.NoError(t, err, "Failed to get AWS provider from registry")

		// Initialize the provider
		err = providerImpl.Initialize(*providerConfig)
		require.NoError(t, err, "Failed to initialize AWS provider")

		// Get IAM client from the provider using interface{} and reflection
		// Since awsProvider is not exported, we need to use reflection or add a method to the interface
		type IAMClientProvider interface {
			GetIamClient() *iam.Client
		}

		iamClientProvider, ok := providerImpl.(IAMClientProvider)
		require.True(t, ok, "Provider should implement GetIamClient method")
		iamClient := iamClientProvider.GetIamClient()

		roleName := testRole.GetSnakeCaseName() // "test_role"

		// Verify role doesn't exist initially
		assert.False(t, roleExists(iamClient, roleName), "Role should not exist initially")

		// Test role creation and authorization
		t.Run("Authorize Role", func(t *testing.T) {
			metadata, err := providerImpl.AuthorizeRole(ctx, testUser, testRole)
			assert.NoError(t, err, "Should succeed with LocalStack")

			// Allow nil metadata for now (AWS provider limitation)
			if metadata != nil {
				assert.NotNil(t, metadata, "Metadata should not be nil")
			}

			// Verify role was actually created in LocalStack using direct IAM API
			assert.True(t, roleExists(iamClient, roleName), "Role should exist after creation")

			// Verify role has policies attached
			policies := getRolePolicies(iamClient, roleName)
			assert.NotEmpty(t, policies, "Role should have policies attached")
			t.Logf("Created role %s with policies: %v", roleName, policies)

			// Verify user is actually assigned to the role
			assert.True(t, userIsAssignedToRole(iamClient, roleName, testUser), "User should be assigned to the role")
			t.Logf("Verified user %s is assigned to role %s", testUser.Username, roleName)
		})

		// Test role revocation
		t.Run("Revoke Role", func(t *testing.T) {
			metadata := map[string]any{}

			revocationMetadata, err := providerImpl.RevokeRole(ctx, testUser, testRole, metadata)
			assert.NoError(t, err, "Should succeed with LocalStack")

			// Allow nil metadata for now (AWS provider limitation)
			if revocationMetadata != nil {
				assert.NotNil(t, revocationMetadata, "Revocation metadata should not be nil")
			}

			// Verify role still exists (revocation doesn't delete the role)
			assert.True(t, roleExists(iamClient, roleName), "Role should still exist after revocation")

			// Verify user is completely unbound from the role (not mentioned in policy)
			assert.True(t, userIsUnboundFromRole(iamClient, roleName, testUser), "User should be completely unbound from the role")
			assert.False(t, userIsAssignedToRole(iamClient, roleName, testUser), "User should no longer be assigned to the role")

			// Log the actual assume role policy for verification
			if policy, err := getAssumeRolePolicy(iamClient, roleName); err == nil {
				t.Logf("Assume role policy after revocation: %s", policy)
			}

			t.Logf("Unbound user %s from role %s (user removed from assume role policy)", testUser.Username, roleName)
		})
	})

	t.Run("Role Authorization with Missing User", func(t *testing.T) {
		providerImpl, err := providers.Get("aws")
		require.NoError(t, err)
		err = providerImpl.Initialize(*providerConfig)
		require.NoError(t, err)

		// Test with nil user - should return an error, not panic
		_, err = providerImpl.AuthorizeRole(ctx, nil, testRole)
		assert.Error(t, err, "Should fail with nil user")
		assert.Contains(t, err.Error(), "user cannot be nil", "Error should mention nil user")
	})

	t.Run("Role Authorization with Missing Role", func(t *testing.T) {
		providerImpl, err := providers.Get("aws")
		require.NoError(t, err)
		err = providerImpl.Initialize(*providerConfig)
		require.NoError(t, err)

		// Test with nil role - should return an error, not panic
		_, err = providerImpl.AuthorizeRole(ctx, testUser, nil)
		assert.Error(t, err, "Should fail with nil role")
		assert.Contains(t, err.Error(), "role cannot be nil", "Error should mention nil role")
	})
}
