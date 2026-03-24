package azure

import (
	"context"
	"fmt"
	"time"

	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/temporal"
)

// azureProviderActivities exposes granular Azure provider operations as individual
// Temporal activities, one per idempotent step of AuthorizeRole / RevokeRole.
type azureProviderActivities struct {
	provider *azureProvider
}

// ─────────────────────────────────────────────────────────────────────────────
// Request / response types
// ─────────────────────────────────────────────────────────────────────────────

type GetOrCreateRoleDefinitionRequest struct {
	RoleIdentifier string                 `json:"role_identifier"` // CSP resource name from CompositeRole.GetName()
	Description    string                 `json:"description"`
	Permissions    models.RolePermissions `json:"permissions"`
}

type GetOrCreateRoleDefinitionResponse struct {
	RoleDefinitionID string `json:"role_definition_id"`
}

type CreateRoleAssignmentRequest struct {
	User             *models.User `json:"user"`
	RoleDefinitionID string       `json:"role_definition_id"`
}

type CreateRoleAssignmentResponse struct {
	PrincipalID string `json:"principal_id"`
}

type GetRoleDefinitionRequest struct {
	RoleIdentifier string `json:"role_identifier"` // CSP resource name from CompositeRole.GetName()
}

type GetRoleDefinitionResponse struct {
	RoleDefinitionID string `json:"role_definition_id"`
}

type DeleteRoleAssignmentRequest struct {
	User             *models.User `json:"user"`
	RoleDefinitionID string       `json:"role_definition_id"`
	PrincipalID      string       `json:"principal_id"`
}

// DeleteRoleDefinitionRequest carries the role definition ID to delete.
// Used for composite roles only — non-composite definitions are retained.
type DeleteRoleDefinitionRequest struct {
	RoleDefinitionID string `json:"role_definition_id"`
}

type AssignBuiltInRolesRequest struct {
	User      *models.User `json:"user"`
	RoleNames []string     `json:"role_names"`
}

type AssignBuiltInRolesResponse struct {
	RoleDefinitionIDs []string `json:"role_definition_ids"`
}

