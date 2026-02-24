package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/identitystore"
	identitystoretypes "github.com/aws/aws-sdk-go-v2/service/identitystore/types"
	"github.com/aws/aws-sdk-go-v2/service/ssoadmin"
	"github.com/aws/aws-sdk-go-v2/service/ssoadmin/types"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	sdkWorkflowsRunner "github.com/thand-io/agent/sdk/workflows/runner"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// authorizeRoleIdentityCenter handles role authorization for Identity Center users.
// Each step is dispatched as a Temporal activity when a workflow context is present,
// or executed inline otherwise. The exec* helpers encapsulate that branching.
func (p *awsProvider) authorizeRoleIdentityCenter(
	task models.ProviderContext,
	req *models.AuthorizeRoleRequest,
	targetAccountID string,
) (*models.AuthorizeRoleResponse, error) {

	// Step 1 — resolve the Identity Center instance
	instanceResp, err := p.execGetIdentityCenterInstance(task, &GetIdentityCenterInstanceRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to find Identity Center instance: %w", err)
	}

	// Step 2 — find or create the permission set for the role
	psResp, err := p.execFindOrCreatePermissionSet(task, &FindOrCreatePermissionSetRequest{
		InstanceArn: instanceResp.InstanceArn,
		Role:        req.GetRole(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find or create permission set: %w", err)
	}

	// Step 2.5 — provision permission set to propagate any policy changes to all accounts.
	// AWS requires ProvisionPermissionSet after PutInlinePolicyToPermissionSet or
	// AttachManagedPolicyToPermissionSet, otherwise updates silently don't propagate.
	psProvisionResp, err := p.execProvisionPermissionSet(task, &ProvisionPermissionSetRequest{
		InstanceArn:      instanceResp.InstanceArn,
		PermissionSetArn: psResp.PermissionSetArn,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to provision permission set: %w", err)
	}
	// Poll until provisioning completes. Check first (may already be done for new sets
	// with no prior accounts), then sleep-and-retry if still in progress.
	provCheckReq := &CheckPermissionSetProvisioningStatusRequest{
		InstanceArn: instanceResp.InstanceArn,
		RequestId:   psProvisionResp.RequestId,
	}
	provFirstCheck, err := p.execCheckPermissionSetProvisioningStatus(task, provCheckReq)
	if err != nil {
		return nil, fmt.Errorf("failed to check permission set provisioning status: %w", err)
	}
	if !provFirstCheck.Succeeded {
		if task.HasTemporalContext() {
			for {
				if err := workflow.Sleep(task.GetTemporalContext(), awsProviderDeleteRoleAssignmentBackoffDuration); err != nil {
					return nil, fmt.Errorf("workflow sleep cancelled while waiting for permission set provisioning: %w", err)
				}
				provCheckResp, err := p.execCheckPermissionSetProvisioningStatus(task, provCheckReq)
				if err != nil {
					return nil, err
				}
				if provCheckResp.Succeeded {
					break
				}
			}
		} else {
			provisioned := false
			for iter := 0; iter < awsProviderDeleteRoleAssignmentBackoffLimit; iter++ {
				if err := task.GetContext().Err(); err != nil {
					return nil, fmt.Errorf("context cancelled while waiting for permission set provisioning: %w", err)
				}
				time.Sleep(awsProviderDeleteRoleAssignmentBackoffDuration)
				provCheckResp, err := p.execCheckPermissionSetProvisioningStatus(task, provCheckReq)
				if err != nil {
					return nil, err
				}
				if provCheckResp.Succeeded {
					provisioned = true
					break
				}
			}
			if !provisioned {
				return nil, fmt.Errorf("timed out waiting for permission set %s to provision", psResp.PermissionSetArn)
			}
		}
	}

	// Step 3 — find the user in Identity Center
	userResp, err := p.execFindIdentityCenterUser(task, &FindIdentityCenterUserRequest{
		IdentityStoreId: instanceResp.IdentityStoreId,
		Email:           req.GetUser().Email,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find user in Identity Center: %w", err)
	}

	// Step 4 — create the account assignment
	if err := p.execCreateAccountAssignment(task, &CreateAccountAssignmentRequest{
		InstanceArn:      instanceResp.InstanceArn,
		PermissionSetArn: psResp.PermissionSetArn,
		PrincipalId:      userResp.PrincipalId,
		TargetAccountID:  targetAccountID,
	}); err != nil {
		return nil, fmt.Errorf("failed to create account assignment: %w", err)
	}

	return &models.AuthorizeRoleResponse{
		Metadata: map[string]any{
			"instanceArn":      instanceResp.InstanceArn,
			"permissionSetArn": psResp.PermissionSetArn,
			"principalId":      userResp.PrincipalId,
			"accountId":        targetAccountID,
		},
	}, nil
}

// getIdentityCenterInstance finds the Identity Center instance ARN and identity store ID.
// AWS Organizations typically have a single Identity Center instance per region.
func (p *awsProvider) getIdentityCenterInstance(ctx context.Context) (instanceArn string, identityStoreId string, err error) {
	resp, err := p.ssoAdminService.ListInstances(ctx, &ssoadmin.ListInstancesInput{})
	if err != nil {
		return "", "", fmt.Errorf("failed to list Identity Center instances in region %s: %w", p.GetRegion(), err)
	}

	if len(resp.Instances) == 0 {
		return "", "", fmt.Errorf("no Identity Center instances found in region: %s", p.GetRegion())
	}

	instance := resp.Instances[0]
	if instance.InstanceArn == nil {
		return "", "", fmt.Errorf("Identity Center instance ARN is nil in region: %s", p.GetRegion())
	}
	if instance.IdentityStoreId == nil {
		return "", "", fmt.Errorf("Identity Center identity store ID is nil in region: %s", p.GetRegion())
	}

	return *instance.InstanceArn, *instance.IdentityStoreId, nil
}

// findOrCreatePermissionSet finds an existing permission set by name, or creates a new one
// with the specified role's policies and permissions. Uses pagination to search all permission sets.
func (p *awsProvider) findOrCreatePermissionSet(ctx context.Context, instanceArn string, role *models.Role) (string, error) {
	permissionSetName := role.GetIdentifier()

	// Search existing permission sets with pagination
	var nextToken *string
	for {
		resp, err := p.ssoAdminService.ListPermissionSets(ctx, &ssoadmin.ListPermissionSetsInput{
			InstanceArn: aws.String(instanceArn),
			NextToken:   nextToken,
		})
		if err != nil {
			logrus.WithError(err).Error("Failed to list permission sets in Identity Center")
			return "", fmt.Errorf("failed to list permission sets: %w", err)
		}

		for _, permissionSetArn := range resp.PermissionSets {
			desc, err := p.ssoAdminService.DescribePermissionSet(ctx, &ssoadmin.DescribePermissionSetInput{
				InstanceArn:      aws.String(instanceArn),
				PermissionSetArn: aws.String(permissionSetArn),
			})
			if err != nil {
				logrus.WithError(err).WithField("permissionSetArn", permissionSetArn).Error("Failed to describe permission set")
				continue
			}

			if desc.PermissionSet == nil || desc.PermissionSet.Name == nil {
				continue
			}

			if *desc.PermissionSet.Name == permissionSetName {
				// Permission set exists, ensure it has the required policies attached

				// Attach inline permissions if any
				if len(role.Permissions.Allow) > 0 || len(role.Permissions.Deny) > 0 {
					err = p.attachPermissionsToPermissionSet(ctx, instanceArn, permissionSetArn, role.Permissions)
					if err != nil {
						logrus.WithError(err).WithField("permissionSetArn", permissionSetArn).Error("Failed to attach permissions to existing permission set")
						return "", fmt.Errorf("failed to attach permissions to existing permission set: %w", err)
					}
				}

				// Attach managed policies from role.Inherits
				if len(role.Inherits) > 0 {
					err = p.attachManagedPoliciesToPermissionSet(ctx, instanceArn, permissionSetArn, role.Inherits)
					if err != nil {
						return "", fmt.Errorf("failed to attach managed policies to existing permission set: %w", err)
					}
				}

				return permissionSetArn, nil
			}
		}

		if resp.NextToken == nil {
			break
		}
		nextToken = resp.NextToken
	}

	// Create new permission set
	createResp, err := p.ssoAdminService.CreatePermissionSet(ctx, &ssoadmin.CreatePermissionSetInput{
		InstanceArn:     aws.String(instanceArn),
		Name:            aws.String(permissionSetName),
		Description:     aws.String(role.Description),
		SessionDuration: aws.String(p.GetConfig().GetStringWithDefault("session_duration", "PT8H")),
	})
	if err != nil {
		return "", fmt.Errorf("failed to create permission set: %w", err)
	}

	permissionSetArn := *createResp.PermissionSet.PermissionSetArn

	// Create inline policy for the permission set using permissions
	if len(role.Permissions.Allow) > 0 || len(role.Permissions.Deny) > 0 {
		err = p.attachPermissionsToPermissionSet(ctx, instanceArn, permissionSetArn, role.Permissions)
		if err != nil {
			return "", fmt.Errorf("failed to attach permissions to permission set: %w", err)
		}
	}

	// Attach managed policies from role.Inherits
	if len(role.Inherits) > 0 {
		err = p.attachManagedPoliciesToPermissionSet(ctx, instanceArn, permissionSetArn, role.Inherits)
		if err != nil {
			return "", fmt.Errorf("failed to attach managed policies to permission set: %w", err)
		}
	}

	return permissionSetArn, nil
}

// attachPermissionsToPermissionSet creates an inline policy for the permission set
func (p *awsProvider) attachPermissionsToPermissionSet(ctx context.Context, instanceArn, permissionSetArn string, permissions models.RolePermissions) error {
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

	_, err = p.ssoAdminService.PutInlinePolicyToPermissionSet(ctx, &ssoadmin.PutInlinePolicyToPermissionSetInput{
		InstanceArn:      aws.String(instanceArn),
		PermissionSetArn: aws.String(permissionSetArn),
		InlinePolicy:     aws.String(string(policyDocumentJSON)),
	})
	if err != nil {
		return fmt.Errorf("failed to put inline policy to permission set: %w", err)
	}

	return nil
}

// attachManagedPoliciesToPermissionSet attaches AWS managed policies to the permission set
func (p *awsProvider) attachManagedPoliciesToPermissionSet(ctx context.Context, instanceArn, permissionSetArn string, inherits []string) error {
	for _, arnOrPolicy := range inherits {
		// Handle different types of ARNs that could be in role.inherits
		if strings.HasPrefix(arnOrPolicy, "arn:aws:iam::") {
			if strings.Contains(arnOrPolicy, ":role/") {
				// This is a role ARN - we cannot directly attach roles to permission sets
				// Log a warning and skip this entry
				logrus.WithField("roleArn", arnOrPolicy).Warn("Cannot attach IAM role directly to permission set - skipping. Consider using managed policy ARNs instead.")
				continue
			} else if strings.Contains(arnOrPolicy, ":policy/") {
				// This is a policy ARN - proceed with attachment
				err := p.attachPolicyToPermissionSet(ctx, instanceArn, permissionSetArn, arnOrPolicy)
				if err != nil {
					return fmt.Errorf("failed to attach policy %s to permission set: %w", arnOrPolicy, err)
				}
			} else {
				return fmt.Errorf("unsupported ARN type in role.inherits: %s", arnOrPolicy)
			}
		} else {
			// Assume it's a managed policy name (like "ReadOnlyAccess") and convert to full ARN
			managedPolicyArn := fmt.Sprintf("arn:aws:iam::aws:policy/%s", arnOrPolicy)
			err := p.attachPolicyToPermissionSet(ctx, instanceArn, permissionSetArn, managedPolicyArn)
			if err != nil {
				return fmt.Errorf("failed to attach managed policy %s to permission set: %w", managedPolicyArn, err)
			}
		}
	}

	return nil
}

// attachPolicyToPermissionSet attaches a single policy ARN to the permission set
func (p *awsProvider) attachPolicyToPermissionSet(ctx context.Context, instanceArn, permissionSetArn, policyArn string) error {
	// Validate that the ARN looks like a valid AWS policy ARN
	if !strings.HasPrefix(policyArn, "arn:aws:iam::") || !strings.Contains(policyArn, ":policy/") {
		return fmt.Errorf("invalid AWS policy ARN format: %s", policyArn)
	}

	// Check if it's an AWS managed policy (contains ":aws:") or customer managed policy
	if strings.Contains(policyArn, ":aws:iam::aws:policy/") {
		// AWS managed policy - check if already attached first
		isAlreadyAttached, err := p.isManagedPolicyAttached(ctx, instanceArn, permissionSetArn, policyArn)
		if err != nil {
			return fmt.Errorf("failed to check if managed policy is already attached: %w", err)
		}

		if isAlreadyAttached {
			logrus.WithFields(logrus.Fields{
				"policyArn":        policyArn,
				"permissionSetArn": permissionSetArn,
			}).Info("AWS managed policy is already attached to permission set - skipping")
			return nil
		}

		_, err = p.ssoAdminService.AttachManagedPolicyToPermissionSet(ctx, &ssoadmin.AttachManagedPolicyToPermissionSetInput{
			InstanceArn:      aws.String(instanceArn),
			PermissionSetArn: aws.String(permissionSetArn),
			ManagedPolicyArn: aws.String(policyArn),
		})
		if err != nil {
			return fmt.Errorf("failed to attach AWS managed policy %s: %w", policyArn, err)
		}

		logrus.WithFields(logrus.Fields{
			"policyArn":        policyArn,
			"permissionSetArn": permissionSetArn,
		}).Info("Successfully attached AWS managed policy to permission set")

	} else {
		// Customer managed policy - parse ARN to extract account ID and policy name
		parsed, err := arn.Parse(policyArn)
		if err != nil {
			return fmt.Errorf("invalid customer managed policy ARN: %w", err)
		}

		policyName := strings.TrimPrefix(parsed.Resource, "policy/")

		// Check if customer managed policy is already attached
		isAlreadyAttached, err := p.isCustomerManagedPolicyAttached(ctx, instanceArn, permissionSetArn, policyName)
		if err != nil {
			return fmt.Errorf("failed to check if customer managed policy is already attached: %w", err)
		}

		if isAlreadyAttached {
			logrus.WithFields(logrus.Fields{
				"policyName":       policyName,
				"accountId":        parsed.AccountID,
				"permissionSetArn": permissionSetArn,
			}).Info("Customer managed policy is already attached to permission set - skipping")
			return nil
		}

		_, err = p.ssoAdminService.AttachCustomerManagedPolicyReferenceToPermissionSet(ctx, &ssoadmin.AttachCustomerManagedPolicyReferenceToPermissionSetInput{
			InstanceArn:      aws.String(instanceArn),
			PermissionSetArn: aws.String(permissionSetArn),
			CustomerManagedPolicyReference: &types.CustomerManagedPolicyReference{
				Name: aws.String(policyName),
				Path: aws.String("/"),
			},
		})
		if err != nil {
			return fmt.Errorf("failed to attach customer managed policy %s (account: %s): %w", policyName, parsed.AccountID, err)
		}

		logrus.WithFields(logrus.Fields{
			"policyName":       policyName,
			"accountId":        parsed.AccountID,
			"permissionSetArn": permissionSetArn,
		}).Info("Successfully attached customer managed policy to permission set")
	}

	return nil
}

// isManagedPolicyAttached checks if a managed policy is already attached to a permission set.
// Paginates through all attached policies to handle sets with more than one page.
func (p *awsProvider) isManagedPolicyAttached(ctx context.Context, instanceArn, permissionSetArn, policyArn string) (bool, error) {
	var nextToken *string
	for {
		resp, err := p.ssoAdminService.ListManagedPoliciesInPermissionSet(ctx, &ssoadmin.ListManagedPoliciesInPermissionSetInput{
			InstanceArn:      aws.String(instanceArn),
			PermissionSetArn: aws.String(permissionSetArn),
			NextToken:        nextToken,
		})
		if err != nil {
			return false, fmt.Errorf("failed to list managed policies in permission set: %w", err)
		}
		for _, ap := range resp.AttachedManagedPolicies {
			if ap.Arn != nil && *ap.Arn == policyArn {
				return true, nil
			}
		}
		if resp.NextToken == nil {
			break
		}
		nextToken = resp.NextToken
	}
	return false, nil
}

// isCustomerManagedPolicyAttached checks if a customer managed policy is already attached to a permission set.
// Paginates through all references to handle sets with more than one page.
func (p *awsProvider) isCustomerManagedPolicyAttached(ctx context.Context, instanceArn, permissionSetArn, policyName string) (bool, error) {
	var nextToken *string
	for {
		resp, err := p.ssoAdminService.ListCustomerManagedPolicyReferencesInPermissionSet(ctx, &ssoadmin.ListCustomerManagedPolicyReferencesInPermissionSetInput{
			InstanceArn:      aws.String(instanceArn),
			PermissionSetArn: aws.String(permissionSetArn),
			NextToken:        nextToken,
		})
		if err != nil {
			return false, fmt.Errorf("failed to list customer managed policies in permission set: %w", err)
		}
		for _, ap := range resp.CustomerManagedPolicyReferences {
			if ap.Name != nil && *ap.Name == policyName {
				return true, nil
			}
		}
		if resp.NextToken == nil {
			break
		}
		nextToken = resp.NextToken
	}
	return false, nil
}

// findIdentityCenterUser finds a user in Identity Center by email.
// It first searches by userName, then falls back to the emails.value attribute.
func (p *awsProvider) findIdentityCenterUser(ctx context.Context, identityStoreId string, email string) (string, error) {
	// Search for user by userName
	usersResp, err := p.identityStoreClient.ListUsers(ctx, &identitystore.ListUsersInput{
		IdentityStoreId: aws.String(identityStoreId),
		Filters: []identitystoretypes.Filter{
			{
				AttributePath:  aws.String("userName"),
				AttributeValue: aws.String(email),
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to search for user by userName: %w", err)
	}

	if len(usersResp.Users) == 0 {
		// Fallback: search by email attribute
		usersResp, err = p.identityStoreClient.ListUsers(ctx, &identitystore.ListUsersInput{
			IdentityStoreId: aws.String(identityStoreId),
			Filters: []identitystoretypes.Filter{
				{
					AttributePath:  aws.String("emails.value"),
					AttributeValue: aws.String(email),
				},
			},
		})
		if err != nil {
			return "", fmt.Errorf("failed to search for user by email attribute: %w", err)
		}

		if len(usersResp.Users) == 0 {
			return "", fmt.Errorf("user with email %s not found in Identity Center", email)
		}
	}

	if usersResp.Users[0].UserId == nil {
		return "", fmt.Errorf("user ID is nil for user with email %s in Identity Center", email)
	}

	return *usersResp.Users[0].UserId, nil
}

// createAccountAssignment assigns a permission set to a user for the target account.
// If the assignment already exists (ConflictException), it is treated as success.
func (p *awsProvider) createAccountAssignment(ctx context.Context, instanceArn, permissionSetArn, principalId, targetAccountID string) error {

	assignmentOutput, err := p.ssoAdminService.CreateAccountAssignment(ctx, &ssoadmin.CreateAccountAssignmentInput{
		InstanceArn:      aws.String(instanceArn),
		PermissionSetArn: aws.String(permissionSetArn),
		PrincipalId:      aws.String(principalId),
		PrincipalType:    types.PrincipalTypeUser,
		TargetId:         aws.String(targetAccountID),
		TargetType:       types.TargetTypeAwsAccount,
	})

	if err != nil {
		// Conflict = assignment already exists; treat as success (idempotent).
		var conflictErr *types.ConflictException
		if errors.As(err, &conflictErr) {
			logrus.WithFields(logrus.Fields{
				"principalId":      principalId,
				"targetAccountID":  targetAccountID,
				"permissionSetArn": permissionSetArn,
			}).Info("Account assignment already exists - treating as success")
			return nil
		}
		return fmt.Errorf("failed to create account assignment: %w", err)
	}

	if assignmentOutput != nil &&
		assignmentOutput.AccountAssignmentCreationStatus != nil &&
		assignmentOutput.AccountAssignmentCreationStatus.PrincipalId != nil {
		logrus.WithFields(logrus.Fields{
			"principalId":     *assignmentOutput.AccountAssignmentCreationStatus.PrincipalId,
			"targetAccountID": targetAccountID,
		}).Info("Created account assignment")
	} else {
		logrus.WithFields(logrus.Fields{
			"principalId":     principalId,
			"targetAccountID": targetAccountID,
		}).Info("Created account assignment")
	}

	return nil
}

// revokeRoleIdentityCenter removes role authorization for Identity Center users.
// Each step is dispatched as a Temporal activity when a workflow context is present,
// or executed inline otherwise. The backoff polling loop sleeps via workflow.Sleep
// on the Temporal path (deterministic, replay-safe) and time.Sleep otherwise.
func (p *awsProvider) revokeRoleIdentityCenter(task models.ProviderContext, req *models.RevokeRoleRequest, targetAccountID string) error {

	user := req.GetUser()
	role := req.GetRole()

	// Step 1 — resolve the Identity Center instance
	instanceResp, err := p.execGetIdentityCenterInstance(task, &GetIdentityCenterInstanceRequest{})
	if err != nil {
		return fmt.Errorf("failed to find Identity Center instance: %w", err)
	}

	// Step 2 — resolve the permission set ARN (stored metadata or name-based lookup)
	var storedPermissionSetArn string
	if req.AuthorizeRoleResponse != nil && req.AuthorizeRoleResponse.Metadata != nil {
		if v, ok := req.AuthorizeRoleResponse.Metadata["permissionSetArn"].(string); ok && len(v) > 0 {
			storedPermissionSetArn = v
			logrus.WithFields(logrus.Fields{
				"permissionSetArn": storedPermissionSetArn,
				"principalId":      user.Email,
				"accountId":        targetAccountID,
			}).Info("Using stored permission set ARN from authorization metadata")
		}
	}
	if len(storedPermissionSetArn) == 0 {
		logrus.WithFields(logrus.Fields{
			"roleName":  role.GetIdentifier(),
			"accountId": targetAccountID,
		}).Warn("No permission set ARN in metadata, falling back to name-based lookup")
	}
	psResp, err := p.execFindPermissionSetByName(task, &FindPermissionSetByNameRequest{
		InstanceArn:      instanceResp.InstanceArn,
		PermissionSetArn: storedPermissionSetArn,
		RoleName:         role.GetIdentifier(),
	})
	if err != nil {
		return fmt.Errorf("failed to find permission set: %w in region: %s", err, p.GetRegion())
	}

	// Step 3 — find the user in Identity Center
	userResp, err := p.execFindIdentityCenterUser(task, &FindIdentityCenterUserRequest{
		IdentityStoreId: instanceResp.IdentityStoreId,
		Email:           user.Email,
	})
	if err != nil {
		return fmt.Errorf("failed to find user in Identity Center: %w", err)
	}

	// Step 4 — delete the account assignment
	deleteResp, err := p.execDeleteAccountAssignment(task, &DeleteAccountAssignmentRequest{
		InstanceArn:      instanceResp.InstanceArn,
		PermissionSetArn: psResp.PermissionSetArn,
		PrincipalId:      userResp.PrincipalId,
		TargetAccountID:  targetAccountID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete account assignment: %w", err)
	}

	cleanupReq := &CleanupPermissionSetRequest{
		InstanceArn:      instanceResp.InstanceArn,
		PermissionSetArn: psResp.PermissionSetArn,
	}
	if deleteResp.RequestId == "" {
		// Already revoked (idempotent); skip polling but still attempt cleanup.
		p.execCleanupPermissionSet(task, cleanupReq)
		return nil
	}

	// Step 5 — poll for deletion confirmation.
	// Temporal path: loop indefinitely — workflow.Sleep is replay-safe and the
	// workflow's own execution timeout is the natural bound. No iteration limit needed.
	// Direct path: bounded by awsProviderDeleteRoleAssignmentBackoffLimit.
	checkReq := &CheckAssignmentDeletionStatusRequest{
		InstanceArn:     instanceResp.InstanceArn,
		RequestId:       deleteResp.RequestId,
		PrincipalId:     userResp.PrincipalId,
		TargetAccountID: targetAccountID,
	}

	if task.HasTemporalContext() {
		for {
			if err := workflow.Sleep(task.GetTemporalContext(), awsProviderDeleteRoleAssignmentBackoffDuration); err != nil {
				return fmt.Errorf("workflow sleep cancelled while waiting for account assignment deletion: %w", err)
			}
			checkResp, err := p.execCheckAssignmentDeletionStatus(task, checkReq)
			if err != nil {
				return err
			}
			if checkResp.Succeeded {
				p.execCleanupPermissionSet(task, cleanupReq)
				return nil
			}
		}
	}

	for iter := 0; iter < awsProviderDeleteRoleAssignmentBackoffLimit; iter++ {
		if err := task.GetContext().Err(); err != nil {
			return fmt.Errorf("context cancelled while waiting for account assignment deletion: %w", err)
		}
		time.Sleep(awsProviderDeleteRoleAssignmentBackoffDuration)
		checkResp, err := p.execCheckAssignmentDeletionStatus(task, checkReq)
		if err != nil {
			return err
		}
		if checkResp.Succeeded {
			p.execCleanupPermissionSet(task, cleanupReq)
			return nil
		}
	}

	return fmt.Errorf(
		"timed out waiting for account assignment deletion for principalId %s in account %s",
		userResp.PrincipalId,
		targetAccountID,
	)
}

// deleteAccountAssignment calls the AWS DeleteAccountAssignment API.
// Returns (requestId, nil) on success, ("", nil) if already deleted (idempotent),
// or ("", error) on unexpected failure.
func (p *awsProvider) deleteAccountAssignment(
	ctx context.Context,
	instanceArn, permissionSetArn, principalId, targetAccountID string,
) (string, error) {
	deleteOutput, err := p.ssoAdminService.DeleteAccountAssignment(ctx, &ssoadmin.DeleteAccountAssignmentInput{
		InstanceArn:      aws.String(instanceArn),
		PermissionSetArn: aws.String(permissionSetArn),
		PrincipalId:      aws.String(principalId),
		PrincipalType:    types.PrincipalTypeUser,
		TargetId:         aws.String(targetAccountID),
		TargetType:       types.TargetTypeAwsAccount,
	})

	if err != nil {
		// Not found = already deleted; treat as success (idempotent).
		var notFoundErr *types.ResourceNotFoundException
		if errors.As(err, &notFoundErr) {
			logrus.WithFields(logrus.Fields{
				"permissionSetArn": permissionSetArn,
				"principalId":      principalId,
				"accountId":        targetAccountID,
			}).Info("Account assignment not found - already revoked or never existed, treating as success")
			return "", nil
		}
		return "", fmt.Errorf("failed to delete account assignment: %w", err)
	}

	if deleteOutput == nil ||
		deleteOutput.AccountAssignmentDeletionStatus == nil ||
		deleteOutput.AccountAssignmentDeletionStatus.RequestId == nil {
		return "", fmt.Errorf("account assignment deletion request ID is nil")
	}

	return *deleteOutput.AccountAssignmentDeletionStatus.RequestId, nil
}

// checkAssignmentDeletionStatus describes the deletion status for a single iteration.
// Returns (true, nil) on Succeeded, (false, nil) on InProgress, and (false, err)
// on Failed or unexpected status.
func (p *awsProvider) checkAssignmentDeletionStatus(
	ctx context.Context,
	instanceArn, requestId, principalId, targetAccountID string,
) (bool, error) {
	statusOutput, err := p.ssoAdminService.DescribeAccountAssignmentDeletionStatus(
		ctx, &ssoadmin.DescribeAccountAssignmentDeletionStatusInput{
			InstanceArn:                        aws.String(instanceArn),
			AccountAssignmentDeletionRequestId: aws.String(requestId),
		})
	if err != nil {
		return false, fmt.Errorf("failed to describe account assignment deletion status: %w", err)
	}

	var statusOutputPrincipalId, statusOutputFailureReason, statusOutputStatus string
	if statusOutput != nil && statusOutput.AccountAssignmentDeletionStatus != nil {
		if statusOutput.AccountAssignmentDeletionStatus.PrincipalId != nil {
			statusOutputPrincipalId = *statusOutput.AccountAssignmentDeletionStatus.PrincipalId
		}
		if statusOutput.AccountAssignmentDeletionStatus.FailureReason != nil {
			statusOutputFailureReason = *statusOutput.AccountAssignmentDeletionStatus.FailureReason
		}
		statusOutputStatus = string(statusOutput.AccountAssignmentDeletionStatus.Status)
	}

	switch statusOutputStatus {
	case string(types.StatusValuesFailed):
		logrus.WithFields(logrus.Fields{
			"principalId":     statusOutputPrincipalId,
			"targetAccountID": targetAccountID,
			"failureReason":   statusOutputFailureReason,
		}).Errorf(
			"account assignment deletion failed for principalId %s in account %s",
			statusOutputPrincipalId,
			targetAccountID,
		)
		return false, fmt.Errorf(
			"account assignment deletion failed for principalId %s in account %s",
			statusOutputPrincipalId,
			targetAccountID,
		)
	case string(types.StatusValuesInProgress):
		return false, nil
	case string(types.StatusValuesSucceeded):
		logrus.WithFields(logrus.Fields{
			"principalId":     statusOutputPrincipalId,
			"targetAccountID": targetAccountID,
		}).Info("Account assignment deletion succeeded")
		return true, nil
	default:
		return false, fmt.Errorf("unknown status value %s", statusOutputStatus)
	}
}

// tryCleanupPermissionSet attempts to clean up a permission set after account assignment deletion.
// Errors are logged but never propagated — the primary revocation has already succeeded.
func (p *awsProvider) tryCleanupPermissionSet(ctx context.Context, instanceArn, permissionSetArn string) {
	err := p.cleanupPermissionSetIfUnused(ctx, instanceArn, permissionSetArn)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"permissionSetArn": permissionSetArn,
			"error":            err.Error(),
		}).Warn("Failed to clean up permission set, but account assignment was successfully deleted")
	}
}

// cleanupPermissionSetIfUnused deletes a permission set if it has no remaining account
// assignments across the entire organization. This prevents dangling permission sets
// from accumulating after revocations.
func (p *awsProvider) cleanupPermissionSetIfUnused(ctx context.Context, instanceArn, permissionSetArn string) error {
	// Check if there are any remaining account assignments for this permission set across all accounts
	inUse, err := p.isPermissionSetInUse(ctx, instanceArn, permissionSetArn)
	if err != nil {
		return fmt.Errorf("failed to check for remaining account assignments: %w", err)
	}

	if inUse {
		logrus.WithFields(logrus.Fields{
			"permissionSetArn": permissionSetArn,
		}).Info("Permission set still has account assignments - skipping deletion")
		return nil
	}

	// No assignments remain, safe to delete the permission set
	logrus.WithFields(logrus.Fields{
		"permissionSetArn": permissionSetArn,
	}).Info("Deleting unused permission set")

	_, err = p.ssoAdminService.DeletePermissionSet(ctx, &ssoadmin.DeletePermissionSetInput{
		InstanceArn:      aws.String(instanceArn),
		PermissionSetArn: aws.String(permissionSetArn),
	})
	if err != nil {
		// Already deleted — treat as success.
		var notFoundErr *types.ResourceNotFoundException
		if errors.As(err, &notFoundErr) {
			logrus.WithFields(logrus.Fields{
				"permissionSetArn": permissionSetArn,
			}).Info("Permission set already deleted")
			return nil
		}
		return fmt.Errorf("failed to delete permission set: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"permissionSetArn": permissionSetArn,
	}).Info("Successfully deleted permission set")

	return nil
}

// isPermissionSetInUse checks whether a permission set is provisioned to any account
// in the organization. Uses ListAccountsForProvisionedPermissionSet which provides
// an org-wide view, unlike ListAccountAssignments which requires a specific account ID.
func (p *awsProvider) isPermissionSetInUse(ctx context.Context, instanceArn, permissionSetArn string) (bool, error) {
	resp, err := p.ssoAdminService.ListAccountsForProvisionedPermissionSet(ctx, &ssoadmin.ListAccountsForProvisionedPermissionSetInput{
		InstanceArn:      aws.String(instanceArn),
		PermissionSetArn: aws.String(permissionSetArn),
	})
	if err != nil {
		// If we get an error, assume it's in use to be safe (avoid accidental deletion)
		logrus.WithFields(logrus.Fields{
			"permissionSetArn": permissionSetArn,
			"error":            err.Error(),
		}).Warn("Failed to list accounts for permission set, assuming it is still in use")
		return true, nil
	}

	return len(resp.AccountIds) > 0, nil
}

// provisionPermissionSet calls ProvisionPermissionSet with TargetType=ALL_PROVISIONED_ACCOUNTS
// to push any policy changes to every account the set is assigned to.
// Returns the async request ID used to poll provisioning status.
func (p *awsProvider) provisionPermissionSet(ctx context.Context, instanceArn, permissionSetArn string) (string, error) {
	resp, err := p.ssoAdminService.ProvisionPermissionSet(ctx, &ssoadmin.ProvisionPermissionSetInput{
		InstanceArn:      aws.String(instanceArn),
		PermissionSetArn: aws.String(permissionSetArn),
		TargetType:       types.ProvisionTargetTypeAllProvisionedAccounts,
	})
	if err != nil {
		return "", fmt.Errorf("failed to provision permission set: %w", err)
	}
	if resp.PermissionSetProvisioningStatus == nil || resp.PermissionSetProvisioningStatus.RequestId == nil {
		return "", fmt.Errorf("permission set provisioning response missing request ID")
	}
	return *resp.PermissionSetProvisioningStatus.RequestId, nil
}

// checkPermissionSetProvisioningStatus performs a single poll of the provisioning
// status. Returns (true, nil) on Succeeded, (false, nil) on InProgress, (false, err) on Failed.
func (p *awsProvider) checkPermissionSetProvisioningStatus(ctx context.Context, instanceArn, requestId string) (bool, error) {
	resp, err := p.ssoAdminService.DescribePermissionSetProvisioningStatus(ctx, &ssoadmin.DescribePermissionSetProvisioningStatusInput{
		InstanceArn:                     aws.String(instanceArn),
		ProvisionPermissionSetRequestId: aws.String(requestId),
	})
	if err != nil {
		return false, fmt.Errorf("failed to describe permission set provisioning status: %w", err)
	}
	if resp.PermissionSetProvisioningStatus == nil {
		return false, fmt.Errorf("permission set provisioning status response is nil")
	}

	status := string(resp.PermissionSetProvisioningStatus.Status)
	switch status {
	case string(types.StatusValuesFailed):
		reason := ""
		if resp.PermissionSetProvisioningStatus.FailureReason != nil {
			reason = *resp.PermissionSetProvisioningStatus.FailureReason
		}
		return false, fmt.Errorf("permission set provisioning failed: %s", reason)
	case string(types.StatusValuesInProgress):
		return false, nil
	case string(types.StatusValuesSucceeded):
		return true, nil
	default:
		return false, fmt.Errorf("unknown permission set provisioning status: %s", status)
	}
}

// findPermissionSetByName finds a permission set by name using paginated search.
// Returns the permission set ARN if found, or an error with diagnostic context.
func (p *awsProvider) findPermissionSetByName(ctx context.Context, instanceArn, name string) (string, error) {
	var nextToken *string
	searchedCount := 0

	for {
		resp, err := p.ssoAdminService.ListPermissionSets(ctx, &ssoadmin.ListPermissionSetsInput{
			InstanceArn: aws.String(instanceArn),
			NextToken:   nextToken,
		})
		if err != nil {
			return "", fmt.Errorf("failed to list permission sets: %w", err)
		}

		for _, permissionSetArn := range resp.PermissionSets {
			searchedCount++
			desc, err := p.ssoAdminService.DescribePermissionSet(ctx, &ssoadmin.DescribePermissionSetInput{
				InstanceArn:      aws.String(instanceArn),
				PermissionSetArn: aws.String(permissionSetArn),
			})
			if err != nil {
				continue
			}

			if desc.PermissionSet == nil || desc.PermissionSet.Name == nil {
				continue
			}

			if *desc.PermissionSet.Name == name {
				return permissionSetArn, nil
			}
		}

		if resp.NextToken == nil {
			break
		}
		nextToken = resp.NextToken
	}

	return "", fmt.Errorf(
		"permission set with name %s not found in region %s (searched %d permission sets in instance %s)",
		name,
		p.GetRegion(),
		searchedCount,
		instanceArn,
	)
}

// ───────────────────────────────────────────────────────────────────────────────
// Operation wrappers
//
// Each wrapper performs one AWS SSO operation. When a Temporal workflow context
// is present it dispatches the operation as an activity (deterministic, retryable,
// observable in the Temporal UI). Otherwise it executes the AWS call inline in
// the current goroutine. Both paths share the same request/response structs so
// there is no logic duplication between the two execution modes.
// ───────────────────────────────────────────────────────────────────────────────

func (p *awsProvider) execGetIdentityCenterInstance(
	task models.ProviderContext,
	req *GetIdentityCenterInstanceRequest,
) (*GetIdentityCenterInstanceResponse, error) {
	if task.HasTemporalContext() {
		wfCtx := workflow.WithActivityOptions(task.GetTemporalContext(), workflow.ActivityOptions{
			TaskQueue:           task.GetTaskQueue(),
			StartToCloseTimeout: 30 * time.Second,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		var resp GetIdentityCenterInstanceResponse
		if err := workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), GetIdentityCenterInstanceActivityName), req).Get(wfCtx, &resp); err != nil {
			return nil, err
		}
		return &resp, nil
	}
	instanceArn, identityStoreId, err := p.getIdentityCenterInstance(task.GetContext())
	if err != nil {
		return nil, err
	}
	return &GetIdentityCenterInstanceResponse{InstanceArn: instanceArn, IdentityStoreId: identityStoreId}, nil
}

func (p *awsProvider) execFindOrCreatePermissionSet(
	task models.ProviderContext,
	req *FindOrCreatePermissionSetRequest,
) (*FindOrCreatePermissionSetResponse, error) {
	if task.HasTemporalContext() {
		wfCtx := workflow.WithActivityOptions(task.GetTemporalContext(), workflow.ActivityOptions{
			TaskQueue:           task.GetTaskQueue(),
			StartToCloseTimeout: 2 * time.Minute,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		var resp FindOrCreatePermissionSetResponse
		if err := workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), FindOrCreatePermissionSetActivityName), req).Get(wfCtx, &resp); err != nil {
			return nil, err
		}
		return &resp, nil
	}
	arn, err := p.findOrCreatePermissionSet(task.GetContext(), req.InstanceArn, req.Role)
	if err != nil {
		return nil, err
	}
	return &FindOrCreatePermissionSetResponse{PermissionSetArn: arn}, nil
}

func (p *awsProvider) execFindIdentityCenterUser(
	task models.ProviderContext,
	req *FindIdentityCenterUserRequest,
) (*FindIdentityCenterUserResponse, error) {
	if task.HasTemporalContext() {
		wfCtx := workflow.WithActivityOptions(task.GetTemporalContext(), workflow.ActivityOptions{
			TaskQueue:           task.GetTaskQueue(),
			StartToCloseTimeout: 30 * time.Second,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		var resp FindIdentityCenterUserResponse
		if err := workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), FindIdentityCenterUserActivityName), req).Get(wfCtx, &resp); err != nil {
			return nil, err
		}
		return &resp, nil
	}
	principalId, err := p.findIdentityCenterUser(task.GetContext(), req.IdentityStoreId, req.Email)
	if err != nil {
		return nil, err
	}
	return &FindIdentityCenterUserResponse{PrincipalId: principalId}, nil
}

func (p *awsProvider) execCreateAccountAssignment(
	task models.ProviderContext,
	req *CreateAccountAssignmentRequest,
) error {
	if task.HasTemporalContext() {
		wfCtx := workflow.WithActivityOptions(task.GetTemporalContext(), workflow.ActivityOptions{
			TaskQueue:           task.GetTaskQueue(),
			StartToCloseTimeout: 2 * time.Minute,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		return workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), CreateAccountAssignmentActivityName), req).Get(wfCtx, nil)
	}
	return p.createAccountAssignment(task.GetContext(), req.InstanceArn, req.PermissionSetArn, req.PrincipalId, req.TargetAccountID)
}

func (p *awsProvider) execFindPermissionSetByName(
	task models.ProviderContext,
	req *FindPermissionSetByNameRequest,
) (*FindPermissionSetByNameResponse, error) {
	if task.HasTemporalContext() {
		wfCtx := workflow.WithActivityOptions(task.GetTemporalContext(), workflow.ActivityOptions{
			TaskQueue:           task.GetTaskQueue(),
			StartToCloseTimeout: 30 * time.Second,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		var resp FindPermissionSetByNameResponse
		if err := workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), FindPermissionSetByNameActivityName), req).Get(wfCtx, &resp); err != nil {
			return nil, err
		}
		return &resp, nil
	}
	// Direct path: short-circuit if the ARN is already known
	if len(req.PermissionSetArn) > 0 {
		return &FindPermissionSetByNameResponse{PermissionSetArn: req.PermissionSetArn}, nil
	}
	arn, err := p.findPermissionSetByName(task.GetContext(), req.InstanceArn, req.RoleName)
	if err != nil {
		return nil, err
	}
	return &FindPermissionSetByNameResponse{PermissionSetArn: arn}, nil
}

