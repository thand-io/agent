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

// CustomAdminRoleResponse represents the response from creating a custom admin role in Okta
type CustomAdminRoleResponse struct {
	ID          string               `json:"id"`
	Label       string               `json:"label"`
	Description string               `json:"description"`
	Created     string               `json:"created"`
	LastUpdated string               `json:"lastUpdated"`
	Links       CustomAdminRoleLinks `json:"_links"`
}

// CustomAdminRoleLinks represents the links in the custom admin role response
type CustomAdminRoleLinks struct {
	Permissions CustomAdminRoleLink `json:"permissions"`
	Self        CustomAdminRoleLink `json:"self"`
}

// CustomAdminRoleLink represents a single link in the custom admin role response
type CustomAdminRoleLink struct {
	Href string `json:"href"`
}

// ResourceSetAssignment represents a principal assignment to a resource set
type ResourceSetAssignment struct {
	PrincipalID     string `json:"principalId"`
	PrincipalType   string `json:"principalType"`
	PermissionSetID string `json:"permissionSetId"`
	ResourceSetID   string `json:"resourceSetId"`
}

// ResourceSetAssignmentRequest represents the request body for resource set assignments
type ResourceSetAssignmentRequest struct {
	Add    []ResourceSetAssignment `json:"add,omitempty"`
	Remove []ResourceSetAssignment `json:"remove,omitempty"`
}

