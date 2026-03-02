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
	RoleName    string       `json:"role_name"`         // CSP resource name (from CompositeRole.GetName())
	IsComposite bool         `json:"is_composite"`      // Determines lifecycle: composite = always refresh; non-composite = version-checked
	Version     string       `json:"version,omitempty"` // Role version for tagging (non-composite only)
}

type FindOrCreatePermissionSetResponse struct {
	PermissionSetArn string `json:"permission_set_arn"`
	NeedsUpdate      bool   `json:"needs_update"` // true when policies were (re-)attached
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

// ProvisionPermissionSetRequest triggers provisioning of a permission set to all
// provisioned accounts. Must be called after any policy mutation on the set.
type ProvisionPermissionSetRequest struct {
	InstanceArn      string `json:"instance_arn"`
	PermissionSetArn string `json:"permission_set_arn"`
}

type ProvisionPermissionSetResponse struct {
	RequestId string `json:"request_id"`
}

// CheckPermissionSetProvisioningStatusRequest polls the status of a
// ProvisionPermissionSet operation (single-shot, no sleep).
type CheckPermissionSetProvisioningStatusRequest struct {
	InstanceArn string `json:"instance_arn"`
	RequestId   string `json:"request_id"`
}

type CheckPermissionSetProvisioningStatusResponse struct {
	Succeeded bool `json:"succeeded"`
}

// ─────────────────────────────────────────────────────────────────────────────
// IAM request & response types
// ─────────────────────────────────────────────────────────────────────────────

type GetOrCreateIAMRoleRequest struct {
	User            *models.User `json:"user"`
	Role            *models.Role `json:"role"`
	RoleName        string       `json:"role_name"` // CSP resource name (from CompositeRole.GetName())
	TargetAccountID string       `json:"target_account_id"`
	IsComposite     bool         `json:"is_composite"`      // Determines lifecycle: composite = always refresh; non-composite = version-checked
	Version         string       `json:"version,omitempty"` // Role version for tagging (non-composite only)
}

type GetOrCreateIAMRoleResponse struct {
	RoleName    string `json:"role_name"`
	RoleArn     string `json:"role_arn"`
	NeedsUpdate bool   `json:"needs_update"` // true when policies should be (re-)attached
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

// GetIAMRoleRequest looks up an existing IAM role by name (read-only, no creation).
type GetIAMRoleRequest struct {
	Role     *models.Role `json:"role"`
	RoleName string       `json:"role_name"` // CSP resource name (from CompositeRole.GetName())
}

type GetIAMRoleResponse struct {
	RoleName string `json:"role_name"`
	RoleArn  string `json:"role_arn"`
}

// TagIAMRoleRequest sets tags on an existing IAM role. Used to record
// version metadata for non-composite roles.
type TagIAMRoleRequest struct {
	RoleName string            `json:"role_name"`
	Tags     map[string]string `json:"tags"`
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
//
// For non-composite roles, the permission set is tagged with a version and
// policies are only refreshed when the version changes.
func (a *awsProviderActivities) FindOrCreatePermissionSet(
	ctx context.Context,
	req *FindOrCreatePermissionSetRequest,
) (*FindOrCreatePermissionSetResponse, error) {
	arn, needsUpdate, err := a.provider.findOrCreatePermissionSet(ctx, req.InstanceArn, req.RoleName, req.Role, req.IsComposite, req.Version)
	if err != nil {
		return nil, err
	}
	return &FindOrCreatePermissionSetResponse{PermissionSetArn: arn, NeedsUpdate: needsUpdate}, nil
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

// ProvisionPermissionSet pushes permission set policy changes to all provisioned
// accounts. Must be called after PutInlinePolicyToPermissionSet or
// AttachManagedPolicyToPermissionSet to ensure updates propagate.
func (a *awsProviderActivities) ProvisionPermissionSet(
	ctx context.Context,
	req *ProvisionPermissionSetRequest,
) (*ProvisionPermissionSetResponse, error) {
	requestId, err := a.provider.provisionPermissionSet(ctx, req.InstanceArn, req.PermissionSetArn)
	if err != nil {
		return nil, err
	}
	return &ProvisionPermissionSetResponse{RequestId: requestId}, nil
}

// CheckPermissionSetProvisioningStatus performs a single poll of the provisioning
// status returned by ProvisionPermissionSet. The backoff loop and sleep live in
// the workflow, matching the pattern used for CheckAssignmentDeletionStatus.
func (a *awsProviderActivities) CheckPermissionSetProvisioningStatus(
	ctx context.Context,
	req *CheckPermissionSetProvisioningStatusRequest,
) (*CheckPermissionSetProvisioningStatusResponse, error) {
	succeeded, err := a.provider.checkPermissionSetProvisioningStatus(ctx, req.InstanceArn, req.RequestId)
	if err != nil {
		return nil, err
	}
	return &CheckPermissionSetProvisioningStatusResponse{Succeeded: succeeded}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// IAM activity implementations
// ─────────────────────────────────────────────────────────────────────────────

// GetOrCreateIAMRole retrieves an existing IAM role or creates it if absent.
// For non-composite roles (IsComposite == false), version tags are compared
// to determine whether the role's policies need refreshing.
// Safe to retry — duplicate creation is handled by AWS returning the existing role.
func (a *awsProviderActivities) GetOrCreateIAMRole(
	ctx context.Context,
	req *GetOrCreateIAMRoleRequest,
) (*GetOrCreateIAMRoleResponse, error) {
	existingRole, err := a.provider.getRole(ctx, req.RoleName)
	if err != nil {
		// Role doesn't exist — create it, optionally with version tags.
		var tags map[string]string
		if !req.IsComposite && len(req.Version) > 0 {
			tags = map[string]string{
				models.ThandVersionTagKey: req.Version,
				models.ThandManagedTagKey: "true",
			}
		}
		existingRole, err = a.provider.createRole(ctx, req.RoleName, req.Role.Description, req.TargetAccountID, tags)
		if err != nil {
			return nil, fmt.Errorf("failed to get or create IAM role: %w", err)
		}
		return buildIAMRoleResponse(existingRole, true), nil
	}

	// Role exists. Determine whether its policies are stale.
	needsUpdate := true // composite roles always get refreshed
	if !req.IsComposite && len(req.Version) > 0 && existingRole.RoleName != nil {
		currentVersion, found := a.provider.getRoleVersionTag(ctx, *existingRole.RoleName)
		if found && currentVersion == req.Version {
			needsUpdate = false
		}
	}
	return buildIAMRoleResponse(existingRole, needsUpdate), nil
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
	return a.provider.attachPoliciesToRole(ctx, req.RoleName, req.Permissions)
}

// BindUserToIAMRole updates the IAM role's assume-role policy to allow the
// specified user to assume it. Idempotent — UpdateAssumeRolePolicy is a full
// replace, so repeating the call with the same user produces the same result.
func (a *awsProviderActivities) BindUserToIAMRole(
	ctx context.Context,
	req *BindUserToIAMRoleRequest,
) error {
	return a.provider.bindUserToRole(ctx, req.User, req.RoleName, req.TargetAccountID)
}

// UnbindUserFromIAMRole removes the user from the IAM role's assume-role policy.
// Idempotent — if the user is not present, the operation is a no-op.
func (a *awsProviderActivities) UnbindUserFromIAMRole(
	ctx context.Context,
	req *UnbindUserFromIAMRoleRequest,
) error {
	return a.provider.unbindUserFromRole(ctx, req.User, req.RoleName)
}

// GetIAMRole retrieves an existing IAM role by name. Returns an error if the role
// does not exist — unlike GetOrCreateIAMRole it never creates one.
func (a *awsProviderActivities) GetIAMRole(
	ctx context.Context,
	req *GetIAMRoleRequest,
) (*GetIAMRoleResponse, error) {
	rawRole, err := a.provider.getRole(ctx, req.RoleName)
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

// TagIAMRole sets or updates tags on an existing IAM role.
// Used to record version metadata for non-composite roles.
func (a *awsProviderActivities) TagIAMRole(
	ctx context.Context,
	req *TagIAMRoleRequest,
) error {
	return a.provider.tagRole(ctx, req.RoleName, req.Tags)
}
