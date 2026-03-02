package gcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	sdkWorkflowsRunner "github.com/thand-io/agent/sdk/workflows/runner"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
	"google.golang.org/api/cloudresourcemanager/v1"
	crmv3 "google.golang.org/api/cloudresourcemanager/v3"
	"google.golang.org/api/googleapi"
	iam "google.golang.org/api/iam/v1"
)

// newThandCondition creates a new IAM condition used to tag bindings managed by thand
// We create a fresh copy each time to avoid shared state mutation
func newThandCondition() *cloudresourcemanager.Expr {
	return &cloudresourcemanager.Expr{
		Title:       "managed-by-thand",
		Description: "This binding is managed by thand",
		Expression:  "true", // Always evaluates to true, used as a tag
	}
}

// gcpRoleID converts a role name into a valid GCP custom role ID
// using common.ConvertToSnakeCase and enforcing GCP length constraints ({3,64}).
func gcpRoleID(name string) string {
	id := common.ConvertToSnakeCase(name)

	// Truncate to GCP maximum of 64 characters
	if len(id) > 64 {
		id = strings.TrimRight(id[:64], "_")
	}

	// Pad if too short (minimum 3 characters)
	for len(id) < 3 {
		id += "_"
	}

	return id
}

// primitiveRoles are GCP basic/primitive roles that do not support IAM conditions.
// See: https://cloud.google.com/iam/docs/conditions-overview#limitations
var primitiveRoles = []string{"roles/owner", "roles/editor", "roles/viewer"}

// isPrimitiveRole checks if a role is a GCP primitive/basic role.
func isPrimitiveRole(roleName string) bool {
	return slices.Contains(primitiveRoles, roleName)
}

// AuthorizeRole grants access for a user to a role.
//
// Role lifecycle:
//   - Composite roles: custom GCP role gets a unique name (hashed suffix).
//     On revoke the role is deleted.
//   - Non-composite roles: custom GCP role uses the base identifier so it
//     can be shared across users. On revoke only the IAM binding is removed.
//     A project label tracks the role version; permissions are patched only
//     when the version is stale.
func (p *gcpProvider) AuthorizeRole(
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
		return nil, fmt.Errorf("user and role must be provided to authorize gcp role")
	}

	user := req.GetUser()
	role := req.GetRole()
	isComposite := role.IsComposite()

	logrus.WithFields(logrus.Fields{
		"role":         role.GetName(),
		"is_composite": isComposite,
	}).Info("GCP authorizeRole: determining role lifecycle")

	if len(role.Inherits) == 0 && len(role.Permissions.Allow) == 0 {
		return nil, fmt.Errorf("role %s has no inherits or permissions defined", role.Name)
	}

	config := p.GetConfig()
	projectId := p.GetProjectId()
	var tenant *models.ProviderTenant
	if req.HasTenant() {
		tenant = req.GetTenant()
		projectId = tenant.ID
	}
	stage := config.GetStringWithDefault("stage", "GA")

	// GCP custom roles are project-scoped only (created via Projects.Roles API).
	// Reject requests that attempt to use custom permissions on a folder tenant.
	if isFolderResource(tenant) && len(role.Permissions.Allow) > 0 {
		return nil, fmt.Errorf("custom roles (permissions.allow) are not supported for folder-level resources (%s); GCP custom roles can only be created at the project level", projectId)
	}

	var assignedRoles []string

	// If inherits is specified, validate and bind predefined GCP roles
	if len(role.Inherits) > 0 {
		for _, inheritedRole := range role.Inherits {
			// Validate that the role is a valid GCP predefined role
			predefinedRole, err := p.GetRole(localCtx, inheritedRole)
			if err != nil {
				return nil, fmt.Errorf("invalid GCP role '%s': %w", inheritedRole, err)
			}

			// Bind the user to the predefined role via IAM policy
			err = p.bindUserToPredefinedRole(localCtx, projectId, user, predefinedRole.Name, tenant)
			if err != nil {
				return nil, temporal.NewApplicationErrorWithOptions(
					fmt.Sprintf("failed to bind user to role %s: %v", predefinedRole.Name, err),
					"GcpRoleBindingError",
					temporal.ApplicationErrorOptions{
						NextRetryDelay: 3 * time.Second,
						Cause:          err,
					},
				)
			}

			logrus.WithFields(logrus.Fields{
				"user_email": user.Email,
				"role":       predefinedRole.Name,
				"project_id": projectId,
			}).Info("Successfully bound user to predefined GCP role")

			assignedRoles = append(assignedRoles, predefinedRole.Name)
		}
	}

	// If permissions are specified, create a custom role with those permissions.
	// Composite roles get a unique name; non-composite roles share a base identifier.
	if len(role.Permissions.Allow) > 0 {
		customRoleName := gcpRoleID(role.GetName())

		existingRole, err := p.getRole(localCtx, projectId, customRoleName)
		if err != nil {
			// If role doesn't exist, create it
			existingRole, err = p.createRole(
				localCtx,
				projectId,
				customRoleName,
				role.Role.GetName(),
				role.GetDescription(),
				stage,
				role.Permissions,
			)
			if err != nil {
				return nil, temporal.NewApplicationErrorWithOptions(
					fmt.Sprintf("failed to create custom role %s: %v", customRoleName, err),
					"GcpCustomRoleCreationError",
					temporal.ApplicationErrorOptions{
						NextRetryDelay: 3 * time.Second,
						Cause:          err,
					},
				)
			}

			// For non-composite roles, record the version in a project label.
			if !isComposite {
				p.setRoleVersionLabel(localCtx, projectId, customRoleName, role.GetVersionString())
			}

			logrus.WithFields(logrus.Fields{
				"role_name":    customRoleName,
				"project_id":   projectId,
				"is_composite": isComposite,
				"permissions":  role.Permissions.Allow,
			}).Info("Created custom GCP role")
		} else {
			// Role already exists — for non-composite roles, check version first.
			needsUpdate := true
			if !isComposite {
				storedVersion := p.getRoleVersionLabel(localCtx, projectId, customRoleName)
				requestedVersion := role.GetVersionString()
				if storedVersion == requestedVersion {
					needsUpdate = false
					logrus.WithFields(logrus.Fields{
						"role_name": customRoleName,
						"version":   requestedVersion,
					}).Info("Non-composite role version is current; skipping permission refresh")
				}
			}

			if needsUpdate {
				existingRole, err = p.patchRoleIfStale(localCtx, projectId, existingRole, role.Permissions)
				if err != nil {
					return nil, fmt.Errorf("failed to update custom role %s: %w", customRoleName, err)
				}
				// Update version label after patching (non-composite only).
				if !isComposite {
					p.setRoleVersionLabel(localCtx, projectId, customRoleName, role.GetVersionString())
				}
			}
		}

		// Bind the user to the custom role via IAM policy
		err = p.bindUserToRole(localCtx, projectId, user, existingRole, tenant)
		if err != nil {
			return nil, temporal.NewApplicationErrorWithOptions(
				fmt.Sprintf("failed to bind user to custom role %s: %v", existingRole.Name, err),
				"GcpCustomRoleBindingError",
				temporal.ApplicationErrorOptions{
					NextRetryDelay: 3 * time.Second,
					Cause:          err,
				},
			)
		}

		logrus.WithFields(logrus.Fields{
			"user_email": user.Email,
			"role":       existingRole.Name,
			"project_id": projectId,
		}).Info("Successfully bound user to custom GCP role")

		assignedRoles = append(assignedRoles, existingRole.Name)
	}

	return &models.AuthorizeRoleResponse{
		UserId: user.Email,
		Roles:  assignedRoles,
	}, nil
}

