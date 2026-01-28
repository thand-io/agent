package azure

import (
	"context"
	"fmt"

	"github.com/thand-io/agent/internal/models"
)

// Authorize grants access for a user to a role
func (p *azureProvider) AuthorizeRole(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
) (*models.AuthorizeRoleResponse, error) {

	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to authorize azure role")
	}

	user := req.GetUser()
	role := req.GetRole()

	// Check if the role exists (as custom role definition)
	existingRole, err := p.getRoleDefinition(ctx, role.Name)
	if err != nil {
		// If role doesn't exist, create it as a custom role
		existingRole, err = p.createRoleDefinition(ctx, role.Name, role.Description, role.Permissions)
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
	req *models.RevokeRoleRequest,
) (*models.RevokeRoleResponse, error) {

	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to revoke azure role")
	}

	user := req.GetUser()
	role := req.GetRole()

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

// permissionsToAzureActions converts CSP-agnostic Permissions to Azure role actions and notActions
// Allow statements become Actions, Deny statements become NotActions
// Returns: (actions, notActions, targets for assignableScopes)
func permissionsToAzureActions(permissions models.Permissions) (actions, notActions []string, targets []string) {
	// Process Allow statements -> Actions
	for _, stmt := range permissions.Allow {
		actions = append(actions, stmt.Operations...)
		// Collect unique targets for assignableScopes
		for _, target := range stmt.Targets {
			found := false
			for _, existing := range targets {
				if existing == target {
					found = true
					break
				}
			}
			if !found {
				targets = append(targets, target)
			}
		}
	}

	// Process Deny statements -> NotActions
	for _, stmt := range permissions.Deny {
		notActions = append(notActions, stmt.Operations...)
		// Collect unique targets for assignableScopes
		for _, target := range stmt.Targets {
			found := false
			for _, existing := range targets {
				if existing == target {
					found = true
					break
				}
			}
			if !found {
				targets = append(targets, target)
			}
		}
	}

	return actions, notActions, targets
}
