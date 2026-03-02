package azure

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	sdkWorkflowsRunner "github.com/thand-io/agent/sdk/workflows/runner"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const PrincipalIdentifierMetadataKey = "principal_id"
const RoleDefinitionIdentifierMetadataKey = "role_definition_id"

// Authorize grants access for a user to a role.
//
// Role lifecycle:
//   - Composite roles: role definition gets a unique name (hashed suffix).
//     On revoke both the assignment and the definition are deleted.
//   - Non-composite roles: role definition uses the base name so it can be
//     shared across users. On revoke only the assignment is removed; the
//     definition is retained.
func (p *azureProvider) AuthorizeRole(
	ctx models.ProviderContext,
	req *models.AuthorizeRoleRequest,
) (*models.AuthorizeRoleResponse, error) {
	if workflowCtx, ok := ctx.(workflow.Context); ok {
		return p.authorizeRoleTemporal(workflowCtx, req)
	}

	localCtx, ok := ctx.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid context type")
	}

	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to authorize azure role")
	}

	user := req.GetUser()
	role := req.GetRole()
	isComposite := role.IsComposite()

	// Composite roles get a unique definition name; non-composite roles share a base name.
	roleName := role.GetName()

	logrus.WithFields(logrus.Fields{
		"role":         roleName,
		"is_composite": isComposite,
	}).Info("Azure authorizeRole: determining role lifecycle")

	// Check if the role definition already exists
	existingRole, err := p.getRoleDefinition(localCtx, roleName)
	if err != nil {
		// If role doesn't exist, create it as a custom role
		existingRole, err = p.createRoleDefinition(localCtx, roleName, role.GetDescription(), role.Permissions)
		if err != nil {
			return nil, fmt.Errorf("failed to create role definition: %w", err)
		}
	}

	// Create role assignment for the user
	principalID, err := p.createRoleAssignment(localCtx, user, *existingRole.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to create role assignment: %w", err)
	}

	return &models.AuthorizeRoleResponse{
		UserId: user.Email,
		Roles:  []string{roleName},
		Metadata: map[string]any{
			PrincipalIdentifierMetadataKey:      principalID,
			RoleDefinitionIdentifierMetadataKey: *existingRole.ID,
		},
	}, nil
}

// Revoke removes access for a user from a role.
//
// Role lifecycle on revoke:
//   - Composite: delete assignment + delete role definition.
//   - Non-composite: delete assignment only; the definition is retained.
func (p *azureProvider) RevokeRole(
	ctx models.ProviderContext,
	req *models.RevokeRoleRequest,
) (*models.RevokeRoleResponse, error) {
	if workflowCtx, ok := ctx.(workflow.Context); ok {
		return p.revokeRoleTemporal(workflowCtx, req)
	}

	localCtx, ok := ctx.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid context type")
	}

	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to revoke azure role")
	}

	user := req.GetUser()
	role := req.GetRole()

	isComposite := role.IsComposite()

	// Use composite-aware name for lookup.
	roleName := role.GetName()

	logrus.WithFields(logrus.Fields{
		"role":         roleName,
		"is_composite": isComposite,
	}).Info("Azure revokeRole: determining cleanup strategy")

	// Get the role definition
	roleDefinition, err := p.getRoleDefinition(localCtx, roleName)
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
		err = p.deleteRoleAssignment(localCtx, user, *roleDefinition.ID, principalID)

		if err != nil {
			return nil, fmt.Errorf("failed to delete role assignment: %w", err)
		}

	} else {

		return nil, fmt.Errorf("invalid principal ID in authorization response metadata")

	}

	// Composite: delete the role definition after removing the assignment.
	// Non-composite: retain the definition for future authorizations.
	if isComposite {
		err = p.deleteRoleDefinition(localCtx, *roleDefinition.ID)
		if err != nil {
			logrus.WithError(err).WithField("role_definition_id", *roleDefinition.ID).
				Warn("Failed to delete composite role definition; assignment was already removed")
		} else {
			logrus.WithField("role_definition_id", *roleDefinition.ID).
				Info("Successfully deleted composite Azure role definition")
		}
	}

	return nil, nil
}

