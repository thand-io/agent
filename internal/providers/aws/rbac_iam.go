package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	sdkWorkflowsRunner "github.com/thand-io/agent/sdk/workflows/runner"
	"go.temporal.io/sdk/workflow"
)

// authorizeRoleTraditionalIAM handles role authorization for traditional IAM users.
// Each sub-step is dispatched as a Temporal activity when a workflow context is
// present, or executed inline otherwise. The exec* helpers encapsulate that branching.
func (p *awsProvider) authorizeRoleTraditionalIAM(
	task models.ProviderContext,
	req *models.AuthorizeRoleRequest,
	targetAccountID string,
) (*models.AuthorizeRoleResponse, error) {

	user := req.GetUser()
	role := req.GetRole()

	// Step 1 — get or create the IAM role
	roleResp, err := p.execGetOrCreateIAMRole(task, &GetOrCreateIAMRoleRequest{
		User:            user,
		Role:            role,
		TargetAccountID: targetAccountID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get or create IAM role: %w", err)
	}

	// Step 2 — attach permissions (only if defined)
	if len(role.Permissions.Allow) > 0 || len(role.Permissions.Deny) > 0 {
		if err := p.execAttachPoliciesToIAMRole(task, &AttachPoliciesToIAMRoleRequest{
			RoleName:    roleResp.RoleName,
			Permissions: role.Permissions,
		}); err != nil {
			return nil, fmt.Errorf("failed to attach policies to role: %w", err)
		}
	}

	// Step 3 — bind the user to the role
	if err := p.execBindUserToIAMRole(task, &BindUserToIAMRoleRequest{
		User:            user,
		RoleName:        roleResp.RoleName,
		TargetAccountID: targetAccountID,
	}); err != nil {
		return nil, fmt.Errorf("failed to bind user to role: %w", err)
	}

	return nil, nil
}

// revokeRoleTraditionalIAM handles role revocation for traditional IAM users.
// Uses GetIAMRole (read-only) to prevent silently re-creating a deleted role.
func (p *awsProvider) revokeRoleTraditionalIAM(task models.ProviderContext, user *models.User, role *models.Role) (*models.RevokeRoleResponse, error) {

	// Step 1 — resolve the existing role; fail fast if not found, never create
	roleResp, err := p.execGetIAMRole(task, &GetIAMRoleRequest{Role: role})
	if err != nil {
		return nil, fmt.Errorf("role not found: %w", err)
	}

	// Step 2 — remove the user from the assume-role policy
	if err := p.execUnbindUserFromIAMRole(task, &UnbindUserFromIAMRoleRequest{
		User:     user,
		RoleName: roleResp.RoleName,
	}); err != nil {
		return nil, fmt.Errorf("failed to unbind user from role: %w", err)
	}

	return nil, nil
}

// getRole retrieves an IAM role by name
func (p *awsProvider) getRole(ctx context.Context, user *models.User, role *models.Role) (*types.Role, error) {
	input := &iam.GetRoleInput{
		RoleName: aws.String(role.GetIdentifier()),
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
		RoleName:                 aws.String(role.GetIdentifier()),
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
func (p *awsProvider) attachPoliciesToRole(ctx context.Context, roleName string, permissions models.RolePermissions) error {
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

	policyName := fmt.Sprintf("thand-%s-policy", common.ConvertToSnakeCase(roleName))
	input := &iam.PutRolePolicyInput{
		RoleName:       aws.String(roleName),
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
func (p *awsProvider) bindUserToRole(ctx context.Context, user *models.User, roleName string, targetAccountID string) error {
	username := p.getUsernameForIAM(user)
	if len(username) == 0 {
		return fmt.Errorf("failed to determine username for user")
	}

	assumeRolePolicy := PolicyDocument{
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

	_, err = p.service.UpdateAssumeRolePolicy(ctx, &iam.UpdateAssumeRolePolicyInput{
		RoleName:       aws.String(roleName),
		PolicyDocument: aws.String(string(assumeRolePolicyJSON)),
	})
	if err != nil {
		return fmt.Errorf("failed to update assume role policy: %w", err)
	}

	return nil
}

// unbindUserFromRole removes the user from the assume role policy
func (p *awsProvider) unbindUserFromRole(ctx context.Context, user *models.User, roleName string) error {
	accountID := p.GetAccountID()

	roleOutput, err := p.service.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		return fmt.Errorf("failed to get role %s: %w", roleName, err)
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
		RoleName:       aws.String(roleName),
		PolicyDocument: aws.String(string(newPolicyJSON)),
	})
	if err != nil {
		return fmt.Errorf("failed to update assume role policy for role %s: %w", roleName, err)
	}

	return nil
}

// getUsernameForIAM determines the appropriate username for AWS IAM user ARN
// Priority: Username field > email prefix > empty string (fallback to account root)
func (p *awsProvider) getUsernameForIAM(user *models.User) string {
	if len(user.Username) > 0 {
		return user.Username
	}
	if len(user.Email) > 0 {
		return common.ExtractUsernameFromEmail(user.Email)
	}
	return ""
}

// ─────────────────────────────────────────────────────────────────────────────
// IAM operation wrappers
//
// Each wrapper handles the Temporal/direct branching identically to the SSO
// exec* helpers in rbac_sso.go. The Temporal path dispatches an activity;
// the direct path calls the private method inline.
// ─────────────────────────────────────────────────────────────────────────────

func (p *awsProvider) execGetOrCreateIAMRole(
	task models.ProviderContext,
	req *GetOrCreateIAMRoleRequest,
) (*GetOrCreateIAMRoleResponse, error) {
	if task.HasTemporalContext() {
		wfCtx := workflow.WithActivityOptions(task.GetTemporalContext(), workflow.ActivityOptions{
			TaskQueue:           task.GetTaskQueue(),
			StartToCloseTimeout: 2 * time.Minute,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		var resp GetOrCreateIAMRoleResponse
		if err := workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), "GetOrCreateIAMRole"), req).Get(wfCtx, &resp); err != nil {
			return nil, err
		}
		return &resp, nil
	}
	existingRole, err := p.getRole(task.GetContext(), req.User, req.Role)
	if err != nil {
		existingRole, err = p.createRole(task.GetContext(), req.User, req.Role, req.TargetAccountID)
		if err != nil {
			return nil, fmt.Errorf("failed to get or create IAM role: %w", err)
		}
	}
	resp := &GetOrCreateIAMRoleResponse{}
	if existingRole.RoleName != nil {
		resp.RoleName = *existingRole.RoleName
	}
	if existingRole.Arn != nil {
		resp.RoleArn = *existingRole.Arn
	}
	return resp, nil
}

func (p *awsProvider) execGetIAMRole(
	task models.ProviderContext,
	req *GetIAMRoleRequest,
) (*GetIAMRoleResponse, error) {
	if task.HasTemporalContext() {
		wfCtx := workflow.WithActivityOptions(task.GetTemporalContext(), workflow.ActivityOptions{
			TaskQueue:           task.GetTaskQueue(),
			StartToCloseTimeout: 30 * time.Second,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		var resp GetIAMRoleResponse
		if err := workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), "GetIAMRole"), req).Get(wfCtx, &resp); err != nil {
			return nil, err
		}
		return &resp, nil
	}
	rawRole, err := p.getRole(task.GetContext(), nil, req.Role)
	if err != nil {
		return nil, fmt.Errorf("IAM role not found: %w", err)
	}
	resp := &GetIAMRoleResponse{}
	if rawRole.RoleName != nil {
		resp.RoleName = *rawRole.RoleName
	}
	if rawRole.Arn != nil {
		resp.RoleArn = *rawRole.Arn
	}
	return resp, nil
}

func (p *awsProvider) execAttachPoliciesToIAMRole(
	task models.ProviderContext,
	req *AttachPoliciesToIAMRoleRequest,
) error {
	if task.HasTemporalContext() {
		wfCtx := workflow.WithActivityOptions(task.GetTemporalContext(), workflow.ActivityOptions{
			TaskQueue:           task.GetTaskQueue(),
			StartToCloseTimeout: 2 * time.Minute,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		return workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), "AttachPoliciesToIAMRole"), req).Get(wfCtx, nil)
	}
	return p.attachPoliciesToRole(task.GetContext(), req.RoleName, req.Permissions)
}

func (p *awsProvider) execBindUserToIAMRole(
	task models.ProviderContext,
	req *BindUserToIAMRoleRequest,
) error {
	if task.HasTemporalContext() {
		wfCtx := workflow.WithActivityOptions(task.GetTemporalContext(), workflow.ActivityOptions{
			TaskQueue:           task.GetTaskQueue(),
			StartToCloseTimeout: 2 * time.Minute,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		return workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), "BindUserToIAMRole"), req).Get(wfCtx, nil)
	}
	return p.bindUserToRole(task.GetContext(), req.User, req.RoleName, req.TargetAccountID)
}

func (p *awsProvider) execUnbindUserFromIAMRole(
	task models.ProviderContext,
	req *UnbindUserFromIAMRoleRequest,
) error {
	if task.HasTemporalContext() {
		wfCtx := workflow.WithActivityOptions(task.GetTemporalContext(), workflow.ActivityOptions{
			TaskQueue:           task.GetTaskQueue(),
			StartToCloseTimeout: 2 * time.Minute,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		return workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), "UnbindUserFromIAMRole"), req).Get(wfCtx, nil)
	}
	return p.unbindUserFromRole(task.GetContext(), req.User, req.RoleName)
}
