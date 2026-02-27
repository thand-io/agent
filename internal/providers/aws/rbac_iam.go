package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	sdkWorkflowsRunner "github.com/thand-io/agent/sdk/workflows/runner"
	"go.temporal.io/sdk/workflow"
)

// authorizeRoleTraditionalIAM handles role authorization for traditional IAM users.
// Each sub-step is dispatched as a Temporal activity when a workflow context is
// present, or executed inline otherwise. The exec* helpers encapsulate that branching.
//
// Role lifecycle:
//   - Composite roles: unique per identity, always created fresh and policies re-applied
//     on each authorize. Deleted on revoke.
//   - Non-composite roles: persistent, created once with a version tag
//     (thand:version / thand:managed). On subsequent authorizations the version
//     tag is checked; policies are only updated when the version has changed.
//     On revoke the user binding is removed but the role is retained.
func (p *awsProvider) authorizeRoleTraditionalIAM(
	task models.ProviderContext,
	req *models.AuthorizeRoleRequest,
	targetAccountID string,
) (*models.AuthorizeRoleResponse, error) {

	user := req.GetUser()
	role := req.GetRole()
	isComposite := role.IsComposite()

	logrus.WithFields(logrus.Fields{
		"role":         role.GetName(),
		"is_composite": isComposite,
	}).Info("IAM authorizeRole: determining role lifecycle")

	// Step 1 — get or create the IAM role.
	// For non-composite roles we pass the current version so the activity can
	// compare it against the existing role's thand:version tag and report
	// whether the role's policies need updating.
	getOrCreateReq := &GetOrCreateIAMRoleRequest{
		User:            user,
		Role:            &role.Role,
		RoleName:        role.GetName(),
		TargetAccountID: targetAccountID,
		IsComposite:     isComposite,
	}
	if !isComposite {
		getOrCreateReq.Version = role.GetVersionString()
	}

	roleResp, err := p.execGetOrCreateIAMRole(task, getOrCreateReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create IAM role: %w", err)
	}

	// Step 2 — attach permissions.
	// Composite: always update policies (role is per-session).
	// Non-composite: only update when NeedsUpdate is true (version changed or role was just created).
	needsPolicyUpdate := isComposite || roleResp.NeedsUpdate
	if needsPolicyUpdate && (len(role.Permissions.Allow) > 0 || len(role.Permissions.Deny) > 0) {
		if err := p.execAttachPoliciesToIAMRole(task, &AttachPoliciesToIAMRoleRequest{
			RoleName:    roleResp.RoleName,
			Permissions: role.Permissions,
		}); err != nil {
			return nil, fmt.Errorf("failed to attach policies to role: %w", err)
		}
	}

	// Step 2.5 — for non-composite roles: update the version tag after policies
	// have been applied so future calls can skip redundant updates.
	if !isComposite && roleResp.NeedsUpdate {
		if err := p.execTagIAMRole(task, &TagIAMRoleRequest{
			RoleName: roleResp.RoleName,
			Tags: map[string]string{
				models.ThandVersionTagKey: role.GetVersionString(),
				models.ThandManagedTagKey: "true",
			},
		}); err != nil {
			// Tagging failure is not fatal — log and continue
			logrus.WithError(err).Warn("IAM: failed to tag role with version; policies were applied successfully")
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

	return &models.AuthorizeRoleResponse{}, nil
}

// revokeRoleTraditionalIAM handles role revocation for traditional IAM users.
// Uses GetIAMRole (read-only) to prevent silently re-creating a deleted role.
func (p *awsProvider) revokeRoleTraditionalIAM(task models.ProviderContext, user *models.User, role *models.CompositeRole) (*models.RevokeRoleResponse, error) {

	// Step 1 — resolve the existing role; fail fast if not found, never create
	roleResp, err := p.execGetIAMRole(task, &GetIAMRoleRequest{Role: &role.Role, RoleName: role.GetName()})
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
func (p *awsProvider) getRole(ctx context.Context, roleName string) (*types.Role, error) {
	input := &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	}

	result, err := p.service.GetRole(ctx, input)
	if err != nil {
		// Return nil role and error if role doesn't exist
		return nil, err
	}
	return result.Role, nil
}

// createRole creates a new IAM role with the specified permissions.
// When tags are provided they are set atomically during creation (used for
// non-composite roles to record the thand:version and thand:managed markers).
func (p *awsProvider) createRole(ctx context.Context, roleName, description, targetAccountID string, tags map[string]string) (*types.Role, error) {
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
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(string(assumeRolePolicyJSON)),
		Description:              aws.String(description),
	}

	// Attach tags when provided (non-composite lifecycle)
	if len(tags) > 0 {
		for k, v := range tags {
			input.Tags = append(input.Tags, types.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}
	}

	result, err := p.service.CreateRole(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create IAM role: %w", err)
	}

	return result.Role, nil
}

// getRoleVersionTag reads the thand:version tag from an existing IAM role.
// Returns the version string and true if found, or "" and false otherwise.
func (p *awsProvider) getRoleVersionTag(ctx context.Context, roleName string) (string, bool) {
	resp, err := p.service.ListRoleTags(ctx, &iam.ListRoleTagsInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		logrus.WithError(err).WithField("role", roleName).Warn("Failed to list IAM role tags")
		return "", false
	}
	for _, tag := range resp.Tags {
		if tag.Key != nil && *tag.Key == models.ThandVersionTagKey && tag.Value != nil {
			return *tag.Value, true
		}
	}
	return "", false
}

// tagRole sets or updates tags on an existing IAM role.
func (p *awsProvider) tagRole(ctx context.Context, roleName string, tags map[string]string) error {
	var iamTags []types.Tag
	for k, v := range tags {
		iamTags = append(iamTags, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	_, err := p.service.TagRole(ctx, &iam.TagRoleInput{
		RoleName: aws.String(roleName),
		Tags:     iamTags,
	})
	if err != nil {
		return fmt.Errorf("failed to tag IAM role %s: %w", roleName, err)
	}
	return nil
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
	ctx models.ProviderContext,
	req *GetOrCreateIAMRoleRequest,
) (*GetOrCreateIAMRoleResponse, error) {
	if workflowCtx, ok := ctx.(workflow.Context); ok {
		wfCtx := workflow.WithActivityOptions(workflowCtx, workflow.ActivityOptions{
			StartToCloseTimeout: 2 * time.Minute,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		var resp GetOrCreateIAMRoleResponse
		if err := workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), GetOrCreateIAMRoleActivityName), req).Get(wfCtx, &resp); err != nil {
			return nil, err
		}
		return &resp, nil
	}

	localCtx := ctx.(context.Context)

	// Inline (non-Temporal) implementation:
	// For non-composite roles, check the version tag to determine if policies
	// need to be refreshed. For composite roles, always report NeedsUpdate.
	existingRole, err := p.getRole(localCtx, req.RoleName)
	if err != nil {
		// Role doesn't exist — create it.
		var tags map[string]string
		if !req.IsComposite && len(req.Version) > 0 {
			tags = map[string]string{
				models.ThandVersionTagKey: req.Version,
				models.ThandManagedTagKey: "true",
			}
		}
		existingRole, err = p.createRole(localCtx, req.RoleName, req.Role.Description, req.TargetAccountID, tags)
		if err != nil {
			return nil, fmt.Errorf("failed to get or create IAM role: %w", err)
		}
		return buildIAMRoleResponse(existingRole, true), nil
	}

	// Role exists. Determine whether its policies are stale.
	needsUpdate := true // composite roles always get refreshed
	if !req.IsComposite && len(req.Version) > 0 && existingRole.RoleName != nil {
		currentVersion, found := p.getRoleVersionTag(localCtx, *existingRole.RoleName)
		if found && currentVersion == req.Version {
			needsUpdate = false // version matches — policies are current
		}
	}
	return buildIAMRoleResponse(existingRole, needsUpdate), nil
}

// buildIAMRoleResponse is a small helper that populates GetOrCreateIAMRoleResponse.
func buildIAMRoleResponse(r *types.Role, needsUpdate bool) *GetOrCreateIAMRoleResponse {
	resp := &GetOrCreateIAMRoleResponse{NeedsUpdate: needsUpdate}
	if r.RoleName != nil {
		resp.RoleName = *r.RoleName
	}
	if r.Arn != nil {
		resp.RoleArn = *r.Arn
	}
	return resp
}

// execTagIAMRole sets tags on an existing IAM role. Used to record version
// metadata on non-composite roles after policies have been applied.
func (p *awsProvider) execTagIAMRole(
	ctx models.ProviderContext,
	req *TagIAMRoleRequest,
) error {
	if workflowCtx, ok := ctx.(workflow.Context); ok {
		wfCtx := workflow.WithActivityOptions(workflowCtx, workflow.ActivityOptions{
			StartToCloseTimeout: 30 * time.Second,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		return workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), TagIAMRoleActivityName), req).Get(wfCtx, nil)
	}
	localCtx := ctx.(context.Context)
	return p.tagRole(localCtx, req.RoleName, req.Tags)
}

func (p *awsProvider) execGetIAMRole(
	ctx models.ProviderContext,
	req *GetIAMRoleRequest,
) (*GetIAMRoleResponse, error) {
	if workflowCtx, ok := ctx.(workflow.Context); ok {
		wfCtx := workflow.WithActivityOptions(workflowCtx, workflow.ActivityOptions{
			StartToCloseTimeout: 30 * time.Second,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		var resp GetIAMRoleResponse
		if err := workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), GetIAMRoleActivityName), req).Get(wfCtx, &resp); err != nil {
			return nil, err
		}
		return &resp, nil
	}
	localCtx := ctx.(context.Context)
	rawRole, err := p.getRole(localCtx, req.RoleName)
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
	ctx models.ProviderContext,
	req *AttachPoliciesToIAMRoleRequest,
) error {
	if workflowCtx, ok := ctx.(workflow.Context); ok {
		wfCtx := workflow.WithActivityOptions(workflowCtx, workflow.ActivityOptions{
			StartToCloseTimeout: 2 * time.Minute,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		return workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), AttachPoliciesToIAMRoleActivityName), req).Get(wfCtx, nil)
	}
	localCtx := ctx.(context.Context)
	return p.attachPoliciesToRole(localCtx, req.RoleName, req.Permissions)
}

func (p *awsProvider) execBindUserToIAMRole(
	ctx models.ProviderContext,
	req *BindUserToIAMRoleRequest,
) error {
	if workflowCtx, ok := ctx.(workflow.Context); ok {
		wfCtx := workflow.WithActivityOptions(workflowCtx, workflow.ActivityOptions{
			StartToCloseTimeout: 2 * time.Minute,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		return workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), BindUserToIAMRoleActivityName), req).Get(wfCtx, nil)
	}
	localCtx := ctx.(context.Context)
	return p.bindUserToRole(localCtx, req.User, req.RoleName, req.TargetAccountID)
}

func (p *awsProvider) execUnbindUserFromIAMRole(
	ctx models.ProviderContext,
	req *UnbindUserFromIAMRoleRequest,
) error {
	if workflowCtx, ok := ctx.(workflow.Context); ok {
		wfCtx := workflow.WithActivityOptions(workflowCtx, workflow.ActivityOptions{
			StartToCloseTimeout: 2 * time.Minute,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		return workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), UnbindUserFromIAMRoleActivityName), req).Get(wfCtx, nil)
	}
	localCtx := ctx.(context.Context)
	return p.unbindUserFromRole(localCtx, req.User, req.RoleName)
}
