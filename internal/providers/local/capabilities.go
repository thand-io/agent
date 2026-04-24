package local

import "github.com/thand-io/agent/internal/models"

var LocalCapabilities = models.NewProviderCapabilities().
	WithPermissionsConfiguration(models.PermissionsConfiguration{
		SynchronizableConfiguration: models.SynchronizableConfiguration{
			Enabled:        true,
			Synchronizable: false,
		},
		SupportsWildcards: true,
	}).
	WithDefaultProvisioningConfiguration()
