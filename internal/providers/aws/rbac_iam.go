package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// authorizeRoleTraditionalIAM handles role authorization for traditional IAM users
func (p *awsProvider) authorizeRoleTraditionalIAM(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
	targetAccountID string,
) (*models.AuthorizeRoleResponse, error) {

	user := req.GetUser()
	role := req.GetRole()

	// Check if the role exists
	existingRole, err := p.getRole(ctx, user, role)
	if err != nil {
		// If role doesn't exist, create it
		existingRole, err = p.createRole(ctx, user, role, targetAccountID)
		if err != nil {
			return nil, fmt.Errorf("failed to create role: %w", err)
		}
	}

	// Attach policies to the role using permissions (handles backward compatibility)
	if len(role.Permissions.Allow) > 0 || len(role.Permissions.Deny) > 0 {
		err = p.attachPoliciesToRole(ctx, existingRole.RoleName, role.Permissions)
		if err != nil {
			return nil, fmt.Errorf("failed to attach policies to role: %w", err)
		}
	}

	// Bind the user to the role (assuming user will assume this role)
	err = p.bindUserToRole(ctx, user, existingRole.RoleName, targetAccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to bind user to role: %w", err)
	}

	return nil, nil
}

// revokeRoleTraditionalIAM handles role revocation for traditional IAM users
func (p *awsProvider) revokeRoleTraditionalIAM(ctx context.Context, user *models.User, role *models.Role) (*models.RevokeRoleResponse, error) {

	// Check if the role exists
	existingRole, err := p.getRole(ctx, user, role)
	if err != nil {
		// If role doesn't exist, nothing to revoke
		return nil, fmt.Errorf("role not found: %w", err)
	}

	// Unbind the user from the role by resetting the assume role policy to deny access
	err = p.unbindUserFromRole(ctx, user, existingRole.RoleName)
	if err != nil {
		return nil, fmt.Errorf("failed to unbind user from role: %w", err)
	}

	return nil, nil
}

// getRole retrieves an IAM role by name
func (p *awsProvider) getRole(ctx context.Context, user *models.User, role *models.Role) (*types.Role, error) {
	input := &iam.GetRoleInput{
		RoleName: aws.String(role.GetUniqueIdentifier(user)),
	}

	result, err := p.service.GetRole(ctx, input)
	if err != nil {
		// Return nil role and error if role doesn't exist
		return nil, err
	}
	return result.Role, nil
}

// createRole creates a new IAM role with the specified permissions
func (p *awsProvider) createRole(ctx context.Context, user *models.User, role *models.Role, targetAccountID string) (*types.Role, error) {
	// Create a basic assume role policy document using structs
	// Initially allow the account root to assume the role (will be updated later)
	assumeRolePolicy := PolicyDocument{
		Version: "2012-10-17",
		Statement: []Statement{
			{
				Effect: "Allow",
				Principal: map[string]string{
					"AWS": fmt.Sprintf("arn:aws:iam::%s:root", targetAccountID),
				},
				Action: "sts:AssumeRole",
			},
		},
	}

	assumeRolePolicyJSON, err := json.Marshal(assumeRolePolicy)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal assume role policy: %w", err)
	}

	input := &iam.CreateRoleInput{
		RoleName:                 aws.String(role.GetUniqueIdentifier(user)),
		AssumeRolePolicyDocument: aws.String(string(assumeRolePolicyJSON)),
		Description:              aws.String(role.Description),
	}

	result, err := p.service.CreateRole(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create IAM role: %w", err)
	}

	return result.Role, nil
}

// attachPoliciesToRole creates and attaches an inline policy with the specified permissions
func (p *awsProvider) attachPoliciesToRole(ctx context.Context, roleName *string, permissions models.RolePermissions) error {
	if len(permissions.Allow) == 0 && len(permissions.Deny) == 0 {
		return nil // No permissions to attach
	}

	// Convert CSP-agnostic permissions to AWS policy document
	policyDocument := permissionsToAwsPolicy(permissions)

	// Skip if no valid statements were generated (e.g., all had empty operations)
	if len(policyDocument.Statement) == 0 {
		return nil
	}

	policyDocumentJSON, err := json.Marshal(policyDocument)
	if err != nil {
		return fmt.Errorf("failed to marshal policy document: %w", err)
	}

	// Create an inline policy for the role
	policyName := fmt.Sprintf("thand-%s-policy", common.ConvertToSnakeCase(*roleName))
	input := &iam.PutRolePolicyInput{
		RoleName:       roleName,
		PolicyName:     aws.String(policyName),
		PolicyDocument: aws.String(string(policyDocumentJSON)),
	}

	_, err = p.service.PutRolePolicy(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to attach policy to role: %w", err)
	}

	return nil
}