type RevokeBuiltInRolesRequest struct {
	User              *models.User `json:"user"`
	PrincipalID       string       `json:"principal_id"`
	RoleDefinitionIDs []string     `json:"role_definition_ids"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Activity implementations
// ─────────────────────────────────────────────────────────────────────────────

// GetOrCreateRoleDefinition retrieves an existing Azure role definition by name, or
// creates a custom one if it does not yet exist.
func (a *azureProviderActivities) GetOrCreateRoleDefinition(
	ctx context.Context,
	req *GetOrCreateRoleDefinitionRequest,
) (*GetOrCreateRoleDefinitionResponse, error) {
	role, err := a.provider.getRoleDefinition(ctx, req.RoleIdentifier)
	if err != nil {
		role, err = a.provider.createRoleDefinition(ctx, req.RoleIdentifier, req.Description, req.Permissions)
		if err != nil {
			return nil, temporal.NewApplicationErrorWithOptions(
				fmt.Sprintf("failed to create Azure role definition '%s': %v", req.RoleIdentifier, err),
				"AzureRoleDefinitionError",
				temporal.ApplicationErrorOptions{
					NextRetryDelay: 3 * time.Second,
					Cause:          err,
				},
			)
		}
	}
	return &GetOrCreateRoleDefinitionResponse{
		RoleDefinitionID: *role.ID,
	}, nil
}

// CreateRoleAssignment assigns the role definition to the given user and returns
// the principal ID that must be stored for later revocation.
func (a *azureProviderActivities) CreateRoleAssignment(
	ctx context.Context,
	req *CreateRoleAssignmentRequest,
) (*CreateRoleAssignmentResponse, error) {
	principalID, err := a.provider.createRoleAssignment(ctx, req.User, req.RoleDefinitionID)
	if err != nil {
		return nil, temporal.NewApplicationErrorWithOptions(
			fmt.Sprintf("failed to create Azure role assignment for user '%s': %v", req.User.Email, err),
			"AzureRoleAssignmentError",
			temporal.ApplicationErrorOptions{
				NextRetryDelay: 3 * time.Second,
				Cause:          err,
			},
		)
	}
	return &CreateRoleAssignmentResponse{
		PrincipalID: principalID,
	}, nil
}

// GetRoleDefinition looks up an Azure role definition by name.
func (a *azureProviderActivities) GetRoleDefinition(
	ctx context.Context,
	req *GetRoleDefinitionRequest,
) (*GetRoleDefinitionResponse, error) {
	role, err := a.provider.getRoleDefinition(ctx, req.RoleIdentifier)
	if err != nil {
		// A missing role definition during revocation is a permanent failure —
		// there is nothing to revoke if the role no longer exists.
		return nil, temporal.NewNonRetryableApplicationError(
			fmt.Sprintf("Azure role definition '%s' not found during revocation", req.RoleIdentifier),
			"AzureRoleNotFoundError",
			err,
		)
	}
	return &GetRoleDefinitionResponse{
		RoleDefinitionID: *role.ID,
	}, nil
}

// DeleteRoleAssignment removes the role assignment for the user.
func (a *azureProviderActivities) DeleteRoleAssignment(
	ctx context.Context,
	req *DeleteRoleAssignmentRequest,
) error {
	if err := a.provider.deleteRoleAssignment(ctx, req.User, req.RoleDefinitionID, req.PrincipalID); err != nil {
		return temporal.NewApplicationErrorWithOptions(
			fmt.Sprintf("failed to delete Azure role assignment for user '%s': %v", req.User.Email, err),
			"AzureRoleAssignmentDeleteError",
			temporal.ApplicationErrorOptions{
				NextRetryDelay: 3 * time.Second,
				Cause:          err,
			},
		)
	}
	return nil
}

// DeleteRoleDefinition removes a custom Azure role definition.
// Used for composite roles only — non-composite definitions are retained.
func (a *azureProviderActivities) DeleteRoleDefinition(
	ctx context.Context,
	req *DeleteRoleDefinitionRequest,
) error {
	if err := a.provider.deleteRoleDefinition(ctx, req.RoleDefinitionID); err != nil {
		return temporal.NewApplicationErrorWithOptions(
			fmt.Sprintf("failed to delete Azure role definition '%s': %v", req.RoleDefinitionID, err),
			"AzureRoleDefinitionDeleteError",
			temporal.ApplicationErrorOptions{
				NextRetryDelay: 3 * time.Second,
				Cause:          err,
			},
		)
	}
	return nil
}

// AssignBuiltInRoles looks up each inherited Azure role by name and creates a
// direct role assignment for the user against each one. Returns the list of
// role definition IDs that were assigned, which must be stored in
// AuthorizeRoleResponse.Metadata for later revocation.
func (a *azureProviderActivities) AssignBuiltInRoles(
	ctx context.Context,
	req *AssignBuiltInRolesRequest,
) (*AssignBuiltInRolesResponse, error) {
	roleDefinitionIDs := make([]string, 0, len(req.RoleNames))
	for _, roleName := range req.RoleNames {
		roleDef, err := a.provider.getRoleDefinition(ctx, roleName)
		if err != nil {
			return nil, temporal.NewApplicationErrorWithOptions(
				fmt.Sprintf("inherited Azure role '%s' not found: %v", roleName, err),
				"AzureBuiltInRoleNotFoundError",
				temporal.ApplicationErrorOptions{
					NextRetryDelay: 3 * time.Second,
					Cause:          err,
				},
			)
		}
		if _, err = a.provider.createRoleAssignment(ctx, req.User, *roleDef.ID); err != nil {
			return nil, temporal.NewApplicationErrorWithOptions(
				fmt.Sprintf("failed to assign inherited role '%s' to user '%s': %v", roleName, req.User.Email, err),
				"AzureBuiltInRoleAssignmentError",
				temporal.ApplicationErrorOptions{
					NextRetryDelay: 3 * time.Second,
					Cause:          err,
				},
			)
		}
		roleDefinitionIDs = append(roleDefinitionIDs, *roleDef.ID)
	}
	return &AssignBuiltInRolesResponse{RoleDefinitionIDs: roleDefinitionIDs}, nil
}

// RevokeBuiltInRoles removes the direct role assignments for each inherited
// built-in role that was assigned during authorization.
func (a *azureProviderActivities) RevokeBuiltInRoles(
	ctx context.Context,
	req *RevokeBuiltInRolesRequest,
) error {
	for _, roleDefID := range req.RoleDefinitionIDs {
		if err := a.provider.deleteRoleAssignment(ctx, req.User, roleDefID, req.PrincipalID); err != nil {
			return temporal.NewApplicationErrorWithOptions(
				fmt.Sprintf("failed to delete built-in role assignment '%s' for user '%s': %v", roleDefID, req.User.Email, err),
				"AzureBuiltInRoleRevocationError",
				temporal.ApplicationErrorOptions{
					NextRetryDelay: 3 * time.Second,
					Cause:          err,
				},
			)
		}
	}
	return nil
}