// Revoke removes access for a user from a role.
//
// Role lifecycle on revoke:
//   - Composite: unbind user + delete custom role.
//   - Non-composite: unbind user only; the custom role is retained.
func (p *gcpProvider) RevokeRole(
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
		return nil, fmt.Errorf("user and role must be provided to revoke gcp role")
	}

	user := req.GetUser()
	role := req.GetRole()
	projectId := p.GetProjectId()
	var tenant *models.ProviderTenant
	if req.HasTenant() {
		tenant = req.GetTenant()
		projectId = tenant.ID
	}

	isComposite := role.IsComposite()

	logrus.WithFields(logrus.Fields{
		"role":         role.GetName(),
		"is_composite": isComposite,
	}).Info("GCP revokeRole: determining cleanup strategy")

	if req.AuthorizeRoleResponse == nil {
		return nil, fmt.Errorf("no authorize role response found for revocation")
	}

	// Get the roles that were assigned during authorization
	metadata := req.AuthorizeRoleResponse

	if len(metadata.Roles) == 0 {
		return nil, fmt.Errorf("no roles found in authorization response for revocation")
	}

	// Revoke each role that was assigned
	for _, roleName := range metadata.Roles {
		// Check if this is a predefined role (starts with "roles/") or custom role (starts with "projects/")
		if strings.HasPrefix(roleName, "roles/") {
			// Predefined role - unbind directly by role name
			err := p.unbindUserFromPredefinedRole(localCtx, projectId, user, roleName, tenant)
			if err != nil {
				return nil, temporal.NewApplicationErrorWithOptions(
					fmt.Sprintf("failed to unbind user from predefined role %s: %v", roleName, err),
					"GcpRoleUnbindingError",
					temporal.ApplicationErrorOptions{
						NextRetryDelay: 3 * time.Second,
						Cause:          err,
					},
				)
			}

			logrus.WithFields(logrus.Fields{
				"user_email": user.Email,
				"role":       roleName,
				"project_id": projectId,
			}).Info("Successfully unbound user from predefined GCP role")
		} else {
			// Custom role - get the role object and unbind
			customRoleName, err := parseCustomRolePath(roleName)
			if err != nil {
				return nil, err
			}

			existingRole, err := p.getRole(localCtx, projectId, customRoleName)
			if err != nil {
				return nil, temporal.NewApplicationErrorWithOptions(
					fmt.Sprintf("failed to get custom role %s: %v", customRoleName, err),
					"GcpGetRoleError",
					temporal.ApplicationErrorOptions{
						NextRetryDelay: 3 * time.Second,
						Cause:          err,
					},
				)
			}

			err = p.unbindUserFromRole(localCtx, projectId, user, existingRole, tenant)
			if err != nil {
				return nil, temporal.NewApplicationErrorWithOptions(
					fmt.Sprintf("failed to unbind user from custom role %s: %v", roleName, err),
					"GcpCustomRoleUnbindingError",
					temporal.ApplicationErrorOptions{
						NextRetryDelay: 3 * time.Second,
						Cause:          err,
					},
				)
			}

			logrus.WithFields(logrus.Fields{
				"user_email": user.Email,
				"role":       roleName,
				"project_id": projectId,
			}).Info("Successfully unbound user from custom GCP role")

			// Composite: delete the custom role after unbinding.
			// Non-composite: retain the role for future authorizations.
			if isComposite {
				err = p.deleteRole(localCtx, projectId, customRoleName)
				if err != nil {
					return nil, temporal.NewApplicationErrorWithOptions(
						fmt.Sprintf("failed to delete custom role %s: %v", customRoleName, err),
						"GcpCustomRoleDeletionError",
						temporal.ApplicationErrorOptions{
							NextRetryDelay: 3 * time.Second,
							Cause:          err,
						},
					)
				}

				logrus.WithFields(logrus.Fields{
					"role_name":  customRoleName,
					"project_id": projectId,
					"user_email": user.Email,
				}).Info("Successfully deleted custom GCP role (composite)")
			} else {
				logrus.WithFields(logrus.Fields{
					"role_name":  customRoleName,
					"project_id": projectId,
					"user_email": user.Email,
				}).Info("Retained custom GCP role (non-composite); only IAM binding removed")
			}
		}
	}

	return &models.RevokeRoleResponse{}, nil
}

