package local

import (
	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

var LocalCapabilities = models.NewProviderCapabilities().
	WithPermissionsConfiguration(models.PermissionsConfiguration{
		SynchronizableConfiguration: models.SynchronizableConfiguration{
			Enabled:        true,
			Synchronizable: false,
		},
		SupportsWildcards: true,
	}).
	WithProvisioningConfiguration(models.ProvisioningConfiguration{
		Runtime: sdkConstants.ModeAgent,
	}).
	WithPermissionsConfiguration(models.PermissionsConfiguration{
		// Runtime: sdkConstants.ModeAgent,
	}).
	WithRolesConfiguration(models.RolesConfiguration{
		//
	}).
	WithDefaultProvisioningConfiguration().
	WithDefaultTenantsConfiguration()
