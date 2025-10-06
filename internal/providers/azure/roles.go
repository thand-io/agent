package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/third_party"
)

// getRoleDefinition retrieves a custom role definition by name
func (p *azureProvider) getRoleDefinition(ctx context.Context, roleName string) (*armauthorization.RoleDefinition, error) {
	scope := p.getScope()

	pager := p.roleDefClient.NewListPager(scope, &armauthorization.RoleDefinitionsClientListOptions{
		Filter: &[]string{fmt.Sprintf("roleName eq '%s'", roleName)}[0],
	})

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list role definitions: %w", err)
		}

		for _, roleDef := range page.Value {
			if roleDef.Properties != nil && roleDef.Properties.RoleName != nil &&
				strings.EqualFold(*roleDef.Properties.RoleName, roleName) {
				return roleDef, nil
			}
		}
	}

	return nil, fmt.Errorf("role definition '%s' not found", roleName)
}

// createRoleDefinition creates a custom role definition
func (p *azureProvider) createRoleDefinition(ctx context.Context, roleName, description string, permissions []string) (*armauthorization.RoleDefinition, error) {
	scope := p.getScope()
	roleDefinitionID := uuid.New().String()

	// Convert permissions to Azure actions
	var actions []*string
	for _, perm := range permissions {
		actions = append(actions, &perm)
	}

	roleDefinition := armauthorization.RoleDefinition{
		Properties: &armauthorization.RoleDefinitionProperties{
			RoleName:         &roleName,
			Description:      &description,
			AssignableScopes: []*string{&scope},
			Permissions: []*armauthorization.Permission{
				{
					Actions:    actions,
					NotActions: []*string{},
				},
			},
		},
	}

	result, err := p.roleDefClient.CreateOrUpdate(ctx, scope, roleDefinitionID, roleDefinition, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create role definition: %w", err)
	}

	return &result.RoleDefinition, nil
}

// createRoleAssignment assigns a role to a user
func (p *azureProvider) createRoleAssignment(ctx context.Context, user *models.User, roleDefinitionID string) error {
	scope := p.getScope()

	// Get the principal ID for the user
	principalID, err := p.getUserPrincipalID(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to get user principal ID: %w", err)
	}

	roleAssignmentID := uuid.New().String()
	roleAssignment := armauthorization.RoleAssignmentCreateParameters{
		Properties: &armauthorization.RoleAssignmentProperties{
			RoleDefinitionID: &roleDefinitionID,
			PrincipalID:      &principalID,
		},
	}

	_, err = p.authClient.Create(ctx, scope, roleAssignmentID, roleAssignment, nil)
	if err != nil {
		return fmt.Errorf("failed to create role assignment: %w", err)
	}

	return nil
}

// deleteRoleAssignment removes a role assignment for a user
func (p *azureProvider) deleteRoleAssignment(ctx context.Context, user *models.User, roleDefinitionID string) error {
	scope := p.getScope()

	// Get the principal ID for the user
	principalID, err := p.getUserPrincipalID(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to get user principal ID: %w", err)
	}

	// Find existing role assignments for this user and role
	pager := p.authClient.NewListForScopePager(scope, &armauthorization.RoleAssignmentsClientListForScopeOptions{
		Filter: &[]string{fmt.Sprintf("principalId eq '%s'", principalID)}[0],
	})

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list role assignments: %w", err)
		}

		for _, assignment := range page.Value {
			if assignment.Properties != nil &&
				assignment.Properties.RoleDefinitionID != nil &&
				*assignment.Properties.RoleDefinitionID == roleDefinitionID {

				_, err = p.authClient.Delete(ctx, scope, *assignment.Name, nil)
				if err != nil {
					return fmt.Errorf("failed to delete role assignment: %w", err)
				}
			}
		}
	}

	return nil
}

