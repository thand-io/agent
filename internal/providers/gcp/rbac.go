package gcp

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/temporal"
	"google.golang.org/api/cloudresourcemanager/v1"
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

// Authorize grants access for a user to a role
func (p *gcpProvider) AuthorizeRole(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
) (*models.AuthorizeRoleResponse, error) {

	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to authorize gcp role")
	}

	user := req.GetUser()
	role := req.GetRole()

	if len(role.Inherits) == 0 && len(role.Permissions.Allow) == 0 {
		return nil, fmt.Errorf("role %s has no inherits or permissions defined", role.Name)
	}

	config := p.GetConfig()
	projectId := p.GetProjectId()
	stage := config.GetStringWithDefault("stage", "GA")

	var assignedRoles []string

	// If inherits is specified, validate and bind predefined GCP roles
	if len(role.Inherits) > 0 {
		for _, inheritedRole := range role.Inherits {
			// Validate that the role is a valid GCP predefined role
			predefinedRole, err := p.GetRole(ctx, inheritedRole)
			if err != nil {
				return nil, fmt.Errorf("invalid GCP role '%s': %w", inheritedRole, err)
			}

			// Bind the user to the predefined role via IAM policy
			err = p.bindUserToPredefinedRole(projectId, user, predefinedRole.Name)
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

	// If permissions are specified, create a custom role with those permissions
	if len(role.Permissions.Allow) > 0 {
		// Check if the custom role already exists
		customRoleName := role.GetSnakeCaseName()
		existingRole, err := p.getRole(projectId, customRoleName)
		if err != nil {
			// If role doesn't exist, create it
			existingRole, err = p.createRole(
				projectId,
				customRoleName,
				role.GetName(),
				role.GetDescription(),
				stage,
				role.Permissions.Allow,
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

			logrus.WithFields(logrus.Fields{
				"role_name":   customRoleName,
				"project_id":  projectId,
				"permissions": role.Permissions.Allow,
			}).Info("Created custom GCP role")
		}

		// Bind the user to the custom role via IAM policy
		err = p.bindUserToRole(projectId, user, existingRole)
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

// Revoke removes access for a user from a role
func (p *gcpProvider) RevokeRole(
	ctx context.Context,
	req *models.RevokeRoleRequest,
) (*models.RevokeRoleResponse, error) {

	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to revoke gcp role")
	}

	user := req.GetUser()
	projectId := p.GetProjectId()

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
			err := p.unbindUserFromPredefinedRole(projectId, user, roleName)
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
			// Extract the role name from the full path (projects/{project}/roles/{roleName})
			parts := strings.Split(roleName, "/")
			// Expected format: projects/{project}/roles/{roleName} (4 parts)
			if len(parts) != 4 || len(parts[len(parts)-1]) == 0 {
				return nil, fmt.Errorf("invalid custom role name format: %q, expected projects/{project}/roles/{roleName}", roleName)
			}
			customRoleName := parts[len(parts)-1]

			existingRole, err := p.getRole(projectId, customRoleName)
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

			err = p.unbindUserFromRole(projectId, user, existingRole)
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
		}
	}

	return &models.RevokeRoleResponse{}, nil
}

func (p *gcpProvider) GetAuthorizedAccessUrl(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
	resp *models.AuthorizeRoleResponse,
) string {

	return p.GetConfig().GetStringWithDefault(
		"sso_start_url", "https://console.cloud.google.com/")
}

// createRole creates a custom role.
func (p *gcpProvider) createRole(projectID, name, title, description, stage string, permissions []string) (*iam.Role, error) {

	service := p.GetIamClient()

	request := &iam.CreateRoleRequest{
		Role: &iam.Role{
			Name:                name,
			Title:               title,
			Description:         description,
			IncludedPermissions: permissions,
			Stage:               stage,
		},
		RoleId: name,
	}
	role, err := service.Projects.Roles.Create("projects/"+projectID, request).Do()
	if err != nil {
		return nil, fmt.Errorf("Projects.Roles.Create: %w", err)
	}
	return role, nil
}

func (p *gcpProvider) getRole(projectID, roleName string) (*iam.Role, error) {
	service := p.GetIamClient()

	role, err := service.Projects.Roles.Get("projects/" + projectID + "/roles/" + roleName).Do()
	if err != nil {
		// Return nil role and error if role doesn't exist
		return nil, err
	}
	return role, nil
}

// bindUserToPredefinedRole binds a user to a predefined GCP role (e.g., roles/viewer)
func (p *gcpProvider) bindUserToPredefinedRole(projectID string, user *models.User, roleName string) error {
	return p.bindUserToRoleByName(projectID, user, roleName)
}

// unbindUserFromPredefinedRole removes a user from a predefined GCP role
func (p *gcpProvider) unbindUserFromPredefinedRole(projectID string, user *models.User, roleName string) error {
	return p.unbindUserFromRoleByName(projectID, user, roleName)
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
	// Check if binding already exists with our thand condition
	for _, binding := range policy.Bindings {
		if binding.Role == roleName && isThandManagedBinding(binding) {
			if slices.Contains(binding.Members, member) {
				return false // Already bound, no modification needed
			}
			// Add member to existing thand-managed binding
			binding.Members = append(binding.Members, member)
			return true
		}
	}

	// No binding exists for this role with our condition, create a new one
	newBinding := &cloudresourcemanager.Binding{
		Role:      roleName,
		Members:   []string{member},
		Condition: newThandCondition(),
	}
	policy.Bindings = append(policy.Bindings, newBinding)
	return true
}

// removeMemberFromPolicy removes a member from a role binding in the policy
// Returns true if the member was found and removed, false otherwise
func removeMemberFromPolicy(policy *cloudresourcemanager.Policy, roleName, member string) bool {
	for i, binding := range policy.Bindings {
		if binding.Role == roleName && isThandManagedBinding(binding) {
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

func (p *gcpProvider) bindUserToRole(projectID string, user *models.User, iamRole *iam.Role) error {
	return p.bindUserToRoleByName(projectID, user, iamRole.Name)
}

func (p *gcpProvider) unbindUserFromRole(projectID string, user *models.User, iamRole *iam.Role) error {
	return p.unbindUserFromRoleByName(projectID, user, iamRole.Name)
}

// bindUserToRoleByName is the core implementation for binding a user to any role
func (p *gcpProvider) bindUserToRoleByName(projectID string, user *models.User, roleName string) error {
	member, err := validateAndFormatMember(user)
	if err != nil {
		return err
	}

	crmService := p.crmClient

	// Get current IAM policy - request version 3 to support conditions
	policy, err := crmService.Projects.GetIamPolicy(projectID, &cloudresourcemanager.GetIamPolicyRequest{
		Options: &cloudresourcemanager.GetPolicyOptions{
			RequestedPolicyVersion: 3,
		},
	}).Do()
	if err != nil {
		return fmt.Errorf("failed to get IAM policy: %w", err)
	}

	// Ensure policy version is 3 for conditions support
	policy.Version = 3

	// Add member to the policy (handles both existing and new bindings)
	if !addMemberToPolicy(policy, roleName, member) {
		// Member already bound, nothing to do
		return nil
	}

	// Set the updated IAM policy
	_, err = crmService.Projects.SetIamPolicy(projectID, &cloudresourcemanager.SetIamPolicyRequest{
		Policy: policy,
	}).Do()
	if err != nil {
		return fmt.Errorf("failed to set IAM policy: %w", err)
	}

	return nil
}

// unbindUserFromRoleByName is the core implementation for unbinding a user from any role
func (p *gcpProvider) unbindUserFromRoleByName(projectID string, user *models.User, roleName string) error {
	member, err := validateAndFormatMember(user)
	if err != nil {
		return err
	}

	crmService := p.crmClient

	// Get current IAM policy - request version 3 to support conditions
	policy, err := crmService.Projects.GetIamPolicy(projectID, &cloudresourcemanager.GetIamPolicyRequest{
		Options: &cloudresourcemanager.GetPolicyOptions{
			RequestedPolicyVersion: 3,
		},
	}).Do()
	if err != nil {
		return fmt.Errorf("failed to get IAM policy: %w", err)
	}

	// Ensure policy version is 3 for conditions support
	policy.Version = 3

	// Remove member from the policy
	if !removeMemberFromPolicy(policy, roleName, member) {
		return fmt.Errorf("thand-managed role binding not found for role %s", roleName)
	}

	// Set the updated IAM policy
	_, err = crmService.Projects.SetIamPolicy(projectID, &cloudresourcemanager.SetIamPolicyRequest{
		Policy: policy,
	}).Do()
	if err != nil {
		return fmt.Errorf("failed to set IAM policy: %w", err)
	}

	return nil
}
