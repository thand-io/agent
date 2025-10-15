package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
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
) (map[string]any, error) {

	user := req.GetUser()
	role := req.GetRole()

	// 1. Find the Identity Center instance
	instanceArn, err := p.getIdentityCenterInstance(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to find Identity Center instance: %w", err)
	}

	// 2. Find or create a Permission Set based on the role
	permissionSetArn, err := p.findOrCreatePermissionSet(ctx, instanceArn, role)
	if err != nil {
		return nil, fmt.Errorf("failed to find or create permission set: %w", err)
	}

	// 3. Find the user in Identity Center by email
	principalId, err := p.findIdentityCenterUser(ctx, user.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to find user in Identity Center: %w", err)
	}

	// 4. Create an Account Assignment
	err = p.createAccountAssignment(ctx, instanceArn, permissionSetArn, principalId)
	if err != nil {
		return nil, fmt.Errorf("failed to create account assignment: %w", err)
	}

	return map[string]any{
		"instanceArn":      instanceArn,
		"permissionSetArn": permissionSetArn,
		"principalId":      principalId,
		"accountId":        p.GetAccountID(),
	}, nil
}

// getIdentityCenterInstance finds the Identity Center instance ARN
func (p *awsProvider) getIdentityCenterInstance(ctx context.Context) (string, error) {
	resp, err := p.ssoAdminService.ListInstances(ctx, &ssoadmin.ListInstancesInput{})
	if err != nil {
		return "", fmt.Errorf("failed to list Identity Center instances: %w in region: %s", err, p.GetRegion())
	}

	if len(resp.Instances) == 0 {
		return "", fmt.Errorf("no Identity Center instances found in region: %s", p.GetRegion())
	}

	// Return the first instance (typically there's only one per organization)
	return *resp.Instances[0].InstanceArn, nil
}

