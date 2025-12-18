package oauth2

import "github.com/thand-io/agent/internal/models"

var OAuth2Capabilities = models.NewProviderCapabilities().
	WithDefaultAuthorizerConfiguration().
	WithIdentitiesConfiguration(models.IdentitiesConfiguration{
		Enabled:        true,
		Synchronizable: false,
	})
