package gcp

import (
	"context"
	"fmt"
	"slices"

	"github.com/thand-io/agent/internal/models"
	"google.golang.org/api/cloudresourcemanager/v1"
	iam "google.golang.org/api/iam/v1"
)

// Authorize grants access for a user to a role
func (p *gcpProvider) AuthorizeRole(ctx context.Context, user *models.User, role *models.Role) (map[string]any, error) {

	if user == nil || role == nil {
		return nil, fmt.Errorf("user and role must be provided to authorize gcp role")
	}

	config := p.GetConfig()
	projectId := p.GetProjectId()

	stage := config.GetStringWithDefault("stage", "GA")

	// Check if the role exists
	existingRole, err := p.getRole(projectId, role.GetSnakeCaseName())
	if err != nil {
		// If role doesn't exist, create it
		existingRole, err = p.createRole(
			projectId,
			role.GetSnakeCaseName(),
			role.GetName(),
			role.GetDescription(),
			stage,
			role.Permissions.Allow,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create role: %w", err)
		}
	}

	// Bind the user to the role via IAM policy
	err = p.bindUserToRole(projectId, user, existingRole)
	if err != nil {
		return nil, fmt.Errorf("failed to bind user to role: %w", err)
	}

	return nil, nil
}

// Revoke removes access for a user from a role
func (p *gcpProvider) RevokeRole(
	ctx context.Context,
	user *models.User,
	role *models.Role,
	metadata map[string]any,
) (map[string]any, error) {

	if user == nil || role == nil {
		return nil, fmt.Errorf("user and role must be provided to revoke gcp role")
	}

	projectId := p.GetProjectId()

	// Check if the role exists
	existingRole, err := p.getRole(projectId, role.GetSnakeCaseName())
	if err != nil {
		return nil, fmt.Errorf("failed to get role: %w", err)
	}

	// Remove the user from the role via IAM policy
	err = p.unbindUserFromRole(projectId, user, existingRole)
	if err != nil {
		return nil, fmt.Errorf("failed to unbind user from role: %w", err)
	}

	return nil, nil
}

// createRole creates a custom role.
func (p *gcpProvider) createRole(projectID, name, title, description, stage string, permissions []string) (*iam.Role, error) {

	service := p.GetIamClient()

	request := &iam.CreateRoleRequest{
		Role: &iam.Role{
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

// findRoleBinding finds a binding for the specified role in the IAM policy
// Returns the binding index and the binding itself, or -1 and nil if not found
func (p *gcpProvider) findRoleBinding(policy *cloudresourcemanager.Policy, roleName string) (int, *cloudresourcemanager.Binding) {
	for i, binding := range policy.Bindings {
		if binding.Role == roleName {
			return i, binding
		}
	}
	return -1, nil
}

func (p *gcpProvider) bindUserToRole(projectID string, user *models.User, iamRole *iam.Role) error {
	crmService := p.crmClient

	// Get current IAM policy
	policy, err := crmService.Projects.GetIamPolicy(projectID, &cloudresourcemanager.GetIamPolicyRequest{}).Do()
	if err != nil {
		return fmt.Errorf("failed to get IAM policy: %w", err)
	}

	// Create member string based on user type
	var member string
	if len(user.Email) > 0 {
		member = "user:" + user.Email
	} else {
		return fmt.Errorf("user email is required for GCP IAM binding")
	}

	// Check if binding already exists
	_, binding := p.findRoleBinding(policy, iamRole.Name)
	if binding != nil {
		// Binding exists, check if member is already in it
		if !slices.Contains(binding.Members, member) {
			// Add member to existing binding
			binding.Members = append(binding.Members, member)
		}
	} else {
		// If no binding exists for this role, create a new one
		newBinding := &cloudresourcemanager.Binding{
			Role:    iamRole.Name,
			Members: []string{member},
		}
		policy.Bindings = append(policy.Bindings, newBinding)
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

func (p *gcpProvider) unbindUserFromRole(projectID string, user *models.User, iamRole *iam.Role) error {
	crmService := p.crmClient

	// Get current IAM policy
	policy, err := crmService.Projects.GetIamPolicy(projectID, &cloudresourcemanager.GetIamPolicyRequest{}).Do()
	if err != nil {
		return fmt.Errorf("failed to get IAM policy: %w", err)
	}

	// Create member string based on user type
	var member string
	if len(user.Email) > 0 {
		member = "user:" + user.Email
	} else {
		return fmt.Errorf("user email is required for GCP IAM binding")
	}

	// Find and remove the user from the role binding
	bindingIndex, binding := p.findRoleBinding(policy, iamRole.Name)
	if binding == nil {
		return fmt.Errorf("role binding not found for role %s", iamRole.Name)
	}

	// Find and remove the member from this binding
	for j, bindingMember := range binding.Members {
		if bindingMember == member {
			// Remove the member from the slice
			binding.Members = append(binding.Members[:j], binding.Members[j+1:]...)
			break
		}
	}

	// If the binding has no members left, remove the entire binding
	if len(binding.Members) == 0 {
		policy.Bindings = append(policy.Bindings[:bindingIndex], policy.Bindings[bindingIndex+1:]...)
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
