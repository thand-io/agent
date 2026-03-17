package gcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
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
		trimmed := strings.TrimRight(id[:64], "_")
		if len(trimmed) > 0 {
			id = trimmed
		} else {
			id = id[:3] // degenerate case: first 64 chars were all underscores
		}
	}

	// Pad if too short (minimum 3 characters)
	for len(id) < 3 {
		id += "_"
	}

	return id
}

// statementRoleID derives the GCP custom role ID for a single permission statement.
//
// When a role has exactly one allow statement, the base role name is used directly
// (backwards-compatible with existing single-role behaviour).
//
// When a role has multiple allow statements, each statement produces a distinct
// role. The suffix is the statement's ID field if set, otherwise "s{index}".
func statementRoleID(baseName string, stmt models.Statement, index, count int) string {
	if count <= 1 {
		return gcpRoleID(baseName)
	}
	if stmt.ID != "" {
		return gcpRoleID(baseName + "_" + stmt.ID)
	}
	return gcpRoleID(baseName + "_s" + strconv.Itoa(index))
}

// stmtLabel returns a human-readable label for a statement in error/log messages.
func stmtLabel(stmt models.Statement, index int) string {
	if stmt.ID != "" {
		return stmt.ID
	}
	return "s" + strconv.Itoa(index)
}

// primitiveRoles are GCP basic/primitive roles that do not support IAM conditions.
// See: https://cloud.google.com/iam/docs/conditions-overview#limitations
var primitiveRoles = []string{"roles/owner", "roles/editor", "roles/viewer"}

// isPrimitiveRole checks if a role is a GCP primitive/basic role.
func isPrimitiveRole(roleName string) bool {
	return slices.Contains(primitiveRoles, roleName)
}

func isOrganizationRoleParent(parent string) bool {
	return strings.HasPrefix(parent, "organizations/")
}

