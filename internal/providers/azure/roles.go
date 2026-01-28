package azure

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
	"github.com/google/uuid"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/data"
	"github.com/thand-io/agent/internal/models"
)

// Never synchronize roles from Azure as they are
// statically defined by Azure and cannot be modified
func (p *azureProvider) CanSynchronizeRoles() bool {
	return false
}

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
func (p *azureProvider) createRoleDefinition(ctx context.Context, roleName, description string, permissions models.Permissions) (*armauthorization.RoleDefinition, error) {
	scope := p.getScope()
	roleDefinitionID := uuid.New().String()

	// Convert permissions to Azure actions and notActions
	actions, notActions, targets := permissionsToAzureActions(permissions)

	// Convert to Azure pointer format
	var actionPtrs []*string
	for _, action := range actions {
		actionPtrs = append(actionPtrs, &action)
	}

	var notActionPtrs []*string
	for _, notAction := range notActions {
		notActionPtrs = append(notActionPtrs, &notAction)
	}

	// Use targets as assignable scopes if provided, otherwise use default scope
	assignableScopes := []*string{&scope}
	if len(targets) > 0 {
		assignableScopes = make([]*string, len(targets))
		for i, target := range targets {
			assignableScopes[i] = &target
		}
	}

	roleDefinition := armauthorization.RoleDefinition{
		Properties: &armauthorization.RoleDefinitionProperties{
			RoleName:         &roleName,
			Description:      &description,
			AssignableScopes: assignableScopes,
			Permissions: []*armauthorization.Permission{
				{
					Actions:    actionPtrs,
					NotActions: notActionPtrs,
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

	// If the user's ID field already contains an Azure object ID (GUID format), use it
	if len(user.ID) > 0 && len(user.ID) >= 32 {
		// Validate it looks like a GUID
		if _, err := uuid.Parse(user.ID); err == nil {
			logrus.WithField("user_id", user.ID).Debug("Using existing Azure object ID from user.ID field")
			return user.ID, nil
		}
	}

	// Use Microsoft Graph API to lookup the user by email and get their object ID
	logrus.WithField("email", user.Email).Debug("Looking up Azure AD object ID via Microsoft Graph API")

	// Create a Microsoft Graph client using the existing Azure credentials
	client, err := msgraphsdk.NewGraphServiceClientWithCredentials(p.cred.Token, []string{"https://graph.microsoft.com/.default"})
	if err != nil {
		return "", fmt.Errorf("failed to create Microsoft Graph client: %w", err)
	}

	// Query the user by their email address (UPN)
	// GET https://graph.microsoft.com/v1.0/users/{email}
	graphUser, err := client.Users().ByUserId(user.Email).Get(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("failed to lookup user '%s' in Azure AD via Microsoft Graph API: %w", user.Email, err)
	}

	// Extract the object ID from the response
	if graphUser == nil || graphUser.GetId() == nil {
		return "", fmt.Errorf("user '%s' found in Azure AD but object ID is missing", user.Email)
	}

	objectID := *graphUser.GetId()
	logrus.WithFields(logrus.Fields{
		"email":     user.Email,
		"object_id": objectID,
	}).Debug("Successfully retrieved Azure AD object ID")

	return objectID, nil
}

// getScope returns the scope for role operations
func (p *azureProvider) getScope() string {
	if len(p.resourceGroupName) > 0 {
		return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", p.subscriptionID, p.resourceGroupName)
	}
	return fmt.Sprintf("/subscriptions/%s", p.subscriptionID)
}

func loadRoles() ([]models.ProviderRole, error) {

	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Parsed Azure roles in %s", elapsed)
	}()

	// Get pre-parsed Azure roles from data package
	azureRoles, err := data.GetParsedAzureRoles()
	if err != nil {
		return nil, fmt.Errorf("failed to get parsed Azure roles: %w", err)
	}

	var roles []models.ProviderRole

	for _, role := range azureRoles {
		r := models.ProviderRole{
			Name:        role.Name,
			Description: role.Description,
		}
		roles = append(roles, r)
	}

	return roles, nil
}
