package azure

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

const PrincipalIdentifierMetadataKey = "principal_id"
const RoleDefinitionIdentifierMetadataKey = "role_definition_id"

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
	principalID, err := p.createRoleAssignment(ctx, user, *existingRole.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to create role assignment: %w", err)
	}

	// Return the response with the principal ID stored in metadata
	// This ensures we use the same principal ID for revocation
	return &models.AuthorizeRoleResponse{
		UserId: user.Email,
		Roles:  []string{role.Name},
		Metadata: map[string]any{
			PrincipalIdentifierMetadataKey: principalID,
			RoleDefinitionIdentifierMetadataKey: *existingRole.ID,
		},
	}, nil
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

	if req.AuthorizeRoleResponse == nil || req.AuthorizeRoleResponse.Metadata == nil {
		return nil, fmt.Errorf("missing authorization response metadata for revocation")
	}

	if principalID, ok := req.AuthorizeRoleResponse.Metadata[PrincipalIdentifierMetadataKey].(string); ok {
		
		logrus.WithFields(logrus.Fields{
			"email":        user.Email,
			"principal_id": principalID,
		}).Debug("Retrieved stored principal ID from authorization response")

		// Find and delete role assignments for this user and role
		err = p.deleteRoleAssignment(ctx, user, *roleDefinition.ID, principalID)

		if err != nil {
			return nil, fmt.Errorf("failed to delete role assignment: %w", err)
		}

	} else {

		return nil, fmt.Errorf("invalid principal ID in authorization response metadata")

	}

	return nil, nil
}

// permissionsToAzureActions converts CSP-agnostic Permissions to Azure role actions and notActions
// Allow statements become Actions, Deny statements become NotActions
// Returns: (actions, notActions, targets for assignableScopes)
func permissionsToAzureActions(permissions models.RolePermissions) (actions, notActions []string, targets []string) {
	// Use a map for efficient target deduplication
	targetSet := make(map[string]bool)

	// Process Allow statements -> Actions
	for _, stmt := range permissions.Allow {
		actions = append(actions, stmt.Operations...)
		for _, target := range stmt.Targets {
			targetSet[target] = true
		}
	}

	// Process Deny statements -> NotActions
	for _, stmt := range permissions.Deny {
		notActions = append(notActions, stmt.Operations...)
		for _, target := range stmt.Targets {
			targetSet[target] = true
		}
	}

	// Convert target set to slice
	for target := range targetSet {
		targets = append(targets, target)
	}

	return actions, notActions, targets
}
