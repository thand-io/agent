package aws

import "github.com/thand-io/agent/internal/models"

var AwsCapabilities = models.NewProviderCapabilities().
	WithDefaultUsersConfiguration().
	WithDefaultGroupsConfiguration().
	WithDefaultIdentitiesConfiguration().
	WithRolesConfiguration(models.RolesConfiguration{
		Enabled:        true,
		Synchronizable: false, // roles are statically defined by AWS
	}).
	WithPermissionsConfiguration(models.PermissionsConfiguration{
		Enabled:        true,
		Synchronizable: false, // permissions are derived from AWS IAM roles
	}).
	WithDefaultProvisioningConfiguration().
	WithDefaultTenantsConfiguration()
