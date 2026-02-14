package aws

import (
	"context"
	"encoding/json"
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
)

// authorizeRoleIdentityCenter handles role authorization for Identity Center users
func (p *awsProvider) authorizeRoleIdentityCenter(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
	targetAccountID string,
) (*models.AuthorizeRoleResponse, error) {

	user := req.GetUser()
	role := req.GetRole()

	// 1. Find the Identity Center instance
	instanceArn, identityStoreId, err := p.getIdentityCenterInstance(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to find Identity Center instance: %w", err)
	}

	// 2. Find or create a Permission Set based on the role
	permissionSetArn, err := p.findOrCreatePermissionSet(ctx, instanceArn, role)
	if err != nil {
		return nil, fmt.Errorf("failed to find or create permission set: %w", err)
	}

	// 3. Find the user in Identity Center by email
	principalId, err := p.findIdentityCenterUser(ctx, identityStoreId, user.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to find user in Identity Center: %w", err)
	}

	// 4. Create an Account Assignment
	err = p.createAccountAssignment(ctx, instanceArn, permissionSetArn, principalId, targetAccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to create account assignment: %w", err)
	}

	return &models.AuthorizeRoleResponse{
		Metadata: map[string]any{
			"instanceArn":      instanceArn,
			"permissionSetArn": permissionSetArn,
			"principalId":      principalId,
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
		SessionDuration: aws.String("PT8H"), // 8 hours
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

// isManagedPolicyAttached checks if a managed policy is already attached to a permission set
func (p *awsProvider) isManagedPolicyAttached(ctx context.Context, instanceArn, permissionSetArn, policyArn string) (bool, error) {
	// List managed policies attached to the permission set
	resp, err := p.ssoAdminService.ListManagedPoliciesInPermissionSet(ctx, &ssoadmin.ListManagedPoliciesInPermissionSetInput{
		InstanceArn:      aws.String(instanceArn),
		PermissionSetArn: aws.String(permissionSetArn),
	})
	if err != nil {
		return false, fmt.Errorf("failed to list managed policies in permission set: %w", err)
	}

	// Check if the policy ARN is in the list
	for _, attachedPolicy := range resp.AttachedManagedPolicies {
		if attachedPolicy.Arn != nil && *attachedPolicy.Arn == policyArn {
			return true, nil
		}
	}

	return false, nil
}

// isCustomerManagedPolicyAttached checks if a customer managed policy is already attached to a permission set
func (p *awsProvider) isCustomerManagedPolicyAttached(ctx context.Context, instanceArn, permissionSetArn, policyName string) (bool, error) {
	// List customer managed policies attached to the permission set
	resp, err := p.ssoAdminService.ListCustomerManagedPolicyReferencesInPermissionSet(ctx, &ssoadmin.ListCustomerManagedPolicyReferencesInPermissionSetInput{
		InstanceArn:      aws.String(instanceArn),
		PermissionSetArn: aws.String(permissionSetArn),
	})
	if err != nil {
		return false, fmt.Errorf("failed to list customer managed policies in permission set: %w", err)
	}

	// Check if the policy name is in the list
	for _, attachedPolicy := range resp.CustomerManagedPolicyReferences {
		if attachedPolicy.Name != nil && *attachedPolicy.Name == policyName {
			return true, nil
		}
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
		// Check if assignment already exists
		if strings.Contains(err.Error(), "ConflictException") {
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
// It deletes the account assignment and cleans up the permission set if no longer in use.
// Operations are idempotent — already-deleted resources are treated as success.
func (p *awsProvider) revokeRoleIdentityCenter(ctx context.Context, req *models.RevokeRoleRequest, targetAccountID string) error {
	user := req.GetUser()
	role := req.GetRole()

	// 1. Find the Identity Center instance
	instanceArn, identityStoreId, err := p.getIdentityCenterInstance(ctx)
	if err != nil {
		return fmt.Errorf("failed to find Identity Center instance: %w", err)
	}

	// 2. Find the Permission Set - try to use stored ARN from authorization metadata first
	var permissionSetArn string
	if req.AuthorizeRoleResponse != nil && req.AuthorizeRoleResponse.Metadata != nil {
		if arn, ok := req.AuthorizeRoleResponse.Metadata["permissionSetArn"].(string); ok && len(arn) > 0 {
			permissionSetArn = arn
			logrus.WithFields(logrus.Fields{
				"permissionSetArn": arn,
				"principalId":      user.Email,
				"accountId":        targetAccountID,
			}).Info("Using stored permission set ARN from authorization metadata")
		}
	}

	// Fallback to lookup by name if no metadata available (for legacy grants)
	if len(permissionSetArn) == 0 {
		logrus.WithFields(logrus.Fields{
			"roleName":  role.GetIdentifier(),
			"accountId": targetAccountID,
		}).Warn("No permission set ARN in metadata, falling back to name-based lookup")
		permissionSetArn, err = p.findPermissionSetByName(ctx, instanceArn, role.GetIdentifier())
		if err != nil {
			return fmt.Errorf("failed to find permission set: %w in region: %s", err, p.GetRegion())
		}
	}

	// 3. Find the user in Identity Center
	principalId, err := p.findIdentityCenterUser(ctx, identityStoreId, user.Email)
	if err != nil {
		return fmt.Errorf("failed to find user in Identity Center: %w", err)
	}

	// 4. Delete the Account Assignment
	deleteOutput, err := p.ssoAdminService.DeleteAccountAssignment(ctx, &ssoadmin.DeleteAccountAssignmentInput{
		InstanceArn:      aws.String(instanceArn),
		PermissionSetArn: aws.String(permissionSetArn),
		PrincipalId:      aws.String(principalId),
		PrincipalType:    types.PrincipalTypeUser,
		TargetId:         aws.String(targetAccountID),
		TargetType:       types.TargetTypeAwsAccount,
	})

	if err != nil {
		// Treat "not found" errors as success - already revoked (idempotent)
		if strings.Contains(err.Error(), "ResourceNotFoundException") ||
			strings.Contains(err.Error(), "NotFoundException") {
			logrus.WithFields(logrus.Fields{
				"permissionSetArn": permissionSetArn,
				"principalId":      principalId,
				"accountId":        targetAccountID,
			}).Info("Account assignment not found - already revoked or never existed, treating as success")
			// Still attempt to clean up the permission set
			p.tryCleanupPermissionSet(ctx, instanceArn, permissionSetArn)
			return nil
		}
		return fmt.Errorf("failed to delete account assignment: %w", err)
	}

	// 5. Lastly, poll to verify deletion
	var deleteOutputRequestId *string
	if deleteOutput != nil && deleteOutput.AccountAssignmentDeletionStatus != nil && deleteOutput.AccountAssignmentDeletionStatus.RequestId != nil {
		deleteOutputRequestId = deleteOutput.AccountAssignmentDeletionStatus.RequestId
	}

	if deleteOutputRequestId == nil {
		return fmt.Errorf("account assignment deletion request ID is nil")
	}

	// poll to verify deletion
	backoffDuration := awsProviderDeleteRoleAssignmentBackoffDuration
	backoffLimit := awsProviderDeleteRoleAssignmentBackoffLimit
	iter := 0
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context cancelled while waiting for account assignment deletion: %w", err)
		}
		if iter >= backoffLimit {
			return fmt.Errorf(
				"timed out waiting for account assignment deletion for principalId %s in account %s",
				principalId,
				targetAccountID,
			)
		}
		iter++
		time.Sleep(backoffDuration)
		statusOutput, err := p.ssoAdminService.DescribeAccountAssignmentDeletionStatus(
			ctx, &ssoadmin.DescribeAccountAssignmentDeletionStatusInput{
				InstanceArn:                        aws.String(instanceArn),
				AccountAssignmentDeletionRequestId: deleteOutputRequestId,
			})
		if err != nil {
			return fmt.Errorf("failed to describe account assignment deletion status: %w", err)
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
			return fmt.Errorf(
				"account assignment deletion failed for principalId %s in account %s",
				statusOutputPrincipalId,
				targetAccountID,
			)

		case string(types.StatusValuesInProgress):
			continue

		case string(types.StatusValuesSucceeded):
			logrus.WithFields(logrus.Fields{
				"principalId":     statusOutputPrincipalId,
				"targetAccountID": targetAccountID,
			}).Info("Account assignment deletion succeeded")

			// 6. Clean up the permission set if no longer in use across the organization
			p.tryCleanupPermissionSet(ctx, instanceArn, permissionSetArn)

			return nil
		default:
			return fmt.Errorf("unknown status value %s", statusOutputStatus)
		}
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
		// Check if it's already deleted
		if strings.Contains(err.Error(), "ResourceNotFoundException") ||
			strings.Contains(err.Error(), "NotFoundException") {
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
