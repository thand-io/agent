package gcp

import "github.com/thand-io/agent/internal/models"

var GcpCapabilities = models.NewProviderCapabilities().
	WithDefaultIdentitiesConfiguration().
	WithDefaultRolesConfiguration().
	WithPermissionsConfiguration(models.PermissionsConfiguration{
		SynchronizableConfiguration: models.SynchronizableConfiguration{
			Enabled:        true,
			Synchronizable: false,
		},
		// SupportsWildcards defaults to false — GCP IAM requires exact permission names
	}).
	WithDefaultProvisioningConfiguration().
	WithDefaultTenantsConfiguration()