func (p *gcpProvider) getCustomRoleParent(resourceID string) string {
	if organizationID := p.GetOrganizationId(); len(organizationID) > 0 {
		return "organizations/" + organizationID
	}
	return "projects/" + resourceID
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

	var assignedRoles []string

	// If inherits is specified, validate and bind predefined GCP roles
	if len(role.Inherits) > 0 {
		for _, inheritedRole := range role.Inherits {
			predefinedRole, err := p.GetRole(localCtx, inheritedRole)
			if err != nil {
				return nil, fmt.Errorf("invalid GCP role '%s': %w", inheritedRole, err)
			}

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

	// Create a custom role per allow statement. Each statement independently
	// resolves its binding tenant and gets its own role ID.
	stmtCount := len(role.Permissions.Allow)
	for stmtIdx, stmt := range role.Permissions.Allow {
		stmtTenant, err := resolveStatementBindingTenant(stmt, tenant, projectId)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve binding for statement %s (index %d): %w", stmtLabel(stmt, stmtIdx), stmtIdx, err)
		}
		stmtResourceID := stmtTenant.ID

		customRoleName := statementRoleID(role.GetName(), stmt, stmtIdx, stmtCount)
		roleParent := p.getCustomRoleParent(stmtResourceID)

		stmtPermissions := models.RolePermissions{
			Allow: models.RoleStatements{stmt},
		}

		existingRole, err := p.getRole(localCtx, roleParent, customRoleName)
		if err != nil {
			existingRole, err = p.createRole(
				localCtx,
				roleParent,
				customRoleName,
				role.Role.GetName(),
				role.GetDescription(),
				stage,
				stmtPermissions,
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

			if !isComposite && !isOrganizationRoleParent(roleParent) {
				p.setRoleVersionLabel(localCtx, stmtResourceID, customRoleName, role.GetVersionString())
			}

			logrus.WithFields(logrus.Fields{
				"role_name":    customRoleName,
				"role_parent":  roleParent,
				"is_composite": isComposite,
				"stmt_index":   stmtIdx,
				"stmt_id":      stmt.ID,
			}).Info("Created custom GCP role for statement")
			logrus.WithFields(logrus.Fields{
				"role_name":  customRoleName,
				"binding":    stmt.Binding,
				"operations": stmt.Operations,
			}).Debug("Custom GCP role statement details")
		} else {
			needsUpdate := true
			if !isComposite && !isOrganizationRoleParent(roleParent) {
				storedVersion := p.getRoleVersionLabel(localCtx, stmtResourceID, customRoleName)
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
				existingRole, err = p.patchRoleIfStale(localCtx, existingRole, stmtPermissions)
				if err != nil {
					return nil, fmt.Errorf("failed to update custom role %s: %w", customRoleName, err)
				}
				if !isComposite && !isOrganizationRoleParent(roleParent) {
					p.setRoleVersionLabel(localCtx, stmtResourceID, customRoleName, role.GetVersionString())
				}
			}
		}

		err = p.bindUserToRole(localCtx, stmtResourceID, user, existingRole, stmtTenant)
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
			"project_id": stmtResourceID,
			"stmt_index": stmtIdx,
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
			roleParent, customRoleName, err := parseCustomRolePath(roleName)
			if err != nil {
				return nil, err
			}

			existingRole, err := p.getRole(localCtx, roleParent, customRoleName)
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

			// Derive tenant from the role's resource path — each per-statement
			// role encodes its binding project (e.g. projects/{project}/roles/{name}).
			customTenantForRole := tenant
			customResourceForRole := projectId
			if roleProjectID, ok := projectIDFromRoleParent(roleParent); ok {
				customTenantForRole = &models.ProviderTenant{
					ID:   roleProjectID,
					Type: "project",
					Name: roleProjectID,
				}
				customResourceForRole = roleProjectID
			}

			err = p.unbindUserFromRole(localCtx, customResourceForRole, user, existingRole, customTenantForRole)
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
				"project_id": customResourceForRole,
			}).Info("Successfully unbound user from custom GCP role")

			// Composite: delete the custom role after unbinding.
			// Non-composite: retain the role for future authorizations.
			if isComposite {
				err = p.deleteRole(localCtx, roleParent, customRoleName)
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
	u := &url.URL{
		Scheme: "https",
		Host:   "console.cloud.google.com",
		Path:   "/welcome",
	}
	query := url.Values{}

	if req.HasTenant() {
		tenant := req.GetTenant()
		if isFolderResource(tenant) {
			u.Path = "/"
			query.Set("folder", tenant.ID)
		} else {
			query.Set("project", tenant.ID)
		}
	} else {
		query.Set("project", p.GetProjectId())
	}

	u.RawQuery = query.Encode()
	return p.GetConfig().GetStringWithDefault("sso_start_url", u.String())
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
	} else {
		// Create synthetic tenant for project-level operations
		tenant = &models.ProviderTenant{
			ID:   projectID,
			Type: "project",
			Name: projectID,
		}
	}
	stage := p.GetConfig().GetStringWithDefault("stage", "GA")

	if len(role.Inherits) == 0 && len(role.Permissions.Allow) == 0 {
		return nil, fmt.Errorf("role %s has no inherits or permissions defined", role.Name)
	}

	var assignedRoles []string

	for _, inheritedRole := range role.Inherits {
		var resp BindUserToPredefinedRoleResponse
		if err := workflow.ExecuteActivity(
			wfCtx,
			models.CreateTemporalProviderWorkflowName(identifier, BindUserToPredefinedRoleActivityName),
			&BindUserToPredefinedRoleRequest{
				User:          user,
				InheritedRole: inheritedRole,
				Tenant:        tenant,
			},
		).Get(wfCtx, &resp); err != nil {
			return nil, fmt.Errorf("BindUserToPredefinedRole activity failed for %s: %w", inheritedRole, err)
		}
		assignedRoles = append(assignedRoles, resp.RoleName)
	}

	// Create a custom role per allow statement via separate activities.
	stmtCount := len(role.Permissions.Allow)
	for stmtIdx, stmt := range role.Permissions.Allow {
		stmtTenant, err := resolveStatementBindingTenant(stmt, tenant, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve binding for statement %s (index %d): %w", stmtLabel(stmt, stmtIdx), stmtIdx, err)
		}

		customRoleName := statementRoleID(role.GetName(), stmt, stmtIdx, stmtCount)
		stmtPermissions := models.RolePermissions{
			Allow: models.RoleStatements{stmt},
		}

		var resp GetOrCreateAndBindCustomRoleResponse
		if err := workflow.ExecuteActivity(
			wfCtx,
			models.CreateTemporalProviderWorkflowName(identifier, GetOrCreateAndBindCustomRoleActivityName),
			&GetOrCreateAndBindCustomRoleRequest{
				User:        user,
				RoleName:    customRoleName,
				Title:       role.Role.GetName(),
				Description: role.GetDescription(),
				Stage:       stage,
				Permissions: stmtPermissions,
				IsComposite: isComposite,
				Version:     role.GetVersionString(),
				Tenant:      stmtTenant,
			},
		).Get(wfCtx, &resp); err != nil {
			return nil, fmt.Errorf("GetOrCreateAndBindCustomRole activity failed for statement %s (index %d): %w", stmtLabel(stmt, stmtIdx), stmtIdx, err)
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
	} else {
		// Create synthetic tenant for project-level operations
		tenant = &models.ProviderTenant{
			ID:   projectID,
			Type: "project",
			Name: projectID,
		}
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
					User:     user,
					RoleName: roleName,
					Tenant:   tenant,
				},
			).Get(wfCtx, nil); err != nil {
				return nil, fmt.Errorf("UnbindUserFromPredefinedRole activity failed for %s: %w", roleName, err)
			}
		} else if isComposite {
			tenantForRole := tenant
			if roleParent, _, err := parseCustomRolePath(roleName); err == nil {
				if roleProjectID, ok := projectIDFromRoleParent(roleParent); ok {
					tenantForRole = &models.ProviderTenant{ID: roleProjectID, Type: "project", Name: roleProjectID}
				}
			}

			// Composite: unbind + delete the custom role.
			if err := workflow.ExecuteActivity(
				wfCtx,
				models.CreateTemporalProviderWorkflowName(identifier, UnbindAndDeleteCustomRoleActivityName),
				&UnbindAndDeleteCustomRoleRequest{
					User:     user,
					RoleName: roleName,
					Tenant:   tenantForRole,
				},
			).Get(wfCtx, nil); err != nil {
				return nil, fmt.Errorf("UnbindAndDeleteCustomRole activity failed for %s: %w", roleName, err)
			}
		} else {
			tenantForRole := tenant
			if roleParent, _, err := parseCustomRolePath(roleName); err == nil {
				if roleProjectID, ok := projectIDFromRoleParent(roleParent); ok {
					tenantForRole = &models.ProviderTenant{ID: roleProjectID, Type: "project", Name: roleProjectID}
				}
			}

			// Non-composite: unbind only; retain the custom role.
			if err := workflow.ExecuteActivity(
				wfCtx,
				models.CreateTemporalProviderWorkflowName(identifier, UnbindUserFromCustomRoleActivityName),
				&UnbindUserFromCustomRoleRequest{
					User:     user,
					RoleName: roleName,
					Tenant:   tenantForRole,
				},
			).Get(wfCtx, nil); err != nil {
				return nil, fmt.Errorf("UnbindUserFromCustomRole activity failed for %s: %w", roleName, err)
			}
		}
	}
	return &models.RevokeRoleResponse{}, nil
}

