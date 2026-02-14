package aws

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

// Authorize grants access for a user to a role
func (p *awsProvider) AuthorizeRole(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
) (*models.AuthorizeRoleResponse, error) {

	// Check for nil inputs first to avoid nil pointer dereference
	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to authorize aws role")
	}

	// Determine target account: selected account or configured default
	targetAccountID := p.GetAccountID()

	if len(req.Tenant) > 0 {
		// If a tenant is specified, use it as the target account ID
		targetAccountID = req.Tenant
	}

	logrus.WithFields(logrus.Fields{
		"req_user_email":    req.User.Email,
		"req_user_source":   req.User.Source,
		"req_user_username": req.User.Username,
		"account_id":        targetAccountID,
	}).Info("AWS AuthorizeRole called")

	// Determine if we should use IAM Identity Center or traditional IAM
	// For now, detect based on the user's source or configuration
	useIdentityCenter := p.shouldUseIdentityCenter(req.GetUser())

	if useIdentityCenter {
		return p.authorizeRoleIdentityCenter(ctx, req, targetAccountID)
	} else {
		return p.authorizeRoleTraditionalIAM(ctx, req, targetAccountID)
	}
}

// Revoke removes access for a user from a role
func (p *awsProvider) RevokeRole(
	ctx context.Context,
	req *models.RevokeRoleRequest,
) (*models.RevokeRoleResponse, error) {
	// Check for nil inputs
	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to revoke aws role")
	}

	user := req.GetUser()
	role := req.GetRole()

	// Determine target account: selected account or configured default
	targetAccountID := p.GetAccountID()

	if len(req.Tenant) > 0 {
		// If a tenant is specified, use it as the target account ID
		targetAccountID = req.Tenant
	}

	// Determine if we should use IAM Identity Center or traditional IAM
	useIdentityCenter := p.shouldUseIdentityCenter(user)

	if useIdentityCenter {
		err := p.revokeRoleIdentityCenter(ctx, req, targetAccountID)
		if err != nil {
			return nil, fmt.Errorf("failed to revoke Identity Center role: %w", err)
		}
		return nil, nil
	} else {
		return p.revokeRoleTraditionalIAM(ctx, user, role)
	}
}

func (p *awsProvider) GetAuthorizedAccessUrl(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
	resp *models.AuthorizeRoleResponse,
) string {

	return p.GetConfig().GetStringWithDefault(
		"sso_start_url", "https://console.aws.amazon.com/")

}

// shouldUseIdentityCenter determines if we should use Identity Center based on user context
func (p *awsProvider) shouldUseIdentityCenter(user *models.User) bool {
	// For now, assume Identity Center if user source suggests SSO
	// You could also check for specific configuration flags
	useIC := true
	if len(user.Email) == 0 && len(user.Username) > 0 {
		// We only have a username, likely IAM
		useIC = false
	} else if user.Source == "iam" && len(user.Username) > 0 {
		// If the source is 'iam' (default to traditional IAM)
		useIC = false
	}
	logrus.WithFields(logrus.Fields{
		"user_source":         user.Source,
		"user_email":          user.Email,
		"user_username":       user.Username,
		"use_identity_center": useIC,
	}).Info("Determining whether to use Identity Center")
	return useIC
}

// PolicyDocument represents an IAM policy document
type PolicyDocument struct {
	Version   string      `json:"Version"`
	Statement []Statement `json:"Statement"`
}

// Statement represents a policy statement
type Statement struct {
	Effect    string         `json:"Effect"`
	Action    any            `json:"Action,omitempty"`    // Can be string or []string
	Resource  any            `json:"Resource,omitempty"`  // Can be string or []string
	Principal any            `json:"Principal,omitempty"` // For assume role policies
	Condition map[string]any `json:"Condition,omitempty"` // IAM policy conditions
}

// permissionsToAwsPolicy converts CSP-agnostic Permissions to AWS PolicyDocument
// The effect (Allow/Deny) comes from the parent Permissions.Allow or Permissions.Deny
func permissionsToAwsPolicy(permissions models.RolePermissions) PolicyDocument {
	awsStatements := []Statement{}

	// Process Allow statements
	for _, stmt := range permissions.Allow {
		// Skip statements with no operations - AWS requires at least one Action
		if len(stmt.Operations) == 0 {
			continue
		}

		// Default resource to "*" if no targets specified (backwards compatibility)
		var resource any = "*"
		if len(stmt.Targets) > 0 {
			resource = stmt.Targets
		}

		awsStatements = append(awsStatements, Statement{
			Effect:    "Allow",
			Action:    stmt.Operations,
			Resource:  resource,
			Condition: stmt.Conditions,
		})
	}

	// Process Deny statements
	for _, stmt := range permissions.Deny {
		// Skip statements with no operations - AWS requires at least one Action
		if len(stmt.Operations) == 0 {
			continue
		}

		// Default resource to "*" if no targets specified (backwards compatibility)
		var resource any = "*"
		if len(stmt.Targets) > 0 {
			resource = stmt.Targets
		}

		awsStatements = append(awsStatements, Statement{
			Effect:    "Deny",
			Action:    stmt.Operations,
			Resource:  resource,
			Condition: stmt.Conditions,
		})
	}

	return PolicyDocument{
		Version:   "2012-10-17",
		Statement: awsStatements,
	}
}
