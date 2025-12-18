package okta

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

func (p *oktaProvider) CanSynchronizeRoles() bool {
	return false
}

// Okta predefined administrator roles
// Reference: https://help.okta.com/en-us/content/topics/security/administrators-admin-comparison.htm
var oktaPredefinedRoles = map[string]models.ProviderRole{
	"SUPER_ADMIN": {
		ID:          "SUPER_ADMIN",
		Name:        "Super Administrator",
		Description: "Full administrative access to the Okta organization. Can perform all administrative tasks including managing other administrators.",
	},
	"ORG_ADMIN": {
		ID:          "ORG_ADMIN",
		Name:        "Organization Administrator",
		Description: "Full administrative access except for managing super administrators. Can manage users, groups, apps, and most org settings.",
	},
	"APP_ADMIN": {
		ID:          "APP_ADMIN",
		Name:        "Application Administrator",
		Description: "Can create and manage applications and their assignments. Cannot manage users or groups unless they are assigned to apps.",
	},
	"USER_ADMIN": {
		ID:          "USER_ADMIN",
		Name:        "User Administrator",
		Description: "Can create and manage users and groups. Cannot manage applications or advanced settings.",
	},
	"GROUP_MEMBERSHIP_ADMIN": {
		ID:          "GROUP_MEMBERSHIP_ADMIN",
		Name:        "Group Membership Administrator",
		Description: "Can manage group membership but cannot create or delete groups.",
	},
	"HELP_DESK_ADMIN": {
		ID:          "HELP_DESK_ADMIN",
		Name:        "Help Desk Administrator",
		Description: "Can reset passwords and MFA factors for users. Limited administrative capabilities for support purposes.",
	},
	"READ_ONLY_ADMIN": {
		ID:          "READ_ONLY_ADMIN",
		Name:        "Read-Only Administrator",
		Description: "Can view all aspects of the Okta organization but cannot make changes.",
	},
	"MOBILE_ADMIN": {
		ID:          "MOBILE_ADMIN",
		Name:        "Mobile Administrator",
		Description: "Can manage mobile device management settings and policies.",
	},
	"API_ACCESS_MANAGEMENT_ADMIN": {
		ID:          "API_ACCESS_MANAGEMENT_ADMIN",
		Name:        "API Access Management Administrator",
		Description: "Can manage authorization servers, scopes, and claims for API access management.",
	},
	"REPORT_ADMIN": {
		ID:          "REPORT_ADMIN",
		Name:        "Report Administrator",
		Description: "Can create and view reports about the Okta organization.",
	},
	"GROUP_ADMIN": {
		ID:          "GROUP_ADMIN",
		Name:        "Group Administrator",
		Description: "Can create, manage, and delete groups. Can manage group membership.",
	},
}

// Also load in user groups as these can have roles assigned too

func (p *oktaProvider) SynchronizeRoles(ctx context.Context, req *models.SynchronizeRolesRequest) (*models.SynchronizeRolesResponse, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Loaded Okta roles in %s", elapsed)
	}()

	var roles []models.ProviderRole

	// Load predefined standard roles
	// These are Okta's built-in administrator roles that are consistent across all Okta orgs
	// Reference: https://help.okta.com/en-us/content/topics/security/administrators-admin-comparison.htm
	for _, role := range oktaPredefinedRoles {
		roles = append(roles, role)
	}

	logrus.WithFields(logrus.Fields{
		"roles": len(roles),
	}).Debug("Loaded Okta standard roles")

	return &models.SynchronizeRolesResponse{
		Roles: roles,
	}, nil
}
