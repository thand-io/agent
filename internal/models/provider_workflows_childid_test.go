package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestCreateChildWorkflowID_Uniqueness verifies that child workflow IDs are unique
// across different identities and tenants for the same role
func TestCreateChildWorkflowID_Uniqueness(t *testing.T) {
	parentWfID := "parent-workflow-123"
	duration := 1 * time.Hour

	// Create the same role for different identities
	role := &Role{
		Identifier: "admin-role",
		Name:       "AdminRole",
	}

	// Request 1: User alice@example.com, no tenant
	req1 := &WorkflowRoleRequest{
		WorkflowID: parentWfID,
		Identity:   "alice@example.com",
		Role:       role,
		Duration:   &duration,
	}

	// Request 2: User bob@example.com, no tenant
	req2 := &WorkflowRoleRequest{
		WorkflowID: parentWfID,
		Identity:   "bob@example.com",
		Role:       role,
		Duration:   &duration,
	}

	// Request 3: User alice@example.com, tenant-A
	req3 := &WorkflowRoleRequest{
		WorkflowID: parentWfID,
		Identity:   "alice@example.com",
		Tenant:     "tenant-A",
		Role:       role,
		Duration:   &duration,
	}

	// Request 4: User alice@example.com, tenant-B
	req4 := &WorkflowRoleRequest{
		WorkflowID: parentWfID,
		Identity:   "alice@example.com",
		Tenant:     "tenant-B",
		Role:       role,
		Duration:   &duration,
	}

	provider := "aws"

	// Generate child workflow IDs
	wfID1 := CreateChildWorkflowID(parentWfID, "authorizeRole", provider, req1)
	wfID2 := CreateChildWorkflowID(parentWfID, "authorizeRole", provider, req2)
	wfID3 := CreateChildWorkflowID(parentWfID, "authorizeRole", provider, req3)
	wfID4 := CreateChildWorkflowID(parentWfID, "authorizeRole", provider, req4)

	// Verify all IDs are different
	assert.NotEqual(t, wfID1, wfID2, "Different identities should produce different workflow IDs")
	assert.NotEqual(t, wfID1, wfID3, "Different tenants should produce different workflow IDs")
	assert.NotEqual(t, wfID1, wfID4, "Different tenants should produce different workflow IDs")
	assert.NotEqual(t, wfID3, wfID4, "Different tenants should produce different workflow IDs")

	// Verify all IDs start with parent workflow ID
	assert.Contains(t, wfID1, parentWfID)
	assert.Contains(t, wfID2, parentWfID)
	assert.Contains(t, wfID3, parentWfID)
	assert.Contains(t, wfID4, parentWfID)

	// Verify all IDs contain the operation name
	assert.Contains(t, wfID1, "authorizeRole")
	assert.Contains(t, wfID2, "authorizeRole")
	assert.Contains(t, wfID3, "authorizeRole")
	assert.Contains(t, wfID4, "authorizeRole")

	t.Logf("Child workflow IDs generated:")
	t.Logf("  alice (no tenant): %s", wfID1)
	t.Logf("  bob (no tenant):   %s", wfID2)
	t.Logf("  alice (tenant-A):  %s", wfID3)
	t.Logf("  alice (tenant-B):  %s", wfID4)
}

// TestCreateChildWorkflowID_Deterministic verifies that the same input produces the same ID
func TestCreateChildWorkflowID_Deterministic(t *testing.T) {
	parentWfID := "parent-workflow-456"
	duration := 2 * time.Hour

	role := &Role{
		Identifier: "viewer-role",
		Name:       "ViewerRole",
	}

	req := &WorkflowRoleRequest{
		WorkflowID: parentWfID,
		Identity:   "user@example.com",
		Tenant:     "tenant-X",
		Role:       role,
		Duration:   &duration,
	}

	provider := "gcp"

	// Generate the same ID multiple times
	wfID1 := CreateChildWorkflowID(parentWfID, "authorizeRole", provider, req)
	wfID2 := CreateChildWorkflowID(parentWfID, "authorizeRole", provider, req)
	wfID3 := CreateChildWorkflowID(parentWfID, "authorizeRole", provider, req)

	// Verify all IDs are identical
	assert.Equal(t, wfID1, wfID2, "Same input should produce identical workflow IDs")
	assert.Equal(t, wfID2, wfID3, "Same input should produce identical workflow IDs")

	t.Logf("Deterministic workflow ID: %s", wfID1)
}

// TestCreateChildWorkflowID_DifferentProviders verifies that different providers produce different IDs
func TestCreateChildWorkflowID_DifferentProviders(t *testing.T) {
	parentWfID := "parent-workflow-789"
	duration := 3 * time.Hour

	role := &Role{
		Identifier: "editor-role",
		Name:       "EditorRole",
	}

	req := &WorkflowRoleRequest{
		WorkflowID: parentWfID,
		Identity:   "user@example.com",
		Role:       role,
		Duration:   &duration,
	}

	// Generate IDs for different providers
	wfIDAWS := CreateChildWorkflowID(parentWfID, "authorizeRole", "aws", req)
	wfIDGCP := CreateChildWorkflowID(parentWfID, "authorizeRole", "gcp", req)
	wfIDAzure := CreateChildWorkflowID(parentWfID, "authorizeRole", "azure", req)

	// Verify all IDs are different
	assert.NotEqual(t, wfIDAWS, wfIDGCP, "Different providers should produce different workflow IDs")
	assert.NotEqual(t, wfIDGCP, wfIDAzure, "Different providers should produce different workflow IDs")
	assert.NotEqual(t, wfIDAWS, wfIDAzure, "Different providers should produce different workflow IDs")

	t.Logf("Provider-specific workflow IDs:")
	t.Logf("  AWS:   %s", wfIDAWS)
	t.Logf("  GCP:   %s", wfIDGCP)
	t.Logf("  Azure: %s", wfIDAzure)
}

// TestCreateChildWorkflowID_DifferentOperations verifies that different operations produce different IDs
func TestCreateChildWorkflowID_DifferentOperations(t *testing.T) {
	parentWfID := "parent-workflow-xyz"
	duration := 4 * time.Hour

	role := &Role{
		Identifier: "operator-role",
		Name:       "OperatorRole",
	}

	req := &WorkflowRoleRequest{
		WorkflowID: parentWfID,
		Identity:   "operator@example.com",
		Role:       role,
		Duration:   &duration,
	}

	provider := "aws"

	// Generate IDs for different operations
	authID := CreateChildWorkflowID(parentWfID, "authorizeRole", provider, req)
	revokeID := CreateChildWorkflowID(parentWfID, "revokeRole", provider, req)

	// Verify IDs are different (different operations)
	assert.NotEqual(t, authID, revokeID, "Different operations should produce different workflow IDs")

	// Verify both contain the operation names
	assert.Contains(t, authID, "authorizeRole")
	assert.Contains(t, revokeID, "revokeRole")

	t.Logf("Operation-specific workflow IDs:")
	t.Logf("  Authorize: %s", authID)
	t.Logf("  Revoke:    %s", revokeID)
}