// authorizeRoleTemporal sequences Azure role authorization as two independent Temporal activities.
func (p *azureProvider) authorizeRoleTemporal(
	wfCtx workflow.Context,
	req *models.AuthorizeRoleRequest,
) (*models.AuthorizeRoleResponse, error) {
	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to authorize azure role")
	}

	identifier := p.GetIdentifier()
	ao := workflow.ActivityOptions{
		StartToCloseTimeout:    2 * time.Minute,
		ScheduleToCloseTimeout: 10 * time.Minute,
		RetryPolicy:            sdkWorkflowsRunner.DefaultRetryPolicy,
	}
	wfCtx = workflow.WithActivityOptions(wfCtx, ao)

	user := req.GetUser()
	role := req.GetRole()

	// Composite-aware role name: unique for composite, base for non-composite.
	roleName := role.GetName()

	// Step 1 — GetOrCreateRoleDefinition
	var roleDefResp GetOrCreateRoleDefinitionResponse
	if err := workflow.ExecuteActivity(
		wfCtx,
		models.CreateTemporalProviderWorkflowName(identifier, GetOrCreateRoleDefinitionActivityName),
		&GetOrCreateRoleDefinitionRequest{
			RoleIdentifier: roleName,
			Description:    role.GetDescription(),
			Permissions:    role.Permissions,
		},
	).Get(wfCtx, &roleDefResp); err != nil {
		return nil, fmt.Errorf("GetOrCreateRoleDefinition activity failed: %w", err)
	}

	// Step 2 — CreateRoleAssignment
	var assignResp CreateRoleAssignmentResponse
	if err := workflow.ExecuteActivity(
		wfCtx,
		models.CreateTemporalProviderWorkflowName(identifier, CreateRoleAssignmentActivityName),
		&CreateRoleAssignmentRequest{
			User:             user,
			RoleDefinitionID: roleDefResp.RoleDefinitionID,
		},
	).Get(wfCtx, &assignResp); err != nil {
		return nil, fmt.Errorf("CreateRoleAssignment activity failed: %w", err)
	}

	return &models.AuthorizeRoleResponse{
		UserId: user.Email,
		Roles:  []string{roleName},
		Metadata: map[string]any{
			PrincipalIdentifierMetadataKey:      assignResp.PrincipalID,
			RoleDefinitionIdentifierMetadataKey: roleDefResp.RoleDefinitionID,
		},
	}, nil
}

// revokeRoleTemporal sequences Azure role revocation as Temporal activities.
//
// Composite roles: delete assignment + delete role definition.
// Non-composite roles: delete assignment only.
func (p *azureProvider) revokeRoleTemporal(
	wfCtx workflow.Context,
	req *models.RevokeRoleRequest,
) (*models.RevokeRoleResponse, error) {
	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to revoke azure role")
	}

	identifier := p.GetIdentifier()
	ao := workflow.ActivityOptions{
		StartToCloseTimeout:    2 * time.Minute,
		ScheduleToCloseTimeout: 10 * time.Minute,
		RetryPolicy:            sdkWorkflowsRunner.DefaultRetryPolicy,
	}
	wfCtx = workflow.WithActivityOptions(wfCtx, ao)

	user := req.GetUser()
	role := req.GetRole()

	isComposite := role.IsComposite()

	// Use composite-aware name for lookup.
	roleName := role.GetName()

	if req.AuthorizeRoleResponse == nil || req.AuthorizeRoleResponse.Metadata == nil {
		return nil, fmt.Errorf("missing authorization response metadata for revocation")
	}

	// A missing or wrong-type principal_id indicates corrupted authorization
	// metadata; retrying will not fix the root cause so we surface it immediately
	// as a non-retryable error.
	principalID, ok := req.AuthorizeRoleResponse.Metadata[PrincipalIdentifierMetadataKey].(string)
	if !ok || len(principalID) == 0 {
		return nil, temporal.NewNonRetryableApplicationError(
			fmt.Sprintf("invalid or missing principal_id in authorization metadata for user '%s'", req.GetUser().Email),
			"AzureMissingPrincipalID",
			nil,
		)
	}

	// Step 1 — GetRoleDefinition
	var roleDefResp GetRoleDefinitionResponse
	if err := workflow.ExecuteActivity(
		wfCtx,
		models.CreateTemporalProviderWorkflowName(identifier, GetRoleDefinitionActivityName),
		&GetRoleDefinitionRequest{RoleIdentifier: roleName},
	).Get(wfCtx, &roleDefResp); err != nil {
		return nil, fmt.Errorf("GetRoleDefinition activity failed: %w", err)
	}

	// Step 2 — DeleteRoleAssignment
	if err := workflow.ExecuteActivity(
		wfCtx,
		models.CreateTemporalProviderWorkflowName(identifier, DeleteRoleAssignmentActivityName),
		&DeleteRoleAssignmentRequest{
			User:             user,
			RoleDefinitionID: roleDefResp.RoleDefinitionID,
			PrincipalID:      principalID,
		},
	).Get(wfCtx, nil); err != nil {
		return nil, fmt.Errorf("DeleteRoleAssignment activity failed: %w", err)
	}

	// Step 3 — For composite roles, delete the role definition.
	if isComposite {
		if err := workflow.ExecuteActivity(
			wfCtx,
			models.CreateTemporalProviderWorkflowName(identifier, DeleteRoleDefinitionActivityName),
			&DeleteRoleDefinitionRequest{
				RoleDefinitionID: roleDefResp.RoleDefinitionID,
			},
		).Get(wfCtx, nil); err != nil {
			// Non-fatal: the assignment was already removed. Log but don't fail the workflow.
			logrus.WithError(err).WithField("role_definition_id", roleDefResp.RoleDefinitionID).
				Warn("DeleteRoleDefinition activity failed for composite role")
		}
	}

	return nil, nil
}

