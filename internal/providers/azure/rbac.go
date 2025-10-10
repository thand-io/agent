package azure

import (
	"context"
	"fmt"

	"github.com/thand-io/agent/internal/models"
)

// Authorize grants access for a user to a role
func (p *azureProvider) AuthorizeRole(
	ctx context.Context, user *models.User, role *models.Role) (map[string]any, error) {
	// Check if the role exists (as custom role definition)
	existingRole, err := p.getRoleDefinition(ctx, role.Name)
	if err != nil {
		// If role doesn't exist, create it as a custom role
		existingRole, err = p.createRoleDefinition(ctx, role.Name, role.Description, role.Permissions.Allow)
		if err != nil {
			return nil, fmt.Errorf("failed to create role definition: %w", err)
		}
	}

	// Create role assignment for the user
	err = p.createRoleAssignment(ctx, user, *existingRole.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to create role assignment: %w", err)
	}

	return nil, nil
}

// Revoke removes access for a user from a role
func (p *azureProvider) RevokeRole(
	ctx context.Context,
	user *models.User,
	role *models.Role,
	metadata map[string]any,
) (map[string]any, error) {
	// Get the role definition
	roleDefinition, err := p.getRoleDefinition(ctx, role.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get role definition: %w", err)
	}

	// Find and delete role assignments for this user and role
	err = p.deleteRoleAssignment(ctx, user, *roleDefinition.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete role assignment: %w", err)
	}

	return nil, nil
}
