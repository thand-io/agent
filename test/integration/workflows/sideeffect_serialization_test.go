package workflows_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// TestWorkflowSideEffectCompositeRoleSerialization tests that workflow.SideEffect properly
// serializes/deserializes CompositeRole with all fields intact.
// This reproduces the bug where UUID, Composite, and Providers fields were lost during
// workflow.SideEffect serialization/deserialization.
func TestWorkflowSideEffectCompositeRoleSerialization(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Create expected values
	expectedUUID := uuid.MustParse("656c50df-6127-5b30-b6f8-f8037531e07f")
	expectedProviders := []string{"aws-prod", "aws-dev", "aws-thand-dev"}
	expectedComposite := true

	// Define a test workflow that uses SideEffect exactly like CreateProviderAuthorizeRoleWorkflow
	testWorkflow := func(ctx workflow.Context) (*models.CompositeRole, error) {
		// This mimics what happens in CreateProviderAuthorizeRoleWorkflow
		type sideEffectResult struct {
			Role *models.CompositeRole `json:"role"`
			Err  string                `json:"error"`
		}

		encodedReq := workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
			// Create the CompositeRole inside SideEffect (non-deterministic operation)
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
			return sideEffectResult{Role: compositeRole}
		})

		var se sideEffectResult
		if err := encodedReq.Get(&se); err != nil {
			return nil, err
		}

		// Return the deserialized role
		return se.Role, nil
	}

	env.ExecuteWorkflow(testWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result *models.CompositeRole
	require.NoError(t, env.GetWorkflowResult(&result))
	require.NotNil(t, result)

	// Verify all fields survived the SideEffect serialization/deserialization
	t.Logf("Expected UUID: %s", expectedUUID.String())
	t.Logf("Actual UUID:   %s", result.UUID.String())
	assert.Equal(t, expectedUUID, result.UUID, "UUID should be preserved through SideEffect")

	t.Logf("Expected Composite: %v", expectedComposite)
	t.Logf("Actual Composite:   %v", result.Composite)
	assert.Equal(t, expectedComposite, result.Composite, "Composite flag should be preserved through SideEffect")

	t.Logf("Expected Providers: %v", expectedProviders)
	t.Logf("Actual Providers:   %v", result.Providers)
	assert.Equal(t, expectedProviders, result.Providers, "Providers should be preserved through SideEffect")
}

// TestWorkflowSideEffectWithAuthRequest tests the actual authorizeRoleRequestSideEffect struct
// used in the real workflow to verify the serialization issue and its fix.
func TestWorkflowSideEffectWithAuthRequest(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	expectedUUID := uuid.MustParse("656c50df-6127-5b30-b6f8-f8037531e07f")
	expectedProviders := []string{"aws-prod", "aws-dev", "aws-thand-dev"}
	expectedComposite := true

	// Define the same wrapper struct used in the actual workflow
	type authorizeRoleRequestSideEffect struct {
		Request *models.AuthorizeRoleRequest `json:"request"`
		Err     string                       `json:"error"`
	}

	testWorkflow := func(ctx workflow.Context) (*models.AuthorizeRoleRequest, error) {
		log := workflow.GetLogger(ctx)

		// Mimic exactly what CreateProviderAuthorizeRoleWorkflow does
		encodedReq := workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
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

			authReq := &models.AuthorizeRoleRequest{
				Role: compositeRole,
				Identity: &models.Identity{
					ID: "test-user",
					User: &models.User{
						Email: "test@example.com",
					},
				},
			}

			// Log before serialization (like in the real workflow)
			log.Info("Before serialization",
				"uuid", authReq.Role.UUID.String(),
				"composite", authReq.Role.Composite,
				"providers", authReq.Role.Providers,
			)

			return authorizeRoleRequestSideEffect{Request: authReq}
		})

		var se authorizeRoleRequestSideEffect
		if err := encodedReq.Get(&se); err != nil {
			return nil, err
		}

		// Log after deserialization (like in the real workflow)
		if se.Request != nil && se.Request.Role != nil {
			log.Info("After deserialization",
				"uuid", se.Request.Role.UUID.String(),
				"composite", se.Request.Role.Composite,
				"providers", se.Request.Role.Providers,
			)
		}

		return se.Request, nil
	}

	env.ExecuteWorkflow(testWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result *models.AuthorizeRoleRequest
	require.NoError(t, env.GetWorkflowResult(&result))
	require.NotNil(t, result)
	require.NotNil(t, result.Role)

	// Verify all fields survived
	t.Logf("Expected UUID: %s", expectedUUID.String())
	t.Logf("Actual UUID:   %s", result.Role.UUID.String())
	assert.Equal(t, expectedUUID, result.Role.UUID, "UUID should be preserved")

	t.Logf("Expected Composite: %v", expectedComposite)
	t.Logf("Actual Composite:   %v", result.Role.Composite)
	assert.Equal(t, expectedComposite, result.Role.Composite, "Composite should be preserved")

	t.Logf("Expected Providers: %v", expectedProviders)
	t.Logf("Actual Providers:   %v", result.Role.Providers)
	assert.Equal(t, expectedProviders, result.Role.Providers, "Providers should be preserved")
}
