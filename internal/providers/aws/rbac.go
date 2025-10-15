package aws

import (
	"context"
	"fmt"

	"github.com/thand-io/agent/internal/models"
)

// Authorize grants access for a user to a role
func (p *awsProvider) AuthorizeRole(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
) (map[string]any, error) {

	// Check for nil inputs
	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to authorize aws role")
	}

	// Determine if we should use IAM Identity Center or traditional IAM
	// For now, detect based on the user's source or configuration
	useIdentityCenter := p.shouldUseIdentityCenter(req.GetUser())

	if useIdentityCenter {
		return p.authorizeRoleIdentityCenter(ctx, req)
	} else {
		return p.authorizeRoleTraditionalIAM(ctx, req)
	}
}

// Revoke removes access for a user from a role
func (p *awsProvider) RevokeRole(
	ctx context.Context,
	user *models.User,
	role *models.Role,
	metadata map[string]any,
) (map[string]any, error) {
	// Check for nil inputs
	if user == nil {
		return nil, fmt.Errorf("user cannot be nil")
	}
	if role == nil {
		return nil, fmt.Errorf("role cannot be nil")
	}

	// Determine if we should use IAM Identity Center or traditional IAM
	useIdentityCenter := p.shouldUseIdentityCenter(user)

	if useIdentityCenter {
		err := p.revokeRoleIdentityCenter(ctx, user, role)
		if err != nil {
			return nil, fmt.Errorf("failed to revoke Identity Center role: %w", err)
		}
		return nil, nil
	} else {
		return p.revokeRoleTraditionalIAM(ctx, user, role, metadata)
	}
}

// shouldUseIdentityCenter determines if we should use Identity Center based on user context
func (p *awsProvider) shouldUseIdentityCenter(user *models.User) bool {
	// For now, assume Identity Center if user source suggests SSO
	// You could also check for specific configuration flags
	return user.Source != "" && user.Source != "iam"
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
