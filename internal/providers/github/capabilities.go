package github

import "github.com/thand-io/agent/internal/models"

var GithubCapabilities = models.NewProviderCapabilities().
	WithDefaultAuthorizerConfiguration().
	WithDefaultUsersConfiguration().
	WithDefaultGroupsConfiguration().
	WithRolesConfiguration(models.RolesConfiguration{
		Enabled:        true,
		Synchronizable: false, // roles are statically defined by GitHub
	}).
	WithDefaultProvisioningConfiguration()
