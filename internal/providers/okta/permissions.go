package okta

import (
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

func (p *oktaProvider) CanSynchronizePermissions() bool {
	return false
}

// Okta permissions map - complete catalog from Okta Management API
// Reference: https://developer.okta.com/docs/api/openapi/okta-management/guides/permissions/#permissions-catalog
// Note: The Okta SDK v2 does not have built-in support for custom admin role permissions.
// These permissions are primarily used for reference and custom role creation via API.
var oktaPermissions = map[string]string{
	// Agent permissions
	"okta.agents.manage":   "Allows the admin to manage agent communication and agent updates",
	"okta.agents.read":     "Allows the admin to download agents and view agent statuses",
	"okta.agents.register": "Allows the admin to register agents and domains",

	// App permissions
	"okta.apps.assignment.manage":      "Allows the admin to manage assignment operations of an app in your Okta org and view the following provisioning errors: app assignment, group push mapping, and Error Profile push updates",
	"okta.apps.clientCredentials.read": "Allows the admin to view information about client credentials for the app",
	"okta.apps.manage":                 "Allows the admin to fully manage apps and their members in your Okta org",
	"okta.apps.manageFirstPartyApps":   "Allows the admin to manage first-party apps",
	"okta.apps.read":                   "Allows the admin to only read information about apps and their members in your Okta org",
	"okta.apps.universalLogout.manage": "Allows the admin to manage universal logout settings for apps",
	"okta.apps.universalLogout.read":   "Allows the admin to view universal logout settings for apps",

	// Authorization Server permissions
	"okta.authzServers.manage": "Allows the admin to manage authorization servers",
	"okta.authzServers.read":   "Allows the admin to read authorization servers",

	// Customization permissions
	"okta.customizations.manage": "Allows the admin to manage customizations",
	"okta.customizations.read":   "Allows the admin to read customizations",

	// Device permissions
	"okta.devices.lifecycle.activate":   "Allows the admin to activate devices",
	"okta.devices.lifecycle.deactivate": "Allows the admin to deactivate devices. When you deactivate a device, it loses all device user links",
	"okta.devices.lifecycle.delete":     "Allows the admin to permanently delete devices",
	"okta.devices.lifecycle.manage":     "Allows the admin to perform any device lifecycle operations",
	"okta.devices.lifecycle.suspend":    "Allows the admin to suspend device access to Okta",
	"okta.devices.lifecycle.unsuspend":  "Allows the admin to unsuspend and restore device access to Okta",
	"okta.devices.manage":               "Allows the admin to manage devices and perform all device lifecycle operations",
	"okta.devices.read":                 "Allows the admin to read device details",

	// Directory permissions
	"okta.directories.manage": "Allows the admin to manage all directory integration settings of an app instance",
	"okta.directories.read":   "Allows the admin to view the directory integration settings of an app instance",

	// Governance permissions
	"okta.governance.accessCertifications.manage": "Allows the admin to view and manage access certification campaigns",
	"okta.governance.accessRequests.manage":       "Allows the admin to view and manage access requests",

	// Group permissions
	"okta.groups.appAssignment.manage": "Allows the admin to manage a group's app assignment (also need okta.apps.assignment.manage to assign to a specific app)",
	"okta.groups.create":               "Allows the admin to create groups",
	"okta.groups.manage":               "Allows the admin to fully manage groups in your Okta org",
	"okta.groups.members.manage":       "Allows the admin to only manage member operations in a group in your Okta org",
	"okta.groups.read":                 "Allows the admin to only read information about groups and their members in your Okta org",

	// IAM permissions
	"okta.iam.read": "Allows the admin to view roles, resources, and admin assignments",

	// Identity Provider permissions
	"okta.identityProviders.manage": "Allows the admin to manage Identity Providers",
	"okta.identityProviders.read":   "Allows the admin to read Identity Providers",

	// Policy permissions
	"okta.policies.manage": "Allows the admin to manage policies",
	"okta.policies.read":   "Allows the admin to view any policy",

	// Profile Source permissions
	"okta.profilesources.import.run": "Allows the admin to run imports for apps with a profile source, such as HRaaS and AD/LDAP apps. Admins with this permission can create users through the import",

	// Realm permissions
	"okta.realms.manage": "Allows the admin to view, create, and manage realms",
	"okta.realms.read":   "Allows the admin to view realms",

	// Shared Signals Framework permissions
	"okta.ssf.securityEventsProviders.manage": "Allows the admin to manage shared signals framework receivers",
	"okta.ssf.securityEventsProviders.read":   "Allows the admin to view shared signals framework receivers",

	// Support permissions
	"okta.support.cases.manage": "Allows the admin to view, create, and manage Okta Support cases",

	// User API Token permissions
	"okta.users.apitokens.clear":  "Allows the admin to clear user API tokens",
	"okta.users.apitokens.manage": "Allows the admin to manage API tokens",
	"okta.users.apitokens.read":   "Allows the admin to view API tokens",

	// User App Assignment permissions
	"okta.users.appAssignment.manage": "Allows the admin to manage a user's app assignment (also need okta.apps.assignment.manage to assign to a specific app)",

	// User Create permissions
	"okta.users.create": "Allows the admin to create users. If the admin is also scoped to manage a group, that admin can add the user to the group on creation and then manage",

	// User Credentials permissions
	"okta.users.credentials.expirePassword":            "Allows the admin to expire a user's password and set a new temporary password",
	"okta.users.credentials.manage":                    "Allows the admin to manage only credential lifecycle operations for a user",
	"okta.users.credentials.manageTemporaryAccessCode": "Allows admin to view, create and delete a user's temporary access code",
	"okta.users.credentials.resetFactors":              "Allows the admin to reset MFA authenticators for users",
	"okta.users.credentials.resetPassword":             "Allows the admin to reset passwords for users",

	// User Group Membership permissions
	"okta.users.groupMembership.manage": "Allows the admin to manage a user's group membership (also need okta.groups.members.manage to assign to a specific group)",

	// User Lifecycle permissions
	"okta.users.lifecycle.activate":      "Allows the admin to activate user accounts",
	"okta.users.lifecycle.clearSessions": "Allows the admin to clear all active Okta sessions and OAuth 2.0 tokens for a user",
	"okta.users.lifecycle.deactivate":    "Allows the admin to deactivate user accounts",
	"okta.users.lifecycle.delete":        "Allows the admin to permanently delete user accounts",
	"okta.users.lifecycle.manage":        "Allows the admin to perform any user lifecycle operations",
	"okta.users.lifecycle.suspend":       "Allows the admin to suspend user access to Okta. When a user is suspended, their user sessions are also cleared",
	"okta.users.lifecycle.unlock":        "Allows the admin to unlock users who have been locked out of Okta",
	"okta.users.lifecycle.unsuspend":     "Allows the admin to restore user access to Okta",

	// User Management permissions
	"okta.users.manage": "Allows the admin to create and manage users and read all profile and credential information for users. Delegated admins with this permission can only manage user credential fields and not the credential values themselves",
	"okta.users.read":   "Allows the admin to read any user's profile and credential information. Delegated admins with this permission can only manage user credential fields and not the credential values themselves",

	// User Risk permissions
	"okta.users.risk.manage": "Allows the admin to provide user risk feedback and elevate user risk",
	"okta.users.risk.read":   "Allows the admin to view user risk",

	// User Profile permissions
	"okta.users.userprofile.manage": "Allows the admin to only perform operations on the user object, including hidden and sensitive attributes",
	"okta.users.userprofile.read":   "Allows the admin to view profile of a user",

	// Workflow permissions
	"okta.workflows.invoke": "Allows the admin to view and run delegated flows",
	"okta.workflows.read":   "Allows the admin to view delegated flows",
}

func (p *oktaProvider) getStaticPermissions() []models.ProviderPermission {

	var permissions []models.ProviderPermission

	// Convert to slice
	for name, description := range oktaPermissions {
		perm := models.ProviderPermission{
			ID:          strings.ToLower(name),
			Name:        name,
			Description: description,
		}
		permissions = append(permissions, perm)
	}

	logrus.WithFields(logrus.Fields{
		"permissions": len(permissions),
	}).Debug("Loaded Okta permissions")

	return permissions
}