func (p *gcpProvider) GetAuthorizedAccessUrl(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
	resp *models.AuthorizeRoleResponse,
) string {

	consoleUrl := fmt.Sprintf("https://console.cloud.google.com/welcome?project=%s", p.GetProjectId())

	return p.GetConfig().GetStringWithDefault(
		"sso_start_url", consoleUrl)
}

// authorizeRoleTemporal sequences GCP role authorization as independent Temporal activities.
func (p *gcpProvider) authorizeRoleTemporal(
	wfCtx workflow.Context,
	req *models.AuthorizeRoleRequest,
) (*models.AuthorizeRoleResponse, error) {
	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to authorize gcp role")
	}

	identifier := p.GetIdentifier()
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
	}
	wfCtx = workflow.WithActivityOptions(wfCtx, ao)

	user := req.GetUser()
	role := req.GetRole()
	isComposite := role.IsComposite()
	projectID := p.GetProjectId()
	var tenant *models.ProviderTenant
	if req.HasTenant() {
		tenant = req.GetTenant()
		projectID = tenant.ID
	}
	stage := p.GetConfig().GetStringWithDefault("stage", "GA")

	if len(role.Inherits) == 0 && len(role.Permissions.Allow) == 0 {
		return nil, fmt.Errorf("role %s has no inherits or permissions defined", role.Name)
	}

	// GCP custom roles are project-scoped only (created via Projects.Roles API).
	// Reject requests that attempt to use custom permissions on a folder tenant.
	if isFolderResource(tenant) && len(role.Permissions.Allow) > 0 {
		return nil, fmt.Errorf("custom roles (permissions.allow) are not supported for folder-level resources (%s); GCP custom roles can only be created at the project level", projectID)
	}

	var assignedRoles []string

	for _, inheritedRole := range role.Inherits {
		var resp BindUserToPredefinedRoleResponse
		if err := workflow.ExecuteActivity(
			wfCtx,
			models.CreateTemporalProviderWorkflowName(identifier, BindUserToPredefinedRoleActivityName),
			&BindUserToPredefinedRoleRequest{
				ProjectID:     projectID,
				User:          user,
				InheritedRole: inheritedRole,
				Tenant:        tenant,
			},
		).Get(wfCtx, &resp); err != nil {
			return nil, fmt.Errorf("BindUserToPredefinedRole activity failed for %s: %w", inheritedRole, err)
		}
		assignedRoles = append(assignedRoles, resp.RoleName)
	}

	if len(role.Permissions.Allow) > 0 {
		// Composite roles get a unique name; non-composite roles share a base identifier.
		customRoleName := gcpRoleID(role.GetName())

		var resp GetOrCreateAndBindCustomRoleResponse
		if err := workflow.ExecuteActivity(
			wfCtx,
			models.CreateTemporalProviderWorkflowName(identifier, GetOrCreateAndBindCustomRoleActivityName),
			&GetOrCreateAndBindCustomRoleRequest{
				ProjectID:   projectID,
				User:        user,
				RoleName:    customRoleName,
				Title:       role.Role.GetName(),
				Description: role.GetDescription(),
				Stage:       stage,
				Permissions: role.Permissions,
				IsComposite: isComposite,
				Version:     role.GetVersionString(),
				Tenant:      tenant,
			},
		).Get(wfCtx, &resp); err != nil {
			return nil, fmt.Errorf("GetOrCreateAndBindCustomRole activity failed: %w", err)
		}
		assignedRoles = append(assignedRoles, resp.RoleName)
	}

	return &models.AuthorizeRoleResponse{
		UserId: user.Email,
		Roles:  assignedRoles,
	}, nil
}