// getUserPrincipalID gets the Azure AD object ID for a user
func (p *azureProvider) getUserPrincipalID(ctx context.Context, user *models.User) (string, error) {
	if len(user.Email) == 0 {
		return "", fmt.Errorf("user email is required for Azure role assignments")
	}

	// For this implementation, we'll use the user's email to lookup the Azure AD object ID
	// In a production implementation, you would query Microsoft Graph API to get the object ID
	// based on the user's email or UPN

	// For now, we'll use the user's ID field if it contains an Azure object ID (GUID format)
	// or derive it from email in a simplified way
	if len(user.ID) > 0 && len(user.ID) >= 32 {
		// Assume ID is already an Azure object ID if it looks like a GUID
		return user.ID, nil
	}

	// TODO: In production, implement Microsoft Graph API lookup:
	// 1. Use Microsoft Graph SDK to query users by email
	// 2. Get the user's object ID from the response
	// Example: GET https://graph.microsoft.com/v1.0/users/{email}

	// For development/testing, return error with instruction
	return "", fmt.Errorf("azure object ID not found. User ID should contain the Azure AD object ID. "+
		"In production, implement Microsoft Graph API lookup to resolve email '%s' to object ID", user.Email)
}

// getScope returns the scope for role operations
func (p *azureProvider) getScope() string {
	if len(p.resourceGroupName) > 0 {
		return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", p.subscriptionID, p.resourceGroupName)
	}
	return fmt.Sprintf("/subscriptions/%s", p.subscriptionID)
}

// GetRole retrieves a specific role by name
func (p *azureProvider) GetRole(ctx context.Context, role string) (*models.ProviderRole, error) {
	// First check in loaded built-in roles
	for _, r := range p.roles {
		if strings.EqualFold(r.Name, role) {
			return &r, nil
		}
	}

	// If not found in built-in roles, try to get custom role definition
	roleDefinition, err := p.getRoleDefinition(ctx, role)
	if err != nil {
		return nil, fmt.Errorf("role '%s' not found", role)
	}

	roleInfo := &models.ProviderRole{
		Name:        *roleDefinition.Properties.RoleName,
		Description: *roleDefinition.Properties.Description,
	}

	return roleInfo, nil
}

// ListRoles returns all available roles
func (p *azureProvider) ListRoles(ctx context.Context, filters ...string) ([]models.ProviderRole, error) {

	return common.BleveListSearch(ctx, p.rolesIndex, func(a *search.DocumentMatch, b models.ProviderRole) bool {
		return strings.Compare(a.ID, b.Name) == 0
	}, p.roles, filters...)

}

// LoadRoles loads Azure built-in roles from the embedded roles data
func (p *azureProvider) LoadRoles() error {
	var rolesData azureBuiltInRoles

	if err := json.Unmarshal(third_party.GetAzureRoles(), &rolesData); err != nil {
		return fmt.Errorf("failed to unmarshal Azure roles: %w", err)
	}

	var roles []models.ProviderRole

	// Create in-memory Bleve index for roles
	mapping := bleve.NewIndexMapping()
	rolesIndex, err := bleve.NewMemOnly(mapping)
	if err != nil {
		return fmt.Errorf("failed to create roles search index: %w", err)
	}

	for _, role := range rolesData.Roles {
		roles = append(roles, models.ProviderRole{
			Name:        role.Name,
			Description: role.Description,
		})
	}

	p.roles = roles
	p.rolesIndex = rolesIndex

	logrus.WithFields(logrus.Fields{
		"roles": len(roles),
	}).Debug("Loaded Azure built-in roles")

	return nil
}

/*
// Start with built-in roles
	allRoles := make([]models.ProviderRole, len(p.roles))
	copy(allRoles, p.roles)

	// Add custom roles from Azure (this could be expensive, so we might want to cache)
	scope := p.getScope()
	pager := p.roleDefClient.NewListPager(scope, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			logrus.WithError(err).Warn("Failed to list custom role definitions")
			break
		}

		for _, roleDef := range page.Value {
			if roleDef.Properties != nil && roleDef.Properties.RoleName != nil {
				roleInfo := models.ProviderRole{
					Name: *roleDef.Properties.RoleName,
				}
				if roleDef.Properties.Description != nil {
					roleInfo.Description = *roleDef.Properties.Description
				}
				allRoles = append(allRoles, roleInfo)
			}
		}
	}
*/
