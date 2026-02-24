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
	RoleName    string                 `json:"role_name"`
	Description string                 `json:"description"`
	Permissions models.RolePermissions `json:"permissions"`
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
	RoleName string `json:"role_name"`
}

type GetRoleDefinitionResponse struct {
	RoleDefinitionID string `json:"role_definition_id"`
}

type DeleteRoleAssignmentRequest struct {
	User             *models.User `json:"user"`
	RoleDefinitionID string       `json:"role_definition_id"`
	PrincipalID      string       `json:"principal_id"`
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
	role, err := a.provider.getRoleDefinition(ctx, req.RoleName)
	if err != nil {
		role, err = a.provider.createRoleDefinition(ctx, req.RoleName, req.Description, req.Permissions)
		if err != nil {
			return nil, temporal.NewApplicationErrorWithOptions(
				fmt.Sprintf("failed to create Azure role definition '%s': %v", req.RoleName, err),
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
	role, err := a.provider.getRoleDefinition(ctx, req.RoleName)
	if err != nil {
		// A missing role definition during revocation is a permanent failure —
		// there is nothing to revoke if the role no longer exists.
		return nil, temporal.NewNonRetryableApplicationError(
			fmt.Sprintf("Azure role definition '%s' not found during revocation", req.RoleName),
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
