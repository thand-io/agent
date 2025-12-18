package cloudflare

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

const CloudflareAllow = "allow"
const CloudflareDeny = "deny"

// AuthorizeRole grants access for a user to a role in Cloudflare
// Supports both account-wide roles and resource-scoped policies
func (p *cloudflareProvider) AuthorizeRole(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
) (*models.AuthorizeRoleResponse, error) {

	// Check for nil inputs
	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to authorize Cloudflare role")
	}

	user := req.GetUser()
	role := req.GetRole()

	if len(role.Resources.Allow) == 0 {
		return nil, fmt.Errorf("role must specify at least one resource in 'resources.allow' to authorize Cloudflare role")
	}

	logrus.WithFields(logrus.Fields{
		"user": user.Email,
		"role": role.Name,
	}).Info("Authorizing Cloudflare role")

	// Get the account resource container
	accountID := p.GetAccountID()
	accountRC := cloudflare.AccountIdentifier(accountID)

	params := cloudflare.CreateAccountMemberParams{
		EmailAddress: user.Email,
	}

	// Use policy-based RBAC for granular resource access
	// Map the role name to get permission group IDs, then build policies
	err := p.buildMembershipFromRole(ctx, &params, role)
	if err != nil {
		return nil, fmt.Errorf("failed to build policies: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"user":         user.Email,
		"role":         role.Name,
		"policy_count": len(params.Policies),
		"role_count":   len(params.Roles),
	}).Info("Creating account member with resource-scoped policies")

	// Check if the member already exists
	existingMember, err := p.findAccountMember(ctx, user.Email)
	if err == nil && existingMember != nil {
		// Member exists, update instead of create
		logrus.WithFields(logrus.Fields{
			"user":      user.Email,
			"member_id": existingMember.ID,
		}).Info("Member already exists, updating instead")

		existingMember.Roles = nil // Clear policies when assigning roles

		updatedMember, err := p.client.UpdateAccountMember(ctx, accountID, existingMember.ID, *existingMember)

		if err != nil {
			return nil, fmt.Errorf("failed to update account member: %w", err)
		}

		logrus.WithFields(logrus.Fields{
			"user":      user.Email,
			"role":      role.Name,
			"member_id": updatedMember.ID,
		}).Info("Successfully updated Cloudflare role")

		return &models.AuthorizeRoleResponse{
			Metadata: map[string]any{
				"member_id": updatedMember.ID,
				"status":    updatedMember.Status,
				"updated":   true,
			},
		}, nil
	}

	params.Roles = nil // Clear roles when assigning policies

	// Member doesn't exist, create new
	member, err := p.client.CreateAccountMember(ctx, accountRC, params)
	if err != nil {
		attempt := activity.GetInfo(ctx).Attempt
		return nil, temporal.NewApplicationErrorWithOptions(
			fmt.Sprintf("failed to create cloudflare account member on attempt %d", attempt),
			"CloudflareAccountMemberCreationError",
			temporal.ApplicationErrorOptions{
				NextRetryDelay: 3 * time.Second,
				Cause:          err,
			},
		)
	}

	logrus.WithFields(logrus.Fields{
		"user":      user.Email,
		"role":      role.Name,
		"member_id": member.ID,
	}).Info("Successfully authorized Cloudflare role")

	// Return metadata about the authorization
	return &models.AuthorizeRoleResponse{
		Metadata: map[string]any{
			"member_id": member.ID,
			"status":    member.Status,
		},
	}, nil
}

// RevokeRole removes access for a user from a role in Cloudflare
// Handles both account-wide roles and resource-scoped policies
func (p *cloudflareProvider) RevokeRole(
	ctx context.Context,
	req *models.RevokeRoleRequest,
) (*models.RevokeRoleResponse, error) {

	// Check for nil inputs
	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to revoke Cloudflare role")
	}

	user := req.GetUser()
	role := req.GetRole()

	logrus.WithFields(logrus.Fields{
		"user": user.Email,
		"role": role.Name,
	}).Info("Revoking Cloudflare role")

	// Get the member ID from the authorization metadata if available
	var memberID string
	if req.AuthorizeRoleResponse != nil && req.AuthorizeRoleResponse.Metadata != nil {
		if id, ok := req.AuthorizeRoleResponse.Metadata["member_id"].(string); ok {
			memberID = id
		}
	}

	// If we don't have the member ID, we need to look it up
	if len(memberID) == 0 {
		accountID := p.GetAccountID()

		// List account members to find the user
		members, _, err := p.client.AccountMembers(ctx, accountID, cloudflare.PaginationOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to list account members: %w", err)
		}

		for _, member := range members {
			if member.User.Email == user.Email {
				memberID = member.ID
				break
			}
		}

		if len(memberID) == 0 {
			return nil, fmt.Errorf("user %s not found in account members", user.Email)
		}
	}

	// Note: Removing the member removes ALL their access (both roles and policies)
	// If partial revocation is needed in the future, use UpdateAccountMember instead
	accountID := p.GetAccountID()

	err := p.client.DeleteAccountMember(ctx, accountID, memberID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete account member: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"user":      user.Email,
		"role":      role.Name,
		"member_id": memberID,
	}).Info("Successfully revoked Cloudflare role")

	return &models.RevokeRoleResponse{}, nil
}