// createRole creates a custom role under projects/{id} or organizations/{id}.
func (p *gcpProvider) createRole(ctx context.Context, roleParent, name, title, description, stage string, permissions models.RolePermissions) (*iam.Role, error) {
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
	if isOrganizationRoleParent(roleParent) {
		role, err := service.Organizations.Roles.Create(roleParent, request).Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("Organizations.Roles.Create: %w", err)
		}
		return role, nil
	}

	role, err := service.Projects.Roles.Create(roleParent, request).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("Projects.Roles.Create: %w", err)
	}
	return role, nil
}

func (p *gcpProvider) getRole(ctx context.Context, roleParent, roleName string) (*iam.Role, error) {
	service := p.GetIamClient()
	roleResource := roleParent + "/roles/" + roleName
	var (
		role *iam.Role
		err  error
	)
	if isOrganizationRoleParent(roleParent) {
		role, err = service.Organizations.Roles.Get(roleResource).Context(ctx).Do()
	} else {
		role, err = service.Projects.Roles.Get(roleResource).Context(ctx).Do()
	}
	if err != nil {
		return nil, err
	}
	return role, nil
}

// deleteRole deletes a custom role under projects/{id} or organizations/{id}.
func (p *gcpProvider) deleteRole(ctx context.Context, roleParent, roleName string) error {
	service := p.GetIamClient()
	roleResource := roleParent + "/roles/" + roleName
	var err error
	if isOrganizationRoleParent(roleParent) {
		_, err = service.Organizations.Roles.Delete(roleResource).Context(ctx).Do()
	} else {
		_, err = service.Projects.Roles.Delete(roleResource).Context(ctx).Do()
	}
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

// parseCustomRolePath parses a full custom role resource path.
// Expected formats:
//   - projects/{project}/roles/{roleName}
//   - organizations/{organization}/roles/{roleName}
func parseCustomRolePath(fullPath string) (string, string, error) {
	parts := strings.Split(fullPath, "/")
	if len(parts) != 4 || len(parts[1]) == 0 || parts[2] != "roles" || len(parts[3]) == 0 {
		return "", "", fmt.Errorf("invalid custom role name format: %q, expected projects/{project}/roles/{roleName} or organizations/{organization}/roles/{roleName}", fullPath)
	}
	if parts[0] != "projects" && parts[0] != "organizations" {
		return "", "", fmt.Errorf("invalid custom role parent in %q: expected projects or organizations", fullPath)
	}
	return parts[0] + "/" + parts[1], parts[3], nil
}

func projectIDFromRoleParent(roleParent string) (string, bool) {
	if !strings.HasPrefix(roleParent, "projects/") {
		return "", false
	}
	projectID := strings.TrimPrefix(roleParent, "projects/")
	if len(projectID) == 0 {
		return "", false
	}
	return projectID, true
}

// resolveStatementBindingTenant determines the project tenant for a single permission
// statement. Used by the per-statement authorization loop.
//
// Resolution order:
//  1. If the statement has an explicit Binding, parse the project from it.
//  2. If the request tenant is not a folder, use it directly.
//  3. Fall back to inferring the project from the statement's Targets (legacy, deprecated).
func resolveStatementBindingTenant(stmt models.Statement, requestTenant *models.ProviderTenant, fallbackProjectID string) (*models.ProviderTenant, error) {
	if stmt.Binding != "" {
		binding := strings.TrimSpace(stmt.Binding)
		if strings.HasPrefix(binding, "folders/") {
			return nil, fmt.Errorf("binding %q targets a folder; GCP custom roles must be created at project or organization scope", binding)
		}
		if strings.HasPrefix(binding, "organizations/") {
			return nil, fmt.Errorf(
				"binding %q targets an organization; organization-level role creation via the 'binding' field is not currently supported — "+
					"set 'organization_id' in the provider configuration to create custom roles at organization scope",
				binding,
			)
		}
		projectID := strings.TrimPrefix(binding, "projects/")
		if len(projectID) == 0 {
			return nil, fmt.Errorf("binding %q does not resolve to a project", binding)
		}
		return &models.ProviderTenant{ID: projectID, Type: "project", Name: projectID}, nil
	}

	if !isFolderResource(requestTenant) {
		return requestTenant, nil
	}

	logrus.WithFields(logrus.Fields{
		"operations":       stmt.Operations,
		"fallback_project": fallbackProjectID,
	}).Warn(
		"permission statement is missing 'binding' and request tenant is a folder; " +
			"inferring project from targets. Set an explicit 'binding' to remove this warning.",
	)
	singleStmt := models.RoleStatements{stmt}
	projectID, err := inferProjectIDFromPermissionTargets(singleStmt)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve binding for statement with operations %v: %w", stmt.Operations, err)
	}
	return &models.ProviderTenant{ID: projectID, Type: "project", Name: projectID}, nil
}