// revokeRoleTemporal sequences GCP role revocation as independent Temporal activities.
//
// Composite roles: unbind user + delete custom role (UnbindAndDeleteCustomRole).
// Non-composite roles: unbind user only (UnbindUserFromCustomRole).
func (p *gcpProvider) revokeRoleTemporal(
	wfCtx workflow.Context,
	req *models.RevokeRoleRequest,
) (*models.RevokeRoleResponse, error) {
	identifier := p.GetIdentifier()
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
	}
	wfCtx = workflow.WithActivityOptions(wfCtx, ao)

	user := req.GetUser()
	role := req.GetRole()
	projectID := p.GetProjectId()
	var tenant *models.ProviderTenant
	if req.HasTenant() {
		tenant = req.GetTenant()
		projectID = tenant.ID
	}

	// Determine composite flag from the role.
	isComposite := role.IsComposite()

	if req.AuthorizeRoleResponse == nil || len(req.AuthorizeRoleResponse.Roles) == 0 {
		return nil, fmt.Errorf("no roles found in authorization response for revocation")
	}

	for _, roleName := range req.AuthorizeRoleResponse.Roles {
		if strings.HasPrefix(roleName, "roles/") {
			if err := workflow.ExecuteActivity(
				wfCtx,
				models.CreateTemporalProviderWorkflowName(identifier, UnbindUserFromPredefinedRoleActivityName),
				&UnbindUserFromPredefinedRoleRequest{
					ProjectID: projectID,
					User:      user,
					RoleName:  roleName,
					Tenant:    tenant,
				},
			).Get(wfCtx, nil); err != nil {
				return nil, fmt.Errorf("UnbindUserFromPredefinedRole activity failed for %s: %w", roleName, err)
			}
		} else if isComposite {
			// Composite: unbind + delete the custom role.
			if err := workflow.ExecuteActivity(
				wfCtx,
				models.CreateTemporalProviderWorkflowName(identifier, UnbindAndDeleteCustomRoleActivityName),
				&UnbindAndDeleteCustomRoleRequest{
					ProjectID: projectID,
					User:      user,
					RoleName:  roleName,
					Tenant:    tenant,
				},
			).Get(wfCtx, nil); err != nil {
				return nil, fmt.Errorf("UnbindAndDeleteCustomRole activity failed for %s: %w", roleName, err)
			}
		} else {
			// Non-composite: unbind only; retain the custom role.
			if err := workflow.ExecuteActivity(
				wfCtx,
				models.CreateTemporalProviderWorkflowName(identifier, UnbindUserFromCustomRoleActivityName),
				&UnbindUserFromCustomRoleRequest{
					ProjectID: projectID,
					User:      user,
					RoleName:  roleName,
					Tenant:    tenant,
				},
			).Get(wfCtx, nil); err != nil {
				return nil, fmt.Errorf("UnbindUserFromCustomRole activity failed for %s: %w", roleName, err)
			}
		}
	}
	return &models.RevokeRoleResponse{}, nil
}

// createRole creates a custom role.
func (p *gcpProvider) createRole(ctx context.Context, projectID, name, title, description, stage string, permissions models.RolePermissions) (*iam.Role, error) {
	service := p.GetIamClient()

	// Convert permissions to GCP format
	gcpPermissions := permissionsToGcpPermissions(permissions)

	request := &iam.CreateRoleRequest{
		Role: &iam.Role{
			Title:               title,
			Description:         description,
			IncludedPermissions: gcpPermissions,
			Stage:               stage,
		},
		RoleId: name,
	}
	role, err := service.Projects.Roles.Create("projects/"+projectID, request).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("Projects.Roles.Create: %w", err)
	}
	return role, nil
}