// GetAuthorizedAccessUrl returns the Azure Portal URL for the configured subscription,
// providing a direct link into the subscription's resource view.
func (p *azureProvider) GetAuthorizedAccessUrl(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
	resp *models.AuthorizeRoleResponse,
) string {
	if p.subscriptionID != "" {
		return fmt.Sprintf("https://portal.azure.com/#@/resource/subscriptions/%s", p.subscriptionID)
	}
	return "https://portal.azure.com/"
}

// permissionsToAzureActions converts CSP-agnostic Permissions to Azure role
// actions. Allow statements become Actions (or DataActions for data-plane
// operations), Deny statements become NotActions (or NotDataActions).
// Returns: (actions, notActions, dataActions, notDataActions, assignableScopes)
func permissionsToAzureActions(permissions models.RolePermissions) (actions, notActions, dataActions, notDataActions []string, targets []string) {
	// Use a map for efficient target deduplication
	targetSet := make(map[string]bool)

	// Process Allow statements — route to Actions or DataActions based on namespace.
	for _, stmt := range permissions.Allow {
		for _, op := range stmt.Operations {
			if isAzureDataAction(op) {
				dataActions = append(dataActions, op)
			} else {
				actions = append(actions, op)
			}
		}
		for _, target := range stmt.Targets {
			targetSet[target] = true
		}
	}

	// Process Deny statements — route to NotActions or NotDataActions.
	for _, stmt := range permissions.Deny {
		for _, op := range stmt.Operations {
			if isAzureDataAction(op) {
				notDataActions = append(notDataActions, op)
			} else {
				notActions = append(notActions, op)
			}
		}
		for _, target := range stmt.Targets {
			targetSet[target] = true
		}
	}

	// Convert target set to slice
	for target := range targetSet {
		targets = append(targets, target)
	}

	return actions, notActions, dataActions, notDataActions, targets
}

// dataActionPrefixes lists the well-known Azure data-plane operation namespaces.
// Operations with these prefixes must be placed in DataActions / NotDataActions
// rather than Actions / NotActions.
// Reference: https://learn.microsoft.com/en-us/azure/role-based-access-control/role-definitions
var dataActionPrefixes = []string{
	"Microsoft.Storage/storageAccounts/blobServices/",
	"Microsoft.Storage/storageAccounts/fileservices/",
	"Microsoft.Storage/storageAccounts/queueServices/",
	"Microsoft.Storage/storageAccounts/tableServices/",
	"Microsoft.KeyVault/vaults/secrets/",
	"Microsoft.KeyVault/vaults/keys/",
	"Microsoft.KeyVault/vaults/certificates/",
	"Microsoft.KeyVault/vaults/storage/",
	"Microsoft.CognitiveServices/accounts/",
	"Microsoft.DocumentDB/databaseAccounts/",
	"Microsoft.EventHub/namespaces/messages/",
	"Microsoft.ServiceBus/namespaces/messages/",
	"Microsoft.SignalRService/",
	"Microsoft.Web/sites/",
}

// isAzureDataAction returns true when the operation string corresponds to an
// Azure data-plane action that must go into DataActions rather than Actions.
func isAzureDataAction(operation string) bool {
	for _, prefix := range dataActionPrefixes {
		if strings.HasPrefix(strings.ToLower(operation), strings.ToLower(prefix)) {
			return true
		}
	}
	return false
}