func (p *awsProvider) execDeleteAccountAssignment(
	task models.ProviderContext,
	req *DeleteAccountAssignmentRequest,
) (*DeleteAccountAssignmentResponse, error) {
	if task.HasTemporalContext() {
		wfCtx := workflow.WithActivityOptions(task.GetTemporalContext(), workflow.ActivityOptions{
			TaskQueue:           task.GetTaskQueue(),
			StartToCloseTimeout: 2 * time.Minute,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		var resp DeleteAccountAssignmentResponse
		if err := workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), DeleteAccountAssignmentActivityName), req).Get(wfCtx, &resp); err != nil {
			return nil, err
		}
		return &resp, nil
	}
	requestId, err := p.deleteAccountAssignment(task.GetContext(), req.InstanceArn, req.PermissionSetArn, req.PrincipalId, req.TargetAccountID)
	if err != nil {
		return nil, err
	}
	return &DeleteAccountAssignmentResponse{RequestId: requestId}, nil
}

func (p *awsProvider) execCheckAssignmentDeletionStatus(
	task models.ProviderContext,
	req *CheckAssignmentDeletionStatusRequest,
) (*CheckAssignmentDeletionStatusResponse, error) {
	if task.HasTemporalContext() {
		wfCtx := workflow.WithActivityOptions(task.GetTemporalContext(), workflow.ActivityOptions{
			TaskQueue:           task.GetTaskQueue(),
			StartToCloseTimeout: 30 * time.Second,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		var resp CheckAssignmentDeletionStatusResponse
		if err := workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), CheckAssignmentDeletionStatusActivityName), req).Get(wfCtx, &resp); err != nil {
			return nil, err
		}
		return &resp, nil
	}
	succeeded, err := p.checkAssignmentDeletionStatus(task.GetContext(), req.InstanceArn, req.RequestId, req.PrincipalId, req.TargetAccountID)
	if err != nil {
		return nil, err
	}
	return &CheckAssignmentDeletionStatusResponse{Succeeded: succeeded}, nil
}