func (p *gcpProvider) getRole(ctx context.Context, projectID, roleName string) (*iam.Role, error) {
	service := p.GetIamClient()

	role, err := service.Projects.Roles.Get("projects/" + projectID + "/roles/" + roleName).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return role, nil
}

// deleteRole deletes a custom role.
func (p *gcpProvider) deleteRole(ctx context.Context, projectID, roleName string) error {
	service := p.GetIamClient()

	_, err := service.Projects.Roles.Delete("projects/" + projectID + "/roles/" + roleName).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}
	return nil
}

// bindUserToPredefinedRole binds a user to a predefined GCP role (e.g., roles/viewer)
func (p *gcpProvider) bindUserToPredefinedRole(ctx context.Context, projectID string, user *models.User, roleName string, tenant *models.ProviderTenant) error {
	return p.bindUserToRoleByName(ctx, projectID, user, roleName, tenant)
}

// unbindUserFromPredefinedRole removes a user from a predefined GCP role
func (p *gcpProvider) unbindUserFromPredefinedRole(ctx context.Context, projectID string, user *models.User, roleName string, tenant *models.ProviderTenant) error {
	return p.unbindUserFromRoleByName(ctx, projectID, user, roleName, tenant)
}

// isThandManagedBinding checks if a binding has the thand condition tag
func isThandManagedBinding(binding *cloudresourcemanager.Binding) bool {
	return binding.Condition != nil && binding.Condition.Title == "managed-by-thand"
}

// validateAndFormatMember validates the user email and returns a formatted IAM member string
func validateAndFormatMember(user *models.User) (string, error) {
	if user == nil {
		return "", fmt.Errorf("user is required for GCP IAM binding")
	}
	if len(user.Email) == 0 {
		return "", fmt.Errorf("user email is required for GCP IAM binding")
	}
	if !common.IsValidEmail(user.Email) {
		return "", fmt.Errorf("invalid email format for GCP IAM binding: %s", user.Email)
	}
	return "user:" + user.Email, nil
}

// addMemberToPolicy adds a member to a role binding in the policy, creating a new binding if necessary
// Returns true if the policy was modified
func addMemberToPolicy(policy *cloudresourcemanager.Policy, roleName, member string) bool {
	isPrimitive := isPrimitiveRole(roleName)

	// Log info when using a primitive role since conditions cannot be applied
	if isPrimitive {
		logrus.WithFields(logrus.Fields{
			"role":   roleName,
			"member": member,
		}).Info("Binding to primitive role - IAM conditions are not supported for primitive roles. Tracking via member-scoped matching.")
	}

	// Check if binding already exists
	for _, binding := range policy.Bindings {
		if binding.Role == roleName {
			// For primitive roles, match any binding without condition
			// For other roles, match our thand-managed binding
			if (isPrimitive && binding.Condition == nil) || (!isPrimitive && isThandManagedBinding(binding)) {
				if slices.Contains(binding.Members, member) {
					return false // Already bound, no modification needed
				}
				// Add member to existing binding
				binding.Members = append(binding.Members, member)
				return true
			}
		}
	}

	// No binding exists for this role, create a new one
	newBinding := &cloudresourcemanager.Binding{
		Role:    roleName,
		Members: []string{member},
	}

	// Only add condition for non-primitive roles
	if !isPrimitive {
		newBinding.Condition = newThandCondition()
	}

	policy.Bindings = append(policy.Bindings, newBinding)
	return true
}

// removeMemberFromPolicy removes a member from a role binding in the policy
// Returns true if the member was found and removed, false otherwise
//
// For primitive roles: Uses member-scoped matching (removes only the specific member from unconditioned bindings)
// For other roles: Uses IAM condition-based matching (removes from thand-managed bindings only)
func removeMemberFromPolicy(policy *cloudresourcemanager.Policy, roleName, member string) bool {
	isPrimitive := isPrimitiveRole(roleName)

	for i, binding := range policy.Bindings {
		if binding.Role != roleName {
			continue
		}

		// For primitive roles, match any binding without condition
		// For other roles, match our thand-managed binding
		if (isPrimitive && binding.Condition == nil) || (!isPrimitive && isThandManagedBinding(binding)) {
			// Find the member index first, then remove outside the loop
			memberIndex := -1
			for j, bindingMember := range binding.Members {
				if bindingMember == member {
					memberIndex = j
					break
				}
			}
			if memberIndex == -1 {
				return false // Member not found in binding
			}

			// Log when removing from primitive role (for transparency)
			if isPrimitive {
				logrus.WithFields(logrus.Fields{
					"role":   roleName,
					"member": member,
				}).Info("Removing member from primitive role binding - only this specific member will be removed")
			}

			// Remove the member from the slice (outside the iteration loop)
			binding.Members = append(binding.Members[:memberIndex], binding.Members[memberIndex+1:]...)
			// If the binding has no members left, remove the entire binding
			if len(binding.Members) == 0 {
				policy.Bindings = append(policy.Bindings[:i], policy.Bindings[i+1:]...)
			}
			return true
		}
	}
	return false // Binding not found
}

