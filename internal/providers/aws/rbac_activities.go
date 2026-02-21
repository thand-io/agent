package aws

import (
	"context"
	"fmt"

	"github.com/thand-io/agent/internal/models"
)

// awsProviderActivities exposes granular AWS provider operations as individual Temporal
// activities. Each method maps to a single idempotent external API call (or a small,
// closely-related group of calls), allowing Temporal to independently retry, time-out,
// and track each step. These are registered in addition to the generic ProviderActivities
// using RegisterActivitiesForStruct.
type awsProviderActivities struct {
	provider *awsProvider
}

// ─────────────────────────────────────────────────────────────────────────────
// SSO / Identity Center request & response types
// ─────────────────────────────────────────────────────────────────────────────

type GetIdentityCenterInstanceRequest struct{}

type GetIdentityCenterInstanceResponse struct {
	InstanceArn     string `json:"instance_arn"`
	IdentityStoreId string `json:"identity_store_id"`
}

type FindOrCreatePermissionSetRequest struct {
	InstanceArn string       `json:"instance_arn"`
	Role        *models.Role `json:"role"`
}

type FindOrCreatePermissionSetResponse struct {
	PermissionSetArn string `json:"permission_set_arn"`
}

type FindIdentityCenterUserRequest struct {
	IdentityStoreId string `json:"identity_store_id"`
	Email           string `json:"email"`
}

type FindIdentityCenterUserResponse struct {
	PrincipalId string `json:"principal_id"`
}

type CreateAccountAssignmentRequest struct {
	InstanceArn      string `json:"instance_arn"`
	PermissionSetArn string `json:"permission_set_arn"`
	PrincipalId      string `json:"principal_id"`
	TargetAccountID  string `json:"target_account_id"`
}

type FindPermissionSetByNameRequest struct {
	InstanceArn      string `json:"instance_arn"`
	PermissionSetArn string `json:"permission_set_arn"` // use stored ARN if available
	RoleName         string `json:"role_name"`          // fallback name-based lookup
}

type FindPermissionSetByNameResponse struct {
	PermissionSetArn string `json:"permission_set_arn"`
}

type DeleteAccountAssignmentRequest struct {
	InstanceArn      string `json:"instance_arn"`
	PermissionSetArn string `json:"permission_set_arn"`
	PrincipalId      string `json:"principal_id"`
	TargetAccountID  string `json:"target_account_id"`
}

type DeleteAccountAssignmentResponse struct {
	RequestId string `json:"request_id"`
}

// CheckAssignmentDeletionStatusRequest carries the information for a single status
// poll. The RequestId is returned by DeleteAccountAssignment.
type CheckAssignmentDeletionStatusRequest struct {
	InstanceArn     string `json:"instance_arn"`
	RequestId       string `json:"request_id"`
	PrincipalId     string `json:"principal_id"`
	TargetAccountID string `json:"target_account_id"`
}

// CheckAssignmentDeletionStatusResponse reports whether the deletion has completed.
type CheckAssignmentDeletionStatusResponse struct {
	Succeeded bool `json:"succeeded"`
}

type CleanupPermissionSetRequest struct {
	InstanceArn      string `json:"instance_arn"`
	PermissionSetArn string `json:"permission_set_arn"`
}

// ─────────────────────────────────────────────────────────────────────────────
// IAM request & response types
// ─────────────────────────────────────────────────────────────────────────────

type GetOrCreateIAMRoleRequest struct {
	User            *models.User `json:"user"`
	Role            *models.Role `json:"role"`
	TargetAccountID string       `json:"target_account_id"`
}

type GetOrCreateIAMRoleResponse struct {
	RoleName string `json:"role_name"`
	RoleArn  string `json:"role_arn"`
}

type AttachPoliciesToIAMRoleRequest struct {
	RoleName    string                 `json:"role_name"`
	Permissions models.RolePermissions `json:"permissions"`
}

type BindUserToIAMRoleRequest struct {
	User            *models.User `json:"user"`
	RoleName        string       `json:"role_name"`
	TargetAccountID string       `json:"target_account_id"`
}

type UnbindUserFromIAMRoleRequest struct {
	User     *models.User `json:"user"`
	RoleName string       `json:"role_name"`
}

// ─────────────────────────────────────────────────────────────────────────────
// SSO activity implementations
// ─────────────────────────────────────────────────────────────────────────────

// GetIdentityCenterInstance retrieves the SSO instance ARN and identity store ID.
// Idempotent read-only operation; safe to retry.
func (a *awsProviderActivities) GetIdentityCenterInstance(
	ctx context.Context,
	_ *GetIdentityCenterInstanceRequest,
) (*GetIdentityCenterInstanceResponse, error) {
	instanceArn, identityStoreId, err := a.provider.getIdentityCenterInstance(ctx)
	if err != nil {
		return nil, err
	}
	return &GetIdentityCenterInstanceResponse{
		InstanceArn:     instanceArn,
		IdentityStoreId: identityStoreId,
	}, nil
}

// FindOrCreatePermissionSet finds an existing permission set by name, or creates
// a new one with the role's policies attached. Idempotent — repeated calls
// with the same role name and policies have no additional effect.
func (a *awsProviderActivities) FindOrCreatePermissionSet(
	ctx context.Context,
	req *FindOrCreatePermissionSetRequest,
) (*FindOrCreatePermissionSetResponse, error) {
	arn, err := a.provider.findOrCreatePermissionSet(ctx, req.InstanceArn, req.Role)
	if err != nil {
		return nil, err
	}
	return &FindOrCreatePermissionSetResponse{PermissionSetArn: arn}, nil
}

