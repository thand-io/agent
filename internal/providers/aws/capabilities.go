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
		SynchronizableConfiguration: models.SynchronizableConfiguration{
			Enabled:        true,
			Synchronizable: false, // permissions are derived from AWS IAM roles
		},
		SupportsWildcards: true, // AWS IAM natively accepts wildcards like ec2:*
	}).
	WithDefaultProvisioningConfiguration().
	WithDefaultTenantsConfiguration()