// parseCustomRolePath extracts the short role name from a full custom role resource path.
// Expected format: projects/{project}/roles/{roleName}
func parseCustomRolePath(fullPath string) (string, error) {
	parts := strings.Split(fullPath, "/")
	if len(parts) != 4 || len(parts[3]) == 0 {
		return "", fmt.Errorf("invalid custom role name format: %q, expected projects/{project}/roles/{roleName}", fullPath)
	}
	return parts[3], nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Project-label based version tracking for non-composite roles
// ─────────────────────────────────────────────────────────────────────────────

// roleVersionLabelKey returns the GCP project label key for a role's version.
// Uses common.ConvertToSnakeCase for sanitization then enforces the 63-char GCP label limit.
func roleVersionLabelKey(roleName string) string {
	key := "thand_role_" + common.ConvertToSnakeCase(roleName)
	if len(key) > 63 {
		key = strings.TrimRight(key[:63], "_")
	}
	return key
}

// getRoleVersionLabel reads the version stored for a custom role in project labels.
// Returns an empty string if the label does not exist or the API call fails.
func (p *gcpProvider) getRoleVersionLabel(ctx context.Context, projectID, roleName string) string {
	if p.crmV3Client == nil {
		return ""
	}
	project, err := p.crmV3Client.Projects.Get("projects/" + projectID).Context(ctx).Do()
	if err != nil {
		logrus.WithError(err).WithField("project_id", projectID).Warn("Failed to read project labels for version check")
		return ""
	}
	if project.Labels == nil {
		return ""
	}
	return project.Labels[roleVersionLabelKey(roleName)]
}

// setRoleVersionLabel writes the version for a custom role into a project label.
// Errors are logged as warnings — labeling failures should not block authorization.
func (p *gcpProvider) setRoleVersionLabel(ctx context.Context, projectID, roleName, version string) {
	if p.crmV3Client == nil {
		return
	}
	labelKey := roleVersionLabelKey(roleName)

	// Read current labels.
	project, err := p.crmV3Client.Projects.Get("projects/" + projectID).Context(ctx).Do()
	if err != nil {
		logrus.WithError(err).WithField("project_id", projectID).Warn("Failed to read project for label update")
		return
	}
	if project.Labels == nil {
		project.Labels = make(map[string]string)
	}
	// Sanitize version value for GCP label (max 63 chars).
	sanitizedVersion := common.ConvertToSnakeCase(version)
	if len(sanitizedVersion) > 63 {
		sanitizedVersion = strings.TrimRight(sanitizedVersion[:63], "_")
	}
	project.Labels[labelKey] = sanitizedVersion

	_, err = p.crmV3Client.Projects.Patch("projects/"+projectID, project).
		UpdateMask("labels").
		Context(ctx).
		Do()
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"project_id": projectID,
			"label_key":  labelKey,
			"version":    sanitizedVersion,
		}).Warn("Failed to set project label for role version")
	}
}

// permissionsEqual returns true when two permission slices contain the same elements (order-independent).
func permissionsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aSorted := make([]string, len(a))
	bSorted := make([]string, len(b))
	copy(aSorted, a)
	copy(bSorted, b)
	sort.Strings(aSorted)
	sort.Strings(bSorted)
	return slices.Equal(aSorted, bSorted)
}

// isEtagConflict reports whether a GCP API error is an IAM policy etag mismatch (HTTP 409).
func isEtagConflict(err error) bool {
	var e *googleapi.Error
	return errors.As(err, &e) && e.Code == http.StatusConflict
}

// patchRoleIfStale updates a custom role's included permissions when they differ from the desired set.
func (p *gcpProvider) patchRoleIfStale(ctx context.Context, projectID string, existingRole *iam.Role, permissions models.RolePermissions) (*iam.Role, error) {
	desired := permissionsToGcpPermissions(permissions)
	if permissionsEqual(existingRole.IncludedPermissions, desired) {
		return existingRole, nil
	}

	logrus.WithFields(logrus.Fields{
		"role":       existingRole.Name,
		"project_id": projectID,
	}).Info("Updating stale custom GCP role permissions")

	existingRole.IncludedPermissions = desired
	updated, err := p.iamClient.Projects.Roles.Patch(existingRole.Name, existingRole).
		UpdateMask("includedPermissions").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("failed to patch custom role permissions: %w", err)
	}
	return updated, nil
}

// maxPolicyRetries is the maximum number of retries for concurrent IAM policy update conflicts.
const maxPolicyRetries = 5