// findAccountMember finds an existing account member by email
func (p *cloudflareProvider) findAccountMember(ctx context.Context, email string) (*cloudflare.AccountMember, error) {
	accountID := p.GetAccountID()

	// List account members to find the user
	members, _, err := p.client.AccountMembers(ctx, accountID, cloudflare.PaginationOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list account members: %w", err)
	}

	for _, member := range members {
		if member.User.Email == email {
			return &member, nil
		}
	}

	return nil, fmt.Errorf("member not found: %s", email)
}

// GetAuthorizedAccessUrl returns the URL to access Cloudflare dashboard
func (p *cloudflareProvider) GetAuthorizedAccessUrl(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
	resp *models.AuthorizeRoleResponse,
) string {
	// Return the Cloudflare dashboard URL for the account
	accountID := p.GetAccountID()
	return fmt.Sprintf("https://dash.cloudflare.com/%s", accountID)
}

// buildMembershipFromRole creates Cloudflare policies from role definition
// Uses the inherited Cloudflare roles to build resource-scoped policies
func (p *cloudflareProvider) buildMembershipFromRole(
	ctx context.Context,
	params *cloudflare.CreateAccountMemberParams,
	role *models.Role,
) error {

	// Inherits field is required - must specify at least one Cloudflare role
	if len(role.Inherits) == 0 {
		return fmt.Errorf("role must specify at least one inherited Cloudflare role in 'inherits' to authorize Cloudflare role")
	}

	roleIDs, err := p.getRoleIDsFromInherits(role.Inherits)
	if err != nil {
		return fmt.Errorf("failed to get role IDs from inherits: %w", err)
	}

	var permissionGroups []cloudflare.PermissionGroup
	for _, roleID := range roleIDs {
		permissionGroups = append(permissionGroups, cloudflare.PermissionGroup{
			ID: roleID,
		})
	}

	resourceGroups, err := p.buildResourceGroups(ctx, role.Resources.Allow)
	if err != nil {
		return fmt.Errorf("failed to build resource groups: %w", err)
	}

	// Create policies for each resource group
	var policies []cloudflare.Policy
	for _, resourceGroup := range resourceGroups {
		policy := cloudflare.Policy{
			PermissionGroups: permissionGroups,
			ResourceGroups: []cloudflare.ResourceGroup{
				resourceGroup,
			},
			Access: CloudflareAllow,
		}

		policies = append(policies, policy)
	}

	if len(policies) == 0 {
		return fmt.Errorf("no policies could be built from the provided roles and resources")
	}

	params.Policies = policies

	logrus.WithFields(logrus.Fields{
		"role":         role.Name,
		"policy_count": len(policies),
	}).Debug("Built resource-scoped policies from inherited roles")

	return nil

}

// getRoleIDsFromInherits gets role IDs from multiple Cloudflare roles
// This extracts the role IDs for direct role assignment in CreateAccountMemberParams
func (p *cloudflareProvider) getRoleIDsFromInherits(inherits []string) ([]string, error) {
	if len(inherits) == 0 {
		return nil, fmt.Errorf("no roles specified in inherits - must specify at least one Cloudflare role (e.g., 'DNS', 'Firewall', 'Workers Admin')")
	}

	var roleIDs []string
	seenIDs := make(map[string]bool) // Track to avoid duplicates

	// Get role ID from each inherited role
	for _, roleName := range inherits {
		// Look up the cached Cloudflare role
		cfRole, err := p.GetRole(context.TODO(), roleName)
		if err != nil {
			return nil, fmt.Errorf("role '%s' is not a recognized Cloudflare role - when using resource-scoped policies, the role name must match a Cloudflare role (e.g., 'DNS', 'Firewall', 'Workers Admin')", roleName)
		}

		// Add unique role IDs
		if !seenIDs[cfRole.ID] {
			roleIDs = append(roleIDs, cfRole.ID)
			seenIDs[cfRole.ID] = true
		}
	}

	if len(roleIDs) == 0 {
		return nil, fmt.Errorf("no role IDs found for inherited roles: %v", inherits)
	}

	logrus.WithFields(logrus.Fields{
		"inherited_roles": inherits,
		"role_id_count":   len(roleIDs),
	}).Debug("Retrieved role IDs from inherited Cloudflare roles")

	return roleIDs, nil
}

// buildResourceGroups creates Cloudflare resource groups from resource specifications
// Supported formats: "*", "account:*", "account:<id>", "zone:*", "zone:<name>", or custom keys
func (p *cloudflareProvider) buildResourceGroups(ctx context.Context, resources []string) ([]cloudflare.ResourceGroup, error) {
	var resourceGroups []cloudflare.ResourceGroup

	for _, resource := range resources {
		groups, err := p.parseResourceSpec(ctx, resource)
		if err != nil {
			return nil, fmt.Errorf("failed to parse resource spec '%s': %w", resource, err)
		}
		resourceGroups = append(resourceGroups, groups...)
	}

	if len(resourceGroups) == 0 {
		return nil, fmt.Errorf("no resource groups could be created from resources: %v", resources)
	}

	return resourceGroups, nil
}

