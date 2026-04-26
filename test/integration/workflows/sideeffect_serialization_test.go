package workflows_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// buildActivityFn is a named activity function used in tests to verify that
// AuthorizeRoleRequest (with its CompositeRole) survives activity serialization.
// Using a named function (rather than an anonymous closure) gives Temporal a
// stable activity type name for the test history.
func buildActivityFn(ctx context.Context, role *models.CompositeRole) (*models.AuthorizeRoleRequest, error) {
	return &models.AuthorizeRoleRequest{
		Role: role,
		Identity: &models.Identity{
			ID: "test-user",
			User: &models.User{
				Email: "test@example.com",
			},
		},
	}, nil
}

// TestProvisioningActivityCompositeRoleSerialization tests that CompositeRole fields
// (UUID, Composite, Providers) survive a Temporal activity serialization round-trip.
// This replaces the earlier SideEffect-based serialization test and covers the same
// correctness concern for the new local-activity approach used in
// CreateProviderAuthorizeRoleWorkflow / CreateProviderRevokeRoleWorkflow.
func TestProvisioningActivityCompositeRoleSerialization(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	expectedUUID := uuid.MustParse("656c50df-6127-5b30-b6f8-f8037531e07f")
	expectedProviders := []string{"aws-prod", "aws-dev", "aws-thand-dev"}
	expectedComposite := true

	env.RegisterActivityWithOptions(buildActivityFn, activity.RegisterOptions{
		Name: "buildActivityFn",
	})

	testWorkflow := func(ctx workflow.Context) (*models.AuthorizeRoleRequest, error) {
		compositeRole := &models.CompositeRole{
			UUID:      expectedUUID,
			Providers: expectedProviders,
			Composite: expectedComposite,
			Role: models.Role{
				Identifier:  "aws_admin",
				Name:        "Admin",
				Description: "Full access",
				Providers:   []string{"aws-prod", "aws-dev", "aws-thand-dev"},
				Enabled:     true,
			},
		}

		lao := workflow.LocalActivityOptions{
			StartToCloseTimeout: 30 * time.Second,
		}
		lctx := workflow.WithLocalActivityOptions(ctx, lao)

		var result models.AuthorizeRoleRequest
		if err := workflow.ExecuteLocalActivity(lctx, buildActivityFn, compositeRole).Get(ctx, &result); err != nil {
			return nil, err
		}
		return &result, nil
	}

	env.ExecuteWorkflow(testWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result *models.AuthorizeRoleRequest
	require.NoError(t, env.GetWorkflowResult(&result))
	require.NotNil(t, result)
	require.NotNil(t, result.Role)

	t.Logf("Expected UUID: %s", expectedUUID.String())
	t.Logf("Actual UUID:   %s", result.Role.UUID.String())
	assert.Equal(t, expectedUUID, result.Role.UUID, "UUID should be preserved through activity serialization")

	t.Logf("Expected Composite: %v", expectedComposite)
	t.Logf("Actual Composite:   %v", result.Role.Composite)
	assert.Equal(t, expectedComposite, result.Role.Composite, "Composite flag should be preserved through activity serialization")

	t.Logf("Expected Providers: %v", expectedProviders)
	t.Logf("Actual Providers:   %v", result.Role.Providers)
	assert.Equal(t, expectedProviders, result.Role.Providers, "Providers should be preserved through activity serialization")
}
