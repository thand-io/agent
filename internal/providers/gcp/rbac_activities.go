package gcp

import (
	"context"
	"fmt"
	"time"

	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/temporal"
)

// gcpProviderActivities exposes granular GCP RBAC operations as individual
// Temporal activities.
type gcpProviderActivities struct {
	provider *gcpProvider
}

// ─────────────────────────────────────────────────────────────────────────────
// Request / response types
// ─────────────────────────────────────────────────────────────────────────────

type BindUserToPredefinedRoleRequest struct {
	ProjectID     string       `json:"project_id"`
	User          *models.User `json:"user"`
	InheritedRole string       `json:"inherited_role"`
}

type BindUserToPredefinedRoleResponse struct {
	RoleName string `json:"role_name"`
}

type GetOrCreateAndBindCustomRoleRequest struct {
	ProjectID   string                 `json:"project_id"`
	User        *models.User           `json:"user"`
	RoleName    string                 `json:"role_name"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Stage       string                 `json:"stage"`
	Permissions models.RolePermissions `json:"permissions"`
}

type GetOrCreateAndBindCustomRoleResponse struct {
	RoleName string `json:"role_name"`
}

type UnbindUserFromPredefinedRoleRequest struct {
	ProjectID string       `json:"project_id"`
	User      *models.User `json:"user"`
	RoleName  string       `json:"role_name"`
}

type UnbindAndDeleteCustomRoleRequest struct {
	ProjectID string       `json:"project_id"`
	User      *models.User `json:"user"`
	RoleName  string       `json:"role_name"` // full path e.g. projects/{p}/roles/{name}
}

// ─────────────────────────────────────────────────────────────────────────────
// Activity implementations
// ─────────────────────────────────────────────────────────────────────────────

// BindUserToPredefinedRole validates a predefined GCP role exists and binds the user to it.
func (a *gcpProviderActivities) BindUserToPredefinedRole(
	ctx context.Context,
	req *BindUserToPredefinedRoleRequest,
) (*BindUserToPredefinedRoleResponse, error) {
	providerRole, err := a.provider.GetRole(ctx, req.InheritedRole)
	if err != nil {
		return nil, fmt.Errorf("invalid GCP role '%s': %w", req.InheritedRole, err)
	}

	err = a.provider.bindUserToPredefinedRole(ctx, req.ProjectID, req.User, providerRole.Name)
	if err != nil {
		return nil, temporal.NewApplicationErrorWithOptions(
			fmt.Sprintf("failed to bind user to role %s: %v", providerRole.Name, err),
			"GcpRoleBindingError",
			temporal.ApplicationErrorOptions{
				NextRetryDelay: 3 * time.Second,
				Cause:          err,
			},
		)
	}
	return &BindUserToPredefinedRoleResponse{RoleName: providerRole.Name}, nil
}

// GetOrCreateAndBindCustomRole creates (or fetches) a custom GCP role and binds the user.
func (a *gcpProviderActivities) GetOrCreateAndBindCustomRole(
	ctx context.Context,
	req *GetOrCreateAndBindCustomRoleRequest,
) (*GetOrCreateAndBindCustomRoleResponse, error) {
	existingRole, err := a.provider.getRole(ctx, req.ProjectID, req.RoleName)
	if err != nil {
		existingRole, err = a.provider.createRole(
			ctx,
			req.ProjectID,
			req.RoleName,
			req.Title,
			req.Description,
			req.Stage,
			req.Permissions,
		)
		if err != nil {
			return nil, temporal.NewApplicationErrorWithOptions(
				fmt.Sprintf("failed to create custom role %s: %v", req.RoleName, err),
				"GcpCustomRoleCreationError",
				temporal.ApplicationErrorOptions{
					NextRetryDelay: 3 * time.Second,
					Cause:          err,
				},
			)
		}
	} else {
		// Role already exists – patch permissions if the desired set has changed
		existingRole, err = a.provider.patchRoleIfStale(ctx, req.ProjectID, existingRole, req.Permissions)
		if err != nil {
			return nil, temporal.NewApplicationErrorWithOptions(
				fmt.Sprintf("failed to update custom role %s: %v", req.RoleName, err),
				"GcpCustomRoleUpdateError",
				temporal.ApplicationErrorOptions{
					NextRetryDelay: 3 * time.Second,
					Cause:          err,
				},
			)
		}
	}

	err = a.provider.bindUserToRole(ctx, req.ProjectID, req.User, existingRole)
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
	return &GetOrCreateAndBindCustomRoleResponse{RoleName: existingRole.Name}, nil
}

// UnbindUserFromPredefinedRole removes a user's IAM binding for a predefined GCP role.
func (a *gcpProviderActivities) UnbindUserFromPredefinedRole(
	ctx context.Context,
	req *UnbindUserFromPredefinedRoleRequest,
) error {
	err := a.provider.unbindUserFromPredefinedRole(ctx, req.ProjectID, req.User, req.RoleName)
	if err != nil {
		return temporal.NewApplicationErrorWithOptions(
			fmt.Sprintf("failed to unbind user from predefined role %s: %v", req.RoleName, err),
			"GcpRoleUnbindingError",
			temporal.ApplicationErrorOptions{
				NextRetryDelay: 3 * time.Second,
				Cause:          err,
			},
		)
	}
	return nil
}

// UnbindAndDeleteCustomRole unbinds the user from a custom GCP role and then deletes it.
func (a *gcpProviderActivities) UnbindAndDeleteCustomRole(
	ctx context.Context,
	req *UnbindAndDeleteCustomRoleRequest,
) error {
	// Extract short name from full path (projects/{project}/roles/{name})
	customRoleName, err := parseCustomRolePath(req.RoleName)
	if err != nil {
		return err
	}

	existingRole, err := a.provider.getRole(ctx, req.ProjectID, customRoleName)
	if err != nil {
		return temporal.NewApplicationErrorWithOptions(
			fmt.Sprintf("failed to get custom role %s: %v", customRoleName, err),
			"GcpGetRoleError",
			temporal.ApplicationErrorOptions{
				NextRetryDelay: 3 * time.Second,
				Cause:          err,
			},
		)
	}

	if err := a.provider.unbindUserFromRole(ctx, req.ProjectID, req.User, existingRole); err != nil {
		return temporal.NewApplicationErrorWithOptions(
			fmt.Sprintf("failed to unbind user from custom role %s: %v", req.RoleName, err),
			"GcpCustomRoleUnbindingError",
			temporal.ApplicationErrorOptions{
				NextRetryDelay: 3 * time.Second,
				Cause:          err,
			},
		)
	}

	if err := a.provider.deleteRole(ctx, req.ProjectID, customRoleName); err != nil {
		return temporal.NewApplicationErrorWithOptions(
			fmt.Sprintf("failed to delete custom role %s: %v", customRoleName, err),
			"GcpCustomRoleDeletionError",
			temporal.ApplicationErrorOptions{
				NextRetryDelay: 3 * time.Second,
				Cause:          err,
			},
		)
	}
	return nil
}