// findOrCreatePermissionSet finds an existing permission set or creates a new one
func (p *awsProvider) findOrCreatePermissionSet(ctx context.Context, instanceArn string, role *models.Role) (string, error) {
	permissionSetName := role.GetSnakeCaseName()

	// First, try to find existing permission set
	resp, err := p.ssoAdminService.ListPermissionSets(ctx, &ssoadmin.ListPermissionSetsInput{
		InstanceArn: aws.String(instanceArn),
	})
	if err != nil {
		return "", fmt.Errorf("failed to list permission sets: %w", err)
	}

	// Check if permission set already exists
	for _, permissionSetArn := range resp.PermissionSets {
		desc, err := p.ssoAdminService.DescribePermissionSet(ctx, &ssoadmin.DescribePermissionSetInput{
			InstanceArn:      aws.String(instanceArn),
			PermissionSetArn: aws.String(permissionSetArn),
		})
		if err != nil {
			continue
		}

		if *desc.PermissionSet.Name == permissionSetName {
			// Permission set exists, ensure it has the required policies attached

			// Attach inline permissions if any
			if len(role.Permissions.Allow) > 0 {
				err = p.attachPermissionsToPermissionSet(ctx, instanceArn, permissionSetArn, role.Permissions.Allow)
				if err != nil {
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

	// Create inline policy for the permission set
	if len(role.Permissions.Allow) > 0 {
		err = p.attachPermissionsToPermissionSet(ctx, instanceArn, permissionSetArn, role.Permissions.Allow)
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
func (p *awsProvider) attachPermissionsToPermissionSet(ctx context.Context, instanceArn, permissionSetArn string, permissions []string) error {
	// Create a policy document
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
		// Customer managed policy - extract account ID and policy name from ARN
		// ARN format: arn:aws:iam::123456789012:policy/PolicyName
		arnParts := strings.Split(policyArn, ":")
		if len(arnParts) != 6 {
			return fmt.Errorf("invalid customer managed policy ARN format: %s", policyArn)
		}

		accountId := arnParts[4]
		policyPath := arnParts[5] // This is "policy/PolicyName"
		policyName := strings.TrimPrefix(policyPath, "policy/")

		// Check if customer managed policy is already attached
		isAlreadyAttached, err := p.isCustomerManagedPolicyAttached(ctx, instanceArn, permissionSetArn, policyName)
		if err != nil {
			return fmt.Errorf("failed to check if customer managed policy is already attached: %w", err)
		}

		if isAlreadyAttached {
			logrus.WithFields(logrus.Fields{
				"policyName":       policyName,
				"accountId":        accountId,
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
			return fmt.Errorf("failed to attach customer managed policy %s (account: %s): %w", policyName, accountId, err)
		}

		logrus.WithFields(logrus.Fields{
			"policyName":       policyName,
			"accountId":        accountId,
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

// findIdentityCenterUser finds a user in Identity Center by email
func (p *awsProvider) findIdentityCenterUser(ctx context.Context, email string) (string, error) {

	// First, get the identity store ID from the SSO instance
	resp, err := p.ssoAdminService.ListInstances(ctx, &ssoadmin.ListInstancesInput{})
	if err != nil {
		return "", fmt.Errorf("failed to list SSO instances: %w", err)
	}

	if len(resp.Instances) == 0 {
		return "", fmt.Errorf("no SSO instances found")
	}

	identityStoreId := resp.Instances[0].IdentityStoreId
	if identityStoreId == nil {
		return "", fmt.Errorf("identity store ID not found in SSO instance")
	}

	// Search for user by email
	usersResp, err := p.identityStoreClient.ListUsers(ctx, &identitystore.ListUsersInput{
		IdentityStoreId: identityStoreId,
		Filters: []identitystoretypes.Filter{
			{
				AttributePath:  aws.String("userName"),
				AttributeValue: aws.String(email),
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to search for user by email: %w", err)
	}

	if len(usersResp.Users) == 0 {
		// Try searching by email attribute as well
		usersResp, err = p.identityStoreClient.ListUsers(ctx, &identitystore.ListUsersInput{
			IdentityStoreId: identityStoreId,
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
	} // Return the first matching user's ID
	return *usersResp.Users[0].UserId, nil
}

// createAccountAssignment assigns a permission set to a user for the current account
func (p *awsProvider) createAccountAssignment(ctx context.Context, instanceArn, permissionSetArn, principalId string) error {

	assingmentOutput, err := p.ssoAdminService.CreateAccountAssignment(ctx, &ssoadmin.CreateAccountAssignmentInput{
		InstanceArn:      aws.String(instanceArn),
		PermissionSetArn: aws.String(permissionSetArn),
		PrincipalId:      aws.String(principalId),
		PrincipalType:    types.PrincipalTypeUser,
		TargetId:         aws.String(p.GetAccountID()),
		TargetType:       types.TargetTypeAwsAccount,
	})

	if err != nil {
		// Check if assignment already exists
		if strings.Contains(err.Error(), "ConflictException") {
			return nil // Assignment already exists, which is fine
		}
		return fmt.Errorf("failed to create account assignment: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"principalId": *assingmentOutput.AccountAssignmentCreationStatus.PrincipalId,
	}).Info("Created account assignment")

	return nil
}

// revokeRoleIdentityCenter removes role authorization for Identity Center users
func (p *awsProvider) revokeRoleIdentityCenter(ctx context.Context, user *models.User, role *models.Role) error {
	// 1. Find the Identity Center instance
	instanceArn, err := p.getIdentityCenterInstance(ctx)
	if err != nil {
		return fmt.Errorf("failed to find Identity Center instance: %w in region: %s", err, p.GetRegion())
	}

	// 2. Find the Permission Set
	permissionSetArn, err := p.findPermissionSetByName(ctx, instanceArn, role.GetSnakeCaseName())
	if err != nil {
		return fmt.Errorf("failed to find permission set: %w in region: %s", err, p.GetRegion())
	}

	// 3. Find the user in Identity Center
	principalId, err := p.findIdentityCenterUser(ctx, user.Email)
	if err != nil {
		return fmt.Errorf("failed to find user in Identity Center: %w in region: %s", err, p.GetRegion())
	}

	// 4. Delete the Account Assignment
	_, err = p.ssoAdminService.DeleteAccountAssignment(ctx, &ssoadmin.DeleteAccountAssignmentInput{
		InstanceArn:      aws.String(instanceArn),
		PermissionSetArn: aws.String(permissionSetArn),
		PrincipalId:      aws.String(principalId),
		PrincipalType:    types.PrincipalTypeUser,
		TargetId:         aws.String(p.GetAccountID()),
		TargetType:       types.TargetTypeAwsAccount,
	})

	if err != nil {
		return fmt.Errorf("failed to delete account assignment: %w", err)
	}

	return nil
}

// findPermissionSetByName finds a permission set by name
func (p *awsProvider) findPermissionSetByName(ctx context.Context, instanceArn, name string) (string, error) {
	resp, err := p.ssoAdminService.ListPermissionSets(ctx, &ssoadmin.ListPermissionSetsInput{
		InstanceArn: aws.String(instanceArn),
	})
	if err != nil {
		return "", fmt.Errorf("failed to list permission sets: %w", err)
	}

	for _, permissionSetArn := range resp.PermissionSets {
		desc, err := p.ssoAdminService.DescribePermissionSet(ctx, &ssoadmin.DescribePermissionSetInput{
			InstanceArn:      aws.String(instanceArn),
			PermissionSetArn: aws.String(permissionSetArn),
		})
		if err != nil {
			continue
		}

		if *desc.PermissionSet.Name == name {
			return permissionSetArn, nil
		}
	}

	return "", fmt.Errorf("permission set with name %s not found", name)
}
