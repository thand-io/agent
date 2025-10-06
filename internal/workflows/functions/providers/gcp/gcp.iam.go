package gcp

import (
	"context"
	"fmt"

	iamadmin "cloud.google.com/go/iam/admin/apiv1"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/option"
	adminpb "google.golang.org/genproto/googleapis/iam/admin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// EnsureRoleAndBindUser checks if a custom role exists in a GCP project, creates it if it doesn't,
// and then binds a specified user to that role.
func EnsureRoleAndBindUser(ctx context.Context, projectID, roleID, userEmail string) error {
	iamClient, err := iamadmin.NewIamClient(ctx, option.WithEndpoint("iam.googleapis.com:443"))
	if err != nil {
		return fmt.Errorf("failed to create IAM client: %w", err)
	}
	defer iamClient.Close()

	roleName := fmt.Sprintf("projects/%s/roles/%s", projectID, roleID)
	_, err = iamClient.GetRole(ctx, &adminpb.GetRoleRequest{Name: roleName})
	if err != nil {
		if isNotFound(err) {
			fmt.Printf("Role %s not found. Creating it...\n", roleID)
			permissions := []string{
				"storage.objects.get",
				"storage.objects.list",
			}
			_, err = createRole(ctx, iamClient, projectID, roleID, "Custom Role Title", "Custom role description", permissions)
			if err != nil {
				return fmt.Errorf("failed to create role: %w", err)
			}
		} else {
			return fmt.Errorf("failed to check role: %w", err)
		}
	} else {
		fmt.Printf("Role %s already exists.\n", roleID)
	}

	crmService, err := cloudresourcemanager.NewService(ctx)
	if err != nil {
		return fmt.Errorf("failed to create Cloud Resource Manager service: %w", err)
	}

	err = bindUserToRole(ctx, crmService, projectID, roleName, userEmail)
	if err != nil {
		return fmt.Errorf("failed to bind user to role: %w", err)
	}

	fmt.Printf("User %s successfully bound to role %s.\n", userEmail, roleName)
	return nil
}

func isNotFound(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	return st.Code() == codes.NotFound
}

func createRole(ctx context.Context, client *iamadmin.IamClient, projectID, roleID, title, description string, permissions []string) (*adminpb.Role, error) {
	role := &adminpb.Role{
		Title:               title,
		Description:         description,
		IncludedPermissions: permissions,
	}

	req := &adminpb.CreateRoleRequest{
		Parent: fmt.Sprintf("projects/%s", projectID),
		RoleId: roleID,
		Role:   role,
	}

	return client.CreateRole(ctx, req)
}

func bindUserToRole(ctx context.Context, crmService *cloudresourcemanager.Service, projectID, roleName, userEmail string) error {
	policy, err := crmService.Projects.GetIamPolicy(projectID, &cloudresourcemanager.GetIamPolicyRequest{}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to get IAM policy for project %s: %w", projectID, err)
	}

	member := fmt.Sprintf("user:%s", userEmail)
	var roleBinding *cloudresourcemanager.Binding
	for _, binding := range policy.Bindings {
		if binding.Role == roleName {
			roleBinding = binding
			break
		}
	}

	if roleBinding != nil {
		for _, m := range roleBinding.Members {
			if m == member {
				return nil // Member already exists in the binding.
			}
		}
		roleBinding.Members = append(roleBinding.Members, member)
	} else {
		policy.Bindings = append(policy.Bindings, &cloudresourcemanager.Binding{
			Role:    roleName,
			Members: []string{member},
		})
	}

	setPolicyRequest := &cloudresourcemanager.SetIamPolicyRequest{
		Policy: policy,
	}
	_, err = crmService.Projects.SetIamPolicy(projectID, setPolicyRequest).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to set IAM policy for project %s: %w", projectID, err)
	}

	return nil
}