// bindUserToRole creates or updates the assume role policy to allow the user to assume the role
func (p *awsProvider) bindUserToRole(ctx context.Context, user *models.User, roleName *string, targetAccountID string) error {
	// Create a basic assume role policy that allows the user to assume the role
	var assumeRolePolicy PolicyDocument

	// Determine the username to use for the IAM user ARN
	username := p.getUsernameForIAM(user)

	if len(username) == 0 {
		return fmt.Errorf("failed to determine username for user")
	}

	// Create policy allowing specific user with proper account ID
	assumeRolePolicy = PolicyDocument{
		Version: "2012-10-17",
		Statement: []Statement{
			{
				Effect: "Allow",
				Principal: map[string]string{
					"AWS": fmt.Sprintf("arn:aws:iam::%s:user/%s", targetAccountID, username),
				},
				Action: "sts:AssumeRole",
			},
		},
	}

	assumeRolePolicyJSON, err := json.Marshal(assumeRolePolicy)
	if err != nil {
		return fmt.Errorf("failed to marshal assume role policy: %w", err)
	}

	// Update the role's assume role policy
	updateInput := &iam.UpdateAssumeRolePolicyInput{
		RoleName:       roleName,
		PolicyDocument: aws.String(string(assumeRolePolicyJSON)),
	}

	_, err = p.service.UpdateAssumeRolePolicy(ctx, updateInput)
	if err != nil {
		return fmt.Errorf("failed to update assume role policy: %w", err)
	}

	return nil
}

// unbindUserFromRole removes the user from the assume role policy
func (p *awsProvider) unbindUserFromRole(ctx context.Context, user *models.User, roleName *string) error {
	// Use the cached account ID
	accountID := p.GetAccountID()

	// Get current assume role policy
	roleOutput, err := p.service.GetRole(ctx, &iam.GetRoleInput{
		RoleName: roleName,
	})
	if err != nil {
		return fmt.Errorf("failed to get role %s: %w", *roleName, err)
	}

	// Parse the current policy document
	var currentPolicy PolicyDocument
	if roleOutput.Role.AssumeRolePolicyDocument != nil {
		if err := json.Unmarshal([]byte(*roleOutput.Role.AssumeRolePolicyDocument), &currentPolicy); err != nil {
			return fmt.Errorf("failed to parse assume role policy: %w", err)
		}
	}

	// Extract username from email
	username := p.getUsernameForIAM(user)
	if len(username) == 0 {
		// If no username can be determined, nothing to unbind specifically
		// The role will still have the account root principal
		return fmt.Errorf("failed to determine username for user")
	}
	userArn := fmt.Sprintf("arn:aws:iam::%s:user/%s", accountID, username)

	// Remove statements that reference this user
	var newStatements []Statement
	for _, stmt := range currentPolicy.Statement {
		// Check if this statement references our user
		if principal, ok := stmt.Principal.(map[string]any); ok {
			if awsPrincipal, exists := principal["AWS"]; exists {
				if awsStr, ok := awsPrincipal.(string); ok && awsStr == userArn {
					// Skip this statement - we're removing the user
					continue
				}
			}
		}
		newStatements = append(newStatements, stmt)
	}

	// If no statements remain, create a minimal deny-all policy to prevent open access
	if len(newStatements) == 0 {
		newStatements = []Statement{
			{
				Effect: "Deny",
				Principal: map[string]string{
					"AWS": "*",
				},
				Action: "sts:AssumeRole",
			},
		}
	}

	// Create new policy document
	newPolicy := PolicyDocument{
		Version:   "2012-10-17",
		Statement: newStatements,
	}

	// Update the assume role policy
	newPolicyJSON, err := json.Marshal(newPolicy)
	if err != nil {
		return fmt.Errorf("failed to marshal new policy: %w", err)
	}

	_, err = p.service.UpdateAssumeRolePolicy(ctx, &iam.UpdateAssumeRolePolicyInput{
		RoleName:       roleName,
		PolicyDocument: aws.String(string(newPolicyJSON)),
	})
	if err != nil {
		return fmt.Errorf("failed to update assume role policy for role %s: %w", *roleName, err)
	}

	return nil
}

// getUsernameForIAM determines the appropriate username for AWS IAM user ARN
// Priority: Username field > email prefix > empty string (fallback to account root)
func (p *awsProvider) getUsernameForIAM(user *models.User) string {
	// First priority: use the Username field if available
	if len(user.Username) > 0 {
		return user.Username
	}

	// Second priority: extract from email if available
	if len(user.Email) > 0 {
		return common.ExtractUsernameFromEmail(user.Email)
	}

	// No valid username found - caller should fallback to account root
	return ""
}