func inferProjectIDFromPermissionTargets(statements models.RoleStatements) (string, error) {
	projectIDs := make(map[string]struct{})

	for _, statement := range statements {
		if len(statement.Targets) == 0 {
			return "", fmt.Errorf("permission statement with operations %v is missing targets", statement.Operations)
		}

		for _, target := range statement.Targets {
			projectID, ok := projectIDFromTarget(target)
			if !ok {
				return "", fmt.Errorf("target %q does not include a specific project path (expected projects/{project}/...)", target)
			}
			projectIDs[projectID] = struct{}{}
		}
	}

	if len(projectIDs) == 0 {
		return "", fmt.Errorf("no project targets found")
	}
	if len(projectIDs) > 1 {
		projects := make([]string, 0, len(projectIDs))
		for projectID := range projectIDs {
			projects = append(projects, projectID)
		}
		sort.Strings(projects)
		return "", fmt.Errorf("targets span multiple projects %v; split permissions into separate roles", projects)
	}

	for projectID := range projectIDs {
		return projectID, nil
	}

	return "", fmt.Errorf("no project targets found")
}

func projectIDFromTarget(target string) (string, bool) {
	target = strings.TrimSpace(target)
	if len(target) == 0 {
		return "", false
	}

	projectMarker := "projects/"
	_, after, ok := strings.Cut(target, projectMarker)
	if !ok {
		return "", false
	}

	remaining := after
	if len(remaining) == 0 {
		return "", false
	}

	nextSeparator := strings.Index(remaining, "/")
	if nextSeparator == -1 {
		nextSeparator = len(remaining)
	}

	projectID := remaining[:nextSeparator]
	if len(projectID) == 0 || projectID == "*" {
		return "", false
	}

	return projectID, true
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
func (p *gcpProvider) patchRoleIfStale(ctx context.Context, existingRole *iam.Role, permissions models.RolePermissions) (*iam.Role, error) {
	desired := permissionsToGcpPermissions(permissions)
	if permissionsEqual(existingRole.IncludedPermissions, desired) {
		return existingRole, nil
	}

	logrus.WithFields(logrus.Fields{
		"role": existingRole.Name,
	}).Info("Updating stale custom GCP role permissions")

	existingRole.IncludedPermissions = desired
	var (
		updated *iam.Role
		err     error
	)
	if strings.HasPrefix(existingRole.Name, "organizations/") {
		updated, err = p.iamClient.Organizations.Roles.Patch(existingRole.Name, existingRole).
			UpdateMask("includedPermissions").
			Context(ctx).
			Do()
	} else {
		updated, err = p.iamClient.Projects.Roles.Patch(existingRole.Name, existingRole).
			UpdateMask("includedPermissions").
			Context(ctx).
			Do()
	}
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
	if strings.EqualFold(tenant.Type, "folder") {
		return true
	}
	// Additionally, check if the ID follows the folder pattern (e.g., "folders/123456789").
	// TODO: Remove this fallback is for Generating the start URL
	return strings.HasPrefix(tenant.ID, "folders/")
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

		// Warn if targets are present (GCP ignores targets in custom role definitions)
		if len(stmt.Targets) > 0 {
			logrus.Warnf("GCP custom roles do not enforce statement targets; targets are metadata only. Use binding field to control IAM assignment scope. Targets: %v", stmt.Targets)
		}
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
