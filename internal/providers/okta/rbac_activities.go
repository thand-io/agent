package okta

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/okta/okta-sdk-golang/v2/okta"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/temporal"
)

// oktaProviderActivities exposes Okta RBAC operations as individual Temporal activities.
type oktaProviderActivities struct {
	provider *oktaProvider
}

// ─────────────────────────────────────────────────────────────────────────────
// Request / response types
// ─────────────────────────────────────────────────────────────────────────────

type FindOktaUserRequest struct {
	UserEmail string `json:"user_email"`
}

type FindOktaUserResponse struct {
	OktaUserID string `json:"okta_user_id"`
}

type OktaAddGroupTargetsRequest struct {
	OktaUserID string       `json:"okta_user_id"`
	UserEmail  string       `json:"user_email"`
	Role       *models.Role `json:"role"`
}

type OktaAddGroupTargetsResponse struct {
	AssignedGroupIDs []string `json:"assigned_group_ids"`
}

type OktaAssignInheritedRolesRequest struct {
	OktaUserID string   `json:"okta_user_id"`
	UserEmail  string   `json:"user_email"`
	Inherits   []string `json:"inherits"`
}

type OktaAssignInheritedRolesResponse struct {
	AssignedRoleIDs []string `json:"assigned_role_ids"`
}

type OktaCreateAndAssignCustomRoleRequest struct {
	OktaUserID string       `json:"okta_user_id"`
	Role       *models.Role `json:"role"`
}

type OktaCreateAndAssignCustomRoleResponse struct {
	RoleID string `json:"role_id"`
}

type OktaAssignApplicationTargetsRequest struct {
	OktaUserID string       `json:"okta_user_id"`
	UserEmail  string       `json:"user_email"`
	Role       *models.Role `json:"role"`
}

type OktaAssignApplicationTargetsResponse struct {
	AssignedResourceIDs []string `json:"assigned_resource_ids"`
}

type OktaRevokeRolesRequest struct {
	OktaUserID string   `json:"okta_user_id"`
	UserEmail  string   `json:"user_email"`
	RoleIDs    []string `json:"role_ids"`
}

type OktaRevokeGroupsRequest struct {
	OktaUserID string   `json:"okta_user_id"`
	UserEmail  string   `json:"user_email"`
	GroupIDs   []string `json:"group_ids"`
}

