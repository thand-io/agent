package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// Authorize grants access for a user to a role
func (p *awsProvider) AuthorizeRole(ctx context.Context, user *models.User, role *models.Role) (map[string]any, error) {
	// Check if the role exists
	existingRole, err := p.getRole(ctx, role)
	if err != nil {
		// If role doesn't exist, create it
		existingRole, err = p.createRole(ctx, role)
		if err != nil {
			return nil, fmt.Errorf("failed to create role: %w", err)
		}
	}

	// Attach policies to the role if they don't exist
	err = p.attachPoliciesToRole(ctx, existingRole.RoleName, role.Permissions.Allow)
	if err != nil {
		return nil, fmt.Errorf("failed to attach policies to role: %w", err)
	}

	// Bind the user to the role (assuming user will assume this role)
	err = p.bindUserToRole(ctx, user, existingRole.RoleName)
	if err != nil {
		return nil, fmt.Errorf("failed to bind user to role: %w", err)
	}

	return nil, nil
}

// Revoke removes access for a user from a role
func (p *awsProvider) RevokeRole(
	ctx context.Context, user *models.User, role *models.Role) (map[string]any, error) {
	// Ok remove our role by name

	return nil, nil
}

// getRole retrieves an IAM role by name
func (p *awsProvider) getRole(ctx context.Context, role *models.Role) (*types.Role, error) {
	input := &iam.GetRoleInput{
		RoleName: aws.String(role.GetSnakeCaseName()),
	}

	result, err := p.service.GetRole(ctx, input)
	if err != nil {
		// Return nil role and error if role doesn't exist
		return nil, err
	}
	return result.Role, nil
}

// createRole creates a new IAM role with the specified permissions
func (p *awsProvider) createRole(ctx context.Context, role *models.Role) (*types.Role, error) {
	// Create a basic assume role policy document using structs
	assumeRolePolicy := PolicyDocument{
		Version: "2012-10-17",
		Statement: []Statement{
			{
				Effect: "Allow",
				Principal: map[string]string{
					"AWS": "*",
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
		RoleName:                 aws.String(role.GetSnakeCaseName()),
		AssumeRolePolicyDocument: aws.String(string(assumeRolePolicyJSON)),
		Description:              aws.String(role.Description),
	}

	result, err := p.service.CreateRole(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create IAM role: %w", err)
	}

	return result.Role, nil
}

// PolicyDocument represents an IAM policy document
type PolicyDocument struct {
	Version   string      `json:"Version"`
	Statement []Statement `json:"Statement"`
}

// Statement represents a policy statement
type Statement struct {
	Effect    string `json:"Effect"`
	Action    any    `json:"Action,omitempty"`    // Can be string or []string
	Resource  any    `json:"Resource,omitempty"`  // Can be string or []string
	Principal any    `json:"Principal,omitempty"` // For assume role policies
}

// attachPoliciesToRole creates and attaches an inline policy with the specified permissions
func (p *awsProvider) attachPoliciesToRole(ctx context.Context, roleName *string, permissions []string) error {
	if len(permissions) == 0 {
		return nil // No permissions to attach
	}

	// Create a policy document using proper structs
	policyDocument := PolicyDocument{
		Version: "2012-10-17",
		Statement: []Statement{
			{
				Effect:   "Allow",
				Action:   permissions,
				Resource: "*",
			},
		},
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
func (p *awsProvider) bindUserToRole(ctx context.Context, user *models.User, roleName *string) error {
	// Create a basic assume role policy that allows the user to assume the role
	var assumeRolePolicy PolicyDocument

	if len(user.Email) > 0 {
		// Extract username from email (part before @)
		username := strings.Split(user.Email, "@")[0]
		// Create policy allowing specific user - note: you may need to adjust the account ID
		assumeRolePolicy = PolicyDocument{
			Version: "2012-10-17",
			Statement: []Statement{
				{
					Effect: "Allow",
					Principal: map[string]string{
						"AWS": fmt.Sprintf("arn:aws:iam::*:user/%s", username),
					},
					Action: "sts:AssumeRole",
				},
			},
		}
	} else {
		// Basic policy allowing any AWS principal if no email provided
		assumeRolePolicy = PolicyDocument{
			Version: "2012-10-17",
			Statement: []Statement{
				{
					Effect: "Allow",
					Principal: map[string]string{
						"AWS": "*",
					},
					Action: "sts:AssumeRole",
				},
			},
		}
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