// withIAMPolicyUpdate atomically fetches, mutates, and writes the project IAM policy.
// It automatically retries on etag-mismatch conflicts (HTTP 409) up to maxPolicyRetries times,
// re-fetching the policy on each attempt to incorporate concurrent changes.
// mutateFn receives the current policy and returns (changed bool, err error);
// if changed is false the SetIamPolicy call is skipped.
func (p *gcpProvider) withIAMPolicyUpdate(
	ctx context.Context,
	projectID string,
	mutateFn func(*cloudresourcemanager.Policy) (bool, error),
) error {
	crmService := p.crmClient
	for attempt := 1; attempt <= maxPolicyRetries; attempt++ {
		policy, err := crmService.Projects.GetIamPolicy(projectID, &cloudresourcemanager.GetIamPolicyRequest{
			Options: &cloudresourcemanager.GetPolicyOptions{
				RequestedPolicyVersion: 3,
			},
		}).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("failed to get IAM policy: %w", err)
		}

		// Ensure policy version is 3 for conditions support
		policy.Version = 3

		changed, err := mutateFn(policy)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}

		_, err = crmService.Projects.SetIamPolicy(projectID, &cloudresourcemanager.SetIamPolicyRequest{
			Policy: policy,
		}).Context(ctx).Do()
		if err != nil {
			if isEtagConflict(err) && attempt < maxPolicyRetries {
				logrus.WithFields(logrus.Fields{
					"project_id": projectID,
					"attempt":    attempt,
				}).Debug("IAM policy etag conflict, retrying")
				time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
				continue
			}
			return fmt.Errorf("failed to set IAM policy: %w", err)
		}
		return nil
	}
	return fmt.Errorf("failed to update IAM policy for project %s after %d attempts due to concurrent modifications", projectID, maxPolicyRetries)
}

// isFolderResource returns true if the tenant represents a GCP folder resource.
func isFolderResource(tenant *models.ProviderTenant) bool {
	if tenant == nil {
		return false
	}
	return strings.EqualFold(tenant.Type, "folder")
}

// newThandConditionV3 creates the thand-managed condition tag using the v3 CRM types.
func newThandConditionV3() *crmv3.Expr {
	return &crmv3.Expr{
		Title:       "managed-by-thand",
		Description: "This binding is managed by thand",
		Expression:  "true",
	}
}

// isThandManagedBindingV3 checks if a v3 binding has the thand condition tag.
func isThandManagedBindingV3(binding *crmv3.Binding) bool {
	return binding.Condition != nil && binding.Condition.Title == "managed-by-thand"
}

// addMemberToPolicyV3 adds a member to a role binding in a v3 policy, creating a new binding if necessary.
// Returns true if the policy was modified.
func addMemberToPolicyV3(policy *crmv3.Policy, roleName, member string) bool {
	isPrimitive := isPrimitiveRole(roleName)

	if isPrimitive {
		logrus.WithFields(logrus.Fields{
			"role":   roleName,
			"member": member,
		}).Warn("Binding to primitive role without IAM condition - GCP does not support conditions on primitive roles. Consider using predefined roles instead for better tracking.")
	}

	for _, binding := range policy.Bindings {
		if binding.Role == roleName {
			if (isPrimitive && binding.Condition == nil) || (!isPrimitive && isThandManagedBindingV3(binding)) {
				if slices.Contains(binding.Members, member) {
					return false
				}
				binding.Members = append(binding.Members, member)
				return true
			}
		}
	}

	newBinding := &crmv3.Binding{
		Role:    roleName,
		Members: []string{member},
	}
	if !isPrimitive {
		newBinding.Condition = newThandConditionV3()
	}
	policy.Bindings = append(policy.Bindings, newBinding)
	return true
}

// removeMemberFromPolicyV3 removes a member from a role binding in a v3 policy.
// Returns true if the member was found and removed.
func removeMemberFromPolicyV3(policy *crmv3.Policy, roleName, member string) bool {
	isPrimitive := isPrimitiveRole(roleName)

	for i, binding := range policy.Bindings {
		if binding.Role != roleName {
			continue
		}
		if !isPrimitive && !isThandManagedBindingV3(binding) {
			continue
		}
		if isPrimitive && binding.Condition != nil {
			continue
		}

		idx := slices.Index(binding.Members, member)
		if idx == -1 {
			return false
		}

		binding.Members = slices.Delete(binding.Members, idx, idx+1)
		if len(binding.Members) == 0 {
			policy.Bindings = slices.Delete(policy.Bindings, i, i+1)
		}
		return true
	}
	return false
}