// AuthorizeRole assigns a role to a user in Okta
func (p *oktaProvider) AuthorizeRole(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
) (*models.AuthorizeRoleResponse, error) {
	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to authorize Okta role")
	}

	user := req.GetUser()
	role := req.GetRole()

	if len(role.Inherits) == 0 &&
		len(role.Groups.Allow) == 0 &&
		len(role.Permissions.Allow) == 0 &&
		len(role.Resources.Allow) == 0 {
		return nil, fmt.Errorf("role %s has no inherits, groups, permissions, or resources defined", role.Name)
	}

	// Get the Okta user
	oktaUser, _, err := p.client.User.GetUser(ctx, user.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to find user in Okta: %w", err)
	}

	// Determine which Okta roles to assign
	var assignedRoles []string
	var assignedGroups []string
	var assignedResources []string

	// Check if there are groups to assign
	if len(role.Groups.Allow) > 0 {

		for _, groupId := range role.Groups.Allow {

			// Get the okta groups and add the user to each
			identity, err := p.GetIdentity(ctx, groupId)

			if err != nil {
				return nil, fmt.Errorf("failed to get group %s: %w", groupId, err)
			}

			if identity.GetGroup() == nil {
				return nil, fmt.Errorf("group %s not found in Okta", groupId)
			}

			err = p.AddUserToGroup(ctx, identity.GetGroup().ID, oktaUser.Id)

			if err != nil {
				return nil, fmt.Errorf("failed to add user to group %s: %w", groupId, err)
			}

			logrus.WithFields(logrus.Fields{
				"user_id":    oktaUser.Id,
				"user_email": user.Email,
				"group_id":   groupId,
			}).Info("Successfully added user to group in Okta")

			assignedGroups = append(assignedGroups, groupId)
		}
	}

	if len(role.Inherits) > 0 {
		// If the role inherits from other roles, assign those roles

		for _, roleType := range role.Inherits {
			// Prepare role assignment request
			roleAssignment := okta.AssignRoleRequest{
				Type: roleType,
			}

			// Assign the role to the user
			assignedRole, _, err := p.client.User.AssignRoleToUser(ctx, oktaUser.Id, roleAssignment, nil)
			if err != nil {
				if oktaErr, ok := err.(*okta.Error); ok {
					if strings.ToUpper(oktaErr.ErrorCode) == "E0000090" {

						logrus.WithFields(logrus.Fields{
							"user_id":    oktaUser.Id,
							"user_email": user.Email,
							"role_type":  roleType,
						}).Info("User already has the role assigned in Okta, skipping assignment")

						// If the role has already been assigned, just skip
						// we don't need to mark it for removal later. As this might
						// be a standing permission.

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

			assignedRoles = append(assignedRoles, assignedRole.Id)

			logrus.WithFields(logrus.Fields{
				"user_id":       oktaUser.Id,
				"user_email":    user.Email,
				"role_type":     roleType,
				"assignment_id": assignedRole.Id,
			}).Info("Successfully assigned role to user in Okta")
		}

	}

	if len(role.Permissions.Allow) > 0 {

		// Create a custom admin role with the specified permissions
		customRoleType, err := p.createCustomAdminRole(ctx, role)
		if err != nil {
			return nil, fmt.Errorf("failed to create custom admin role: %w", err)
		}

		logrus.WithFields(logrus.Fields{
			"role_name":        role.Name,
			"custom_role_type": customRoleType,
			"permissions":      role.Permissions.Allow,
		}).Info("Created custom admin role in Okta")

		// Assign the role to the user
		err = p.assignCustomRoleToUser(ctx, ResourceSetAssignmentRequest{
			Add: []ResourceSetAssignment{
				{
					PrincipalID:     oktaUser.Id,
					PrincipalType:   "USER",
					PermissionSetID: customRoleType.ID,
					ResourceSetID:   "", // Assuming empty for now; adjust as needed
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

		assignedRoles = append(assignedRoles, customRoleType.ID)

	}

	if len(role.Resources.Allow) > 0 {
		// Check for resources starting with "application:" prefix
		for _, resource := range role.Resources.Allow {
			if after, ok := strings.CutPrefix(resource, "application:"); ok {
				// Extract the application ID or name by removing the prefix
				appIdentifier := after

				// Get the application resource
				appResource, err := p.GetResource(ctx, appIdentifier)
				if err != nil {
					return nil, fmt.Errorf("failed to get application %s: %w", appIdentifier, err)
				}

				if appResource == nil || appResource.Type != "application" {
					return nil, fmt.Errorf("resource %s is not an application", appIdentifier)
				}

				// Assign the user to the application
				appUser := okta.AppUser{
					Id: oktaUser.Id,
				}

				_, _, err = p.client.Application.AssignUserToApplication(ctx, appResource.ID, appUser)

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
					"user_id":    oktaUser.Id,
					"user_email": user.Email,
					"app_id":     appResource.ID,
					"app_name":   appResource.Name,
				}).Info("Successfully assigned user to application in Okta")

				assignedResources = append(assignedResources, fmt.Sprintf("application:%s", appResource.ID))
			} else {
				logrus.WithFields(logrus.Fields{
					"role_name": role.Name,
					"resource":  resource,
				}).Warn("Resource does not match expected 'application:' prefix and will be ignored")
			}
		}
	}

	return &models.AuthorizeRoleResponse{
		UserId:    oktaUser.Id,
		Roles:     assignedRoles,
		Groups:    assignedGroups,
		Resources: assignedResources,
	}, nil
}

// RevokeRole removes a role from a user in Okta
func (p *oktaProvider) RevokeRole(
	ctx context.Context,
	req *models.RevokeRoleRequest,
) (*models.RevokeRoleResponse, error) {

	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to revoke Okta role")
	}

	user := req.GetUser()

	// Get the Okta user
	oktaUser, _, err := p.client.User.GetUser(ctx, user.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to find user in Okta: %w", err)
	}

	if req.AuthorizeRoleResponse == nil {
		return nil, fmt.Errorf("no authorize role response found for revocation")
	}

	// Convert metadata to strongly typed structure
	metadata := req.AuthorizeRoleResponse

	// Revoke roles
	if len(metadata.Roles) > 0 {
		if err := p.revokeRoles(ctx, metadata.Roles, oktaUser.Id, user.Email); err != nil {
			return nil, temporal.NewApplicationErrorWithOptions(
				"Failed to revoke roles from user",
				"OktaRolesRevokationError",
				temporal.ApplicationErrorOptions{
					NextRetryDelay: 3 * time.Second,
					Cause:          err,
				},
			)
		}
	}

	// Revoke groups
	if len(metadata.Groups) > 0 {
		if err := p.revokeGroups(ctx, metadata.Groups, oktaUser.Id, user.Email); err != nil {
			return nil, temporal.NewApplicationErrorWithOptions(
				"Failed to revoke groups from user",
				"OktaGroupsRevocationError",
				temporal.ApplicationErrorOptions{
					NextRetryDelay: 3 * time.Second,
					Cause:          err,
				},
			)
		}
	}

	// Revoke applications
	if len(metadata.Resources) > 0 {
		if err := p.revokeResources(ctx, metadata.Resources, oktaUser.Id, user.Email); err != nil {
			return nil, temporal.NewApplicationErrorWithOptions(
				"Failed to revoke resources from user",
				"OktaResourcesRevocationError",
				temporal.ApplicationErrorOptions{
					NextRetryDelay: 3 * time.Second,
					Cause:          err,
				},
			)
		}
	}

	return &models.RevokeRoleResponse{}, nil
}

// revokeRoles revokes roles from user
func (p *oktaProvider) revokeRoles(ctx context.Context, roleIds []string, userId string, userEmail string) error {
	for _, roleId := range roleIds {

		// This is a standard role, remove via Assignments API
		_, err := p.client.User.RemoveRoleFromUser(ctx, userId, roleId)

		if err != nil {
			return fmt.Errorf("failed to revoke role %s from user: %w", roleId, err)
		}

		logrus.WithFields(logrus.Fields{
			"user_id":    userId,
			"user_email": userEmail,
			"role_id":    roleId,
		}).Info("Successfully revoked role from user in Okta")

	}

	return nil
}

// revokeGroups removes user from groups
func (p *oktaProvider) revokeGroups(ctx context.Context, groupIds []string, userId string, userEmail string) error {
	for _, groupId := range groupIds {

		// Get the okta groups and remove the user from each
		identity, err := p.GetIdentity(ctx, groupId)

		if err != nil {
			return fmt.Errorf("failed to get group %s: %w", groupId, err)
		}

		if identity.GetGroup() == nil {
			return fmt.Errorf("group %s not found in Okta", groupId)
		}

		err = p.RemoveUserFromGroup(ctx, identity.ID, userId)

		if err != nil {
			return fmt.Errorf("failed to remove user from group %s: %w", groupId, err)
		}

		logrus.WithFields(logrus.Fields{
			"user_id":    userId,
			"user_email": userEmail,
			"group_id":   groupId,
		}).Info("Successfully removed user from group in Okta")
	}

	return nil
}

// revokeResources removes user from resources
func (p *oktaProvider) revokeResources(ctx context.Context, resourceIds []string, userId string, userEmail string) error {
	for _, resourceId := range resourceIds {

		if after, ok := strings.CutPrefix(resourceId, "application:"); ok {

			_, err := p.client.Application.DeleteApplicationUser(ctx, after, userId, nil)

			if err != nil {
				return fmt.Errorf("failed to remove user from resource %s: %w", resourceId, err)
			}

			logrus.WithFields(logrus.Fields{
				"user_id":     userId,
				"user_email":  userEmail,
				"resource_id": resourceId,
			}).Info("Successfully removed user from resource in Okta")

		} else {
			logrus.WithFields(logrus.Fields{
				"user_id":     userId,
				"user_email":  userEmail,
				"resource_id": resourceId,
			}).Warn("Resource does not match expected 'application:' prefix and will be ignored")
		}
	}

	return nil
}

// GetAuthorizedAccessUrl returns the URL where the user can access their Okta dashboard
func (p *oktaProvider) GetAuthorizedAccessUrl(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
	resp *models.AuthorizeRoleResponse,
) string {
	// Return the Okta organization URL where users can log in
	return p.orgUrl
}

// Helper function to list groups in Okta
func (p *oktaProvider) ListGroups(ctx context.Context) ([]*okta.Group, error) {
	groups, _, err := p.client.Group.ListGroups(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list groups: %w", err)
	}
	return groups, nil
}

// Helper function to get a specific group
func (p *oktaProvider) GetGroup(ctx context.Context, groupId string) (*okta.Group, error) {
	group, _, err := p.client.Group.GetGroup(ctx, groupId)
	if err != nil {
		return nil, fmt.Errorf("failed to get group: %w", err)
	}
	return group, nil
}

// Helper function to list users in Okta
func (p *oktaProvider) ListUsers(ctx context.Context) ([]*okta.User, error) {
	users, _, err := p.client.User.ListUsers(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	return users, nil
}

// Helper function to get a specific user
func (p *oktaProvider) GetUser(ctx context.Context, userId string) (*okta.User, error) {
	user, _, err := p.client.User.GetUser(ctx, userId)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return user, nil
}

// Helper function to list applications in Okta
func (p *oktaProvider) ListApplications(ctx context.Context) ([]okta.App, error) {
	apps, _, err := p.client.Application.ListApplications(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list applications: %w", err)
	}
	return apps, nil
}

// Helper function to add user to a group
func (p *oktaProvider) AddUserToGroup(ctx context.Context, groupId string, userId string) error {
	_, err := p.client.Group.AddUserToGroup(ctx, groupId, userId)
	if err != nil {
		return fmt.Errorf("failed to add user to group: %w", err)
	}
	return nil
}

// Helper function to remove user from a group
func (p *oktaProvider) RemoveUserFromGroup(ctx context.Context, groupId string, userId string) error {
	_, err := p.client.Group.RemoveUserFromGroup(ctx, groupId, userId)
	if err != nil {
		return fmt.Errorf("failed to remove user from group: %w", err)
	}
	return nil
}