type OktaRevokeResourcesRequest struct {
	OktaUserID  string   `json:"okta_user_id"`
	UserEmail   string   `json:"user_email"`
	ResourceIDs []string `json:"resource_ids"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Activity implementations — authorize
// ─────────────────────────────────────────────────────────────────────────────

// FindOktaUser looks up the Okta user ID by email.
func (a *oktaProviderActivities) FindOktaUser(
	ctx context.Context,
	req *FindOktaUserRequest,
) (*FindOktaUserResponse, error) {
	user, _, err := a.provider.client.User.GetUser(ctx, req.UserEmail)
	if err != nil {
		return nil, fmt.Errorf("failed to find user in Okta: %w", err)
	}
	return &FindOktaUserResponse{OktaUserID: user.Id}, nil
}

// OktaAddGroupTargets adds the user to all group targets specified in the role's permissions.
func (a *oktaProviderActivities) OktaAddGroupTargets(
	ctx context.Context,
	req *OktaAddGroupTargetsRequest,
) (*OktaAddGroupTargetsResponse, error) {
	var assignedGroupIDs []string
	for _, stmt := range req.Role.Permissions.Allow {
		for _, target := range stmt.Targets {
			if after, ok := strings.CutPrefix(target, "group:"); ok {
				identity, err := a.provider.GetIdentity(ctx, after)
				if err != nil {
					return nil, fmt.Errorf("failed to get group %s: %w", after, err)
				}
				if identity.GetGroup() == nil {
					return nil, fmt.Errorf("group %s not found in Okta", after)
				}
				if err := a.provider.AddUserToGroup(ctx, identity.GetGroup().ID, req.OktaUserID); err != nil {
					return nil, fmt.Errorf("failed to add user to group %s: %w", after, err)
				}
				logrus.WithFields(logrus.Fields{
					"user_id":    req.OktaUserID,
					"user_email": req.UserEmail,
					"group_id":   after,
				}).Info("Added user to group in Okta")
				assignedGroupIDs = append(assignedGroupIDs, after)
			}
		}
	}
	return &OktaAddGroupTargetsResponse{AssignedGroupIDs: assignedGroupIDs}, nil
}

// OktaAssignInheritedRoles assigns all inherited (standard) Okta roles to the user.
func (a *oktaProviderActivities) OktaAssignInheritedRoles(
	ctx context.Context,
	req *OktaAssignInheritedRolesRequest,
) (*OktaAssignInheritedRolesResponse, error) {
	var assignedRoleIDs []string
	for _, roleType := range req.Inherits {
		roleAssignment := okta.AssignRoleRequest{Type: roleType}
		assigned, _, err := a.provider.client.User.AssignRoleToUser(ctx, req.OktaUserID, roleAssignment, nil)
		if err != nil {
			if oktaErr, ok := err.(*okta.Error); ok {
				if strings.ToUpper(oktaErr.ErrorCode) == "E0000090" {
					logrus.WithFields(logrus.Fields{
						"user_id":    req.OktaUserID,
						"user_email": req.UserEmail,
						"role_type":  roleType,
					}).Info("User already has the role assigned in Okta, skipping")
					continue
				}
			}
			return nil, temporal.NewApplicationErrorWithOptions(
				fmt.Sprintf("failed to assign role %s to user: %v", roleType, err),
				"OktaRoleAssignmentError",
				temporal.ApplicationErrorOptions{
					NextRetryDelay: 3 * time.Second,
					Cause:          err,
				},
			)
		}
		assignedRoleIDs = append(assignedRoleIDs, assigned.Id)
		logrus.WithFields(logrus.Fields{
			"user_id":   req.OktaUserID,
			"role_type": roleType,
		}).Info("Assigned role to user in Okta")
	}
	return &OktaAssignInheritedRolesResponse{AssignedRoleIDs: assignedRoleIDs}, nil
}

// OktaCreateAndAssignCustomRole creates a custom admin role from the role's permissions
// and assigns it to the user.
func (a *oktaProviderActivities) OktaCreateAndAssignCustomRole(
	ctx context.Context,
	req *OktaCreateAndAssignCustomRoleRequest,
) (*OktaCreateAndAssignCustomRoleResponse, error) {
	customRoleType, err := a.provider.createCustomAdminRole(ctx, req.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to create custom admin role: %w", err)
	}

	err = a.provider.assignCustomRoleToUser(ctx, ResourceSetAssignmentRequest{
		Add: []ResourceSetAssignment{
			{
				PrincipalID:     req.OktaUserID,
				PrincipalType:   "USER",
				PermissionSetID: customRoleType.ID,
				ResourceSetID:   "",
			},
		},
	})
	if err != nil {
		return nil, temporal.NewApplicationErrorWithOptions(
			fmt.Sprintf("failed to assign custom role to user: %v", err),
			"OktaCustomRoleAssignmentError",
			temporal.ApplicationErrorOptions{
				NextRetryDelay: 3 * time.Second,
				Cause:          err,
			},
		)
	}
	return &OktaCreateAndAssignCustomRoleResponse{RoleID: customRoleType.ID}, nil
}

// OktaAssignApplicationTargets assigns the user to all application targets in the role.
func (a *oktaProviderActivities) OktaAssignApplicationTargets(
	ctx context.Context,
	req *OktaAssignApplicationTargetsRequest,
) (*OktaAssignApplicationTargetsResponse, error) {
	var assignedResourceIDs []string
	for _, stmt := range req.Role.Permissions.Allow {
		for _, target := range stmt.Targets {
			if after, ok := strings.CutPrefix(target, "application:"); ok {
				appResource, err := a.provider.GetResource(ctx, after)
				if err != nil {
					return nil, fmt.Errorf("failed to get application %s: %w", after, err)
				}
				if appResource == nil || appResource.Type != "application" {
					return nil, fmt.Errorf("resource %s is not an application", after)
				}
				_, _, err = a.provider.client.Application.AssignUserToApplication(ctx, appResource.ID, okta.AppUser{Id: req.OktaUserID})
				if err != nil {
					return nil, temporal.NewApplicationErrorWithOptions(
						fmt.Sprintf("failed to assign user to application %s: %v", appResource.Name, err),
						"OktaApplicationAssignmentError",
						temporal.ApplicationErrorOptions{
							NextRetryDelay: 3 * time.Second,
							Cause:          err,
						},
					)
				}
				logrus.WithFields(logrus.Fields{
					"user_id":  req.OktaUserID,
					"app_id":   appResource.ID,
					"app_name": appResource.Name,
				}).Info("Assigned user to application in Okta")
				assignedResourceIDs = append(assignedResourceIDs, fmt.Sprintf("application:%s", appResource.ID))
			}
		}
	}
	return &OktaAssignApplicationTargetsResponse{AssignedResourceIDs: assignedResourceIDs}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Activity implementations — revoke
// ─────────────────────────────────────────────────────────────────────────────

// OktaRevokeRoles removes all specified roles from the user.
func (a *oktaProviderActivities) OktaRevokeRoles(
	ctx context.Context,
	req *OktaRevokeRolesRequest,
) error {
	return a.provider.revokeRoles(ctx, req.RoleIDs, req.OktaUserID, req.UserEmail)
}

// OktaRevokeGroups removes the user from all specified groups.
func (a *oktaProviderActivities) OktaRevokeGroups(
	ctx context.Context,
	req *OktaRevokeGroupsRequest,
) error {
	return a.provider.revokeGroups(ctx, req.GroupIDs, req.OktaUserID, req.UserEmail)
}

// OktaRevokeResources removes the user from all specified application resources.
func (a *oktaProviderActivities) OktaRevokeResources(
	ctx context.Context,
	req *OktaRevokeResourcesRequest,
) error {
	return a.provider.revokeResources(ctx, req.ResourceIDs, req.OktaUserID, req.UserEmail)
}