// withFolderIAMPolicyUpdate atomically fetches, mutates, and writes the IAM policy for a GCP folder.
// It automatically retries on etag-mismatch conflicts up to maxPolicyRetries times.
func (p *gcpProvider) withFolderIAMPolicyUpdate(
	ctx context.Context,
	folderID string,
	mutateFn func(*crmv3.Policy) (bool, error),
) error {
	crmV3Service := p.crmV3Client
	if crmV3Service == nil {
		return fmt.Errorf("Cloud Resource Manager v3 client is not initialized, cannot manage folder IAM policies")
	}

	for attempt := 1; attempt <= maxPolicyRetries; attempt++ {
		policy, err := crmV3Service.Folders.GetIamPolicy(folderID, &crmv3.GetIamPolicyRequest{
			Options: &crmv3.GetPolicyOptions{
				RequestedPolicyVersion: 3,
			},
		}).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("failed to get IAM policy for folder %s: %w", folderID, err)
		}

		policy.Version = 3

		changed, err := mutateFn(policy)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}

		_, err = crmV3Service.Folders.SetIamPolicy(folderID, &crmv3.SetIamPolicyRequest{
			Policy: policy,
		}).Context(ctx).Do()
		if err != nil {
			if isEtagConflict(err) && attempt < maxPolicyRetries {
				logrus.WithFields(logrus.Fields{
					"folder_id": folderID,
					"attempt":   attempt,
				}).Debug("IAM policy etag conflict, retrying")
				time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
				continue
			}
			return fmt.Errorf("failed to set IAM policy for folder %s: %w", folderID, err)
		}
		return nil
	}
	return fmt.Errorf("failed to update IAM policy for folder %s after %d attempts due to concurrent modifications", folderID, maxPolicyRetries)
}

func (p *gcpProvider) bindUserToRole(ctx context.Context, projectID string, user *models.User, iamRole *iam.Role, tenant *models.ProviderTenant) error {
	return p.bindUserToRoleByName(ctx, projectID, user, iamRole.Name, tenant)
}

func (p *gcpProvider) unbindUserFromRole(ctx context.Context, projectID string, user *models.User, iamRole *iam.Role, tenant *models.ProviderTenant) error {
	return p.unbindUserFromRoleByName(ctx, projectID, user, iamRole.Name, tenant)
}

// bindUserToRoleByName is the core implementation for binding a user to any role.
// It supports both project-level and folder-level IAM bindings.
func (p *gcpProvider) bindUserToRoleByName(ctx context.Context, resourceID string, user *models.User, roleName string, tenant *models.ProviderTenant) error {
	member, err := validateAndFormatMember(user)
	if err != nil {
		return err
	}

	if isFolderResource(tenant) {
		return p.withFolderIAMPolicyUpdate(ctx, resourceID, func(policy *crmv3.Policy) (bool, error) {
			return addMemberToPolicyV3(policy, roleName, member), nil
		})
	}

	return p.withIAMPolicyUpdate(ctx, resourceID, func(policy *cloudresourcemanager.Policy) (bool, error) {
		return addMemberToPolicy(policy, roleName, member), nil
	})
}

// unbindUserFromRoleByName is the core implementation for unbinding a user from any role.
// It supports both project-level and folder-level IAM bindings.
func (p *gcpProvider) unbindUserFromRoleByName(ctx context.Context, resourceID string, user *models.User, roleName string, tenant *models.ProviderTenant) error {
	member, err := validateAndFormatMember(user)
	if err != nil {
		return err
	}

	if isFolderResource(tenant) {
		return p.withFolderIAMPolicyUpdate(ctx, resourceID, func(policy *crmv3.Policy) (bool, error) {
			if !removeMemberFromPolicyV3(policy, roleName, member) {
				return false, fmt.Errorf("thand-managed role binding not found for role %s", roleName)
			}
			return true, nil
		})
	}

	return p.withIAMPolicyUpdate(ctx, resourceID, func(policy *cloudresourcemanager.Policy) (bool, error) {
		if !removeMemberFromPolicy(policy, roleName, member) {
			return false, fmt.Errorf("thand-managed role binding not found for role %s", roleName)
		}
		return true, nil
	})
}

// permissionsToGcpPermissions converts CSP-agnostic Permissions to GCP permissions list
// Only Allow statements are used (GCP custom roles don't support deny)
// Note: Targets are not used in GCP custom roles; resource scope is handled via IAM bindings
func permissionsToGcpPermissions(permissions models.RolePermissions) []string {
	var gcpPermissions []string

	// Process Allow statements
	for _, stmt := range permissions.Allow {
		gcpPermissions = append(gcpPermissions, stmt.Operations...)
	}

	// Log warning for Deny statements (GCP doesn't support deny in custom roles)
	if len(permissions.Deny) > 0 {
		logrus.Warnf("GCP custom roles don't support deny permissions, skipping %d deny statements", len(permissions.Deny))
	}

	// Deduplicate permissions
	uniquePerms := make(map[string]bool)
	result := make([]string, 0, len(gcpPermissions))
	for _, perm := range gcpPermissions {
		if !uniquePerms[perm] {
			uniquePerms[perm] = true
			result = append(result, perm)
		}
	}

	return result
}
