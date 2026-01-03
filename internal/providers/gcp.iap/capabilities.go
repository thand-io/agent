package gcpiap

import "github.com/thand-io/agent/internal/models"

var GcpIAPCapabilities = models.NewProviderCapabilities().
	WithDefaultAuthorizerConfiguration().
	WithIdentitiesConfiguration(models.IdentitiesConfiguration{
		Enabled:        true,
		Synchronizable: false,
	})