// parseResourceSpec parses a single resource specification into resource groups
func (p *cloudflareProvider) parseResourceSpec(ctx context.Context, resource string) ([]cloudflare.ResourceGroup, error) {
	// Handle wildcard for full account access
	if resource == "*" || resource == "account:*" {
		account, err := p.getAccountByID(ctx, p.GetAccountID())
		if err != nil {
			return nil, err
		}
		return []cloudflare.ResourceGroup{cloudflare.NewResourceGroupForAccount(*account)}, nil
	}

	// Parse resource type and identifier
	parts := strings.SplitN(resource, ":", 2)
	if len(parts) != 2 {
		// No colon found, treat as custom resource group key
		return []cloudflare.ResourceGroup{cloudflare.NewResourceGroup(resource)}, nil
	}

	resourceType := parts[0]
	identifier := parts[1]

	switch resourceType {
	case resourceTypeAccount:
		return p.buildAccountResourceGroups(ctx, identifier)
	case resourceTypeZone:
		return p.buildZoneResourceGroups(ctx, identifier)
	default:
		// Unknown type, treat as custom resource group key
		return []cloudflare.ResourceGroup{cloudflare.NewResourceGroup(resource)}, nil
	}
}

// buildAccountResourceGroups creates resource groups for account specifications
func (p *cloudflareProvider) buildAccountResourceGroups(ctx context.Context, identifier string) ([]cloudflare.ResourceGroup, error) {
	if identifier == "*" {
		// Already handled in parseResourceSpec, but include for safety
		account, err := p.getAccountByID(ctx, p.GetAccountID())
		if err != nil {
			return nil, err
		}
		return []cloudflare.ResourceGroup{cloudflare.NewResourceGroupForAccount(*account)}, nil
	}

	// Specific account ID
	account, err := p.getAccountByID(ctx, identifier)
	if err != nil {
		return nil, fmt.Errorf("failed to get account %s: %w", identifier, err)
	}
	return []cloudflare.ResourceGroup{cloudflare.NewResourceGroupForAccount(*account)}, nil
}

// buildZoneResourceGroups creates resource groups for zone specifications
func (p *cloudflareProvider) buildZoneResourceGroups(ctx context.Context, identifier string) ([]cloudflare.ResourceGroup, error) {
	if identifier == "*" {
		// All zones - use cached resources
		var groups []cloudflare.ResourceGroup
		resourceList, err := p.ListResources(ctx, &models.SearchRequest{})
		if err != nil {
			return nil, fmt.Errorf("failed to list cached resources for zones: %w", err)
		}
		for _, resResult := range resourceList {
			res := resResult.Result
			if res.Type == resourceTypeZone {
				zone, ok := res.Resource.(cloudflare.Zone)
				if !ok {
					logrus.WithFields(logrus.Fields{
						"zone_name": res.Name,
						"zone_id":   res.ID,
					}).Warn("Zone resource does not contain zone details, skipping")
					continue
				}
				groups = append(groups, cloudflare.NewResourceGroupForZone(zone))
			}
		}
		if len(groups) == 0 {
			return nil, fmt.Errorf("no zones found in cache")
		}
		return groups, nil
	}

	// Specific zone - look up from cache
	zone, err := p.getZoneByName(ctx, identifier)
	if err != nil {
		return nil, fmt.Errorf("failed to get zone %s: %w", identifier, err)
	}
	return []cloudflare.ResourceGroup{cloudflare.NewResourceGroupForZone(zone)}, nil
}

// getAccountByID retrieves an account by ID from cache or API
func (p *cloudflareProvider) getAccountByID(ctx context.Context, accountID string) (*cloudflare.Account, error) {
	// Check cache first
	resourceList, err := p.ListResources(ctx, &models.SearchRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list cached resources for account: %w", err)
	}
	for _, resResult := range resourceList {
		res := resResult.Result
		if res.Type == resourceTypeAccount && res.ID == accountID {
			if account, ok := res.Resource.(cloudflare.Account); ok {
				return &account, nil
			}
		}
	}

	// Fallback to API call
	account, _, err := p.client.Account(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account from API: %w", err)
	}
	return &account, nil
}

// getZoneByName retrieves a zone by name from cache
func (p *cloudflareProvider) getZoneByName(ctx context.Context, zoneName string) (cloudflare.Zone, error) {
	resourceKey := fmt.Sprintf("%s:%s", resourceTypeZone, zoneName)
	cachedResource, err := p.GetResource(ctx, resourceKey)
	if err != nil {
		return cloudflare.Zone{}, fmt.Errorf("zone not found in cache: %s", zoneName)
	}

	if cachedResource.Type != resourceTypeZone {
		return cloudflare.Zone{}, fmt.Errorf("cached resource is not a zone: %s", zoneName)
	}

	zone, ok := cachedResource.Resource.(cloudflare.Zone)
	if !ok {
		return cloudflare.Zone{}, fmt.Errorf("zone resource does not contain zone details: %s", zoneName)
	}

	return zone, nil
}
