package thand

import "github.com/thand-io/agent/internal/models"

var ThandCapabilities = models.NewProviderCapabilities().
	WithIdentitiesConfiguration(models.IdentitiesConfiguration{
		Enabled:        true,
		Synchronizable: false,
	}).
	WithDefaultAuthorizerConfiguration()