// execCleanupPermissionSet attempts to clean up a permission set. Non-fatal on
// both paths — errors are swallowed and logged so a cleanup failure never
// blocks the primary revocation result.
func (p *awsProvider) execCleanupPermissionSet(
	task models.ProviderContext,
	req *CleanupPermissionSetRequest,
) {
	if task.HasTemporalContext() {
		wfCtx := workflow.WithActivityOptions(task.GetTemporalContext(), workflow.ActivityOptions{
			TaskQueue:           task.GetTaskQueue(),
			StartToCloseTimeout: 1 * time.Minute,
			RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 1},
		})
		_ = workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), CleanupPermissionSetActivityName), req).Get(wfCtx, nil)
		return
	}
	p.tryCleanupPermissionSet(task.GetContext(), req.InstanceArn, req.PermissionSetArn)
}

func (p *awsProvider) execProvisionPermissionSet(
	task models.ProviderContext,
	req *ProvisionPermissionSetRequest,
) (*ProvisionPermissionSetResponse, error) {
	if task.HasTemporalContext() {
		wfCtx := workflow.WithActivityOptions(task.GetTemporalContext(), workflow.ActivityOptions{
			TaskQueue:           task.GetTaskQueue(),
			StartToCloseTimeout: 30 * time.Second,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		var resp ProvisionPermissionSetResponse
		if err := workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), ProvisionPermissionSetActivityName), req).Get(wfCtx, &resp); err != nil {
			return nil, err
		}
		return &resp, nil
	}
	requestId, err := p.provisionPermissionSet(task.GetContext(), req.InstanceArn, req.PermissionSetArn)
	if err != nil {
		return nil, err
	}
	return &ProvisionPermissionSetResponse{RequestId: requestId}, nil
}

func (p *awsProvider) execCheckPermissionSetProvisioningStatus(
	task models.ProviderContext,
	req *CheckPermissionSetProvisioningStatusRequest,
) (*CheckPermissionSetProvisioningStatusResponse, error) {
	if task.HasTemporalContext() {
		wfCtx := workflow.WithActivityOptions(task.GetTemporalContext(), workflow.ActivityOptions{
			TaskQueue:           task.GetTaskQueue(),
			StartToCloseTimeout: 30 * time.Second,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		var resp CheckPermissionSetProvisioningStatusResponse
		if err := workflow.ExecuteActivity(wfCtx, models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), CheckPermissionSetProvisioningStatusActivityName), req).Get(wfCtx, &resp); err != nil {
			return nil, err
		}
		return &resp, nil
	}
	succeeded, err := p.checkPermissionSetProvisioningStatus(task.GetContext(), req.InstanceArn, req.RequestId)
	if err != nil {
		return nil, err
	}
	return &CheckPermissionSetProvisioningStatusResponse{Succeeded: succeeded}, nil
}