// FindIdentityCenterUser searches for a user in the Identity Center identity store
// by email (userName first, emails.value fallback). Idempotent read-only.
func (a *awsProviderActivities) FindIdentityCenterUser(
	ctx context.Context,
	req *FindIdentityCenterUserRequest,
) (*FindIdentityCenterUserResponse, error) {
	principalId, err := a.provider.findIdentityCenterUser(ctx, req.IdentityStoreId, req.Email)
	if err != nil {
		return nil, err
	}
	return &FindIdentityCenterUserResponse{PrincipalId: principalId}, nil
}

// CreateAccountAssignment assigns a permission set to a user for the target account.
// Idempotent — ConflictException (assignment already exists) is treated as success.
func (a *awsProviderActivities) CreateAccountAssignment(
	ctx context.Context,
	req *CreateAccountAssignmentRequest,
) error {
	return a.provider.createAccountAssignment(
		ctx,
		req.InstanceArn,
		req.PermissionSetArn,
		req.PrincipalId,
		req.TargetAccountID,
	)
}

// FindPermissionSetByName resolves the permission set ARN for revocation. It uses
// the stored ARN from authorization metadata if present; otherwise falls back to
// a name-based paginated search.
func (a *awsProviderActivities) FindPermissionSetByName(
	ctx context.Context,
	req *FindPermissionSetByNameRequest,
) (*FindPermissionSetByNameResponse, error) {
	if len(req.PermissionSetArn) > 0 {
		return &FindPermissionSetByNameResponse{PermissionSetArn: req.PermissionSetArn}, nil
	}
	arn, err := a.provider.findPermissionSetByName(ctx, req.InstanceArn, req.RoleName)
	if err != nil {
		return nil, err
	}
	return &FindPermissionSetByNameResponse{PermissionSetArn: arn}, nil
}

// DeleteAccountAssignment removes the user's permission set assignment from the account.
// Idempotent — ResourceNotFoundException is treated as success.
func (a *awsProviderActivities) DeleteAccountAssignment(
	ctx context.Context,
	req *DeleteAccountAssignmentRequest,
) (*DeleteAccountAssignmentResponse, error) {
	requestId, err := a.provider.deleteAccountAssignment(ctx, req.InstanceArn, req.PermissionSetArn, req.PrincipalId, req.TargetAccountID)
	if err != nil {
		return nil, err
	}
	return &DeleteAccountAssignmentResponse{RequestId: requestId}, nil
}

// CheckAssignmentDeletionStatus performs a single poll of the AWS account assignment
// deletion status. The backoff loop and sleep live in the workflow (or calling
// goroutine) so Temporal can schedule waits deterministically via workflow.Sleep.
func (a *awsProviderActivities) CheckAssignmentDeletionStatus(
	ctx context.Context,
	req *CheckAssignmentDeletionStatusRequest,
) (*CheckAssignmentDeletionStatusResponse, error) {
	succeeded, err := a.provider.checkAssignmentDeletionStatus(
		ctx, req.InstanceArn, req.RequestId, req.PrincipalId, req.TargetAccountID,
	)
	if err != nil {
		return nil, err
	}
	return &CheckAssignmentDeletionStatusResponse{Succeeded: succeeded}, nil
}

// CleanupPermissionSet attempts to delete a permission set if it is no longer
// assigned to any account. Non-fatal — errors are returned but the caller
// (registered with MaxAttempts:1) treats failure as a warning, not a blocker.
func (a *awsProviderActivities) CleanupPermissionSet(
	ctx context.Context,
	req *CleanupPermissionSetRequest,
) error {
	return a.provider.cleanupPermissionSetIfUnused(ctx, req.InstanceArn, req.PermissionSetArn)
}

// ─────────────────────────────────────────────────────────────────────────────
// IAM activity implementations
// ─────────────────────────────────────────────────────────────────────────────

// GetOrCreateIAMRole retrieves an existing IAM role or creates it if absent.
// Safe to retry — duplicate creation is handled by AWS returning the existing role.
func (a *awsProviderActivities) GetOrCreateIAMRole(
	ctx context.Context,
	req *GetOrCreateIAMRoleRequest,
) (*GetOrCreateIAMRoleResponse, error) {
	existingRole, err := a.provider.getRole(ctx, req.User, req.Role)
	if err != nil {
		existingRole, err = a.provider.createRole(ctx, req.User, req.Role, req.TargetAccountID)
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

// AttachPoliciesToIAMRole puts the inline policy onto the IAM role. Idempotent —
// PutRolePolicy is a full replace of the named policy, so re-running is safe.
func (a *awsProviderActivities) AttachPoliciesToIAMRole(
	ctx context.Context,
	req *AttachPoliciesToIAMRoleRequest,
) error {
	if len(req.Permissions.Allow) == 0 && len(req.Permissions.Deny) == 0 {
		return nil
	}
	roleName := req.RoleName
	return a.provider.attachPoliciesToRole(ctx, &roleName, req.Permissions)
}

// BindUserToIAMRole updates the IAM role's assume-role policy to allow the
// specified user to assume it. Idempotent — UpdateAssumeRolePolicy is a full
// replace, so repeating the call with the same user produces the same result.
func (a *awsProviderActivities) BindUserToIAMRole(
	ctx context.Context,
	req *BindUserToIAMRoleRequest,
) error {
	roleName := req.RoleName
	return a.provider.bindUserToRole(ctx, req.User, &roleName, req.TargetAccountID)
}

// UnbindUserFromIAMRole removes the user from the IAM role's assume-role policy.
// Idempotent — if the user is not present, the operation is a no-op.
func (a *awsProviderActivities) UnbindUserFromIAMRole(
	ctx context.Context,
	req *UnbindUserFromIAMRoleRequest,
) error {
	roleName := req.RoleName
	return a.provider.unbindUserFromRole(ctx, req.User, &roleName)
}
