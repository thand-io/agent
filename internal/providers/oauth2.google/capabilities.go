package googleoauth2

import "github.com/thand-io/agent/internal/models"

var GoogleOAuth2Capabilities = models.NewProviderCapabilities().
	WithDefaultAuthorizerConfiguration().
	WithIdentitiesConfiguration(models.IdentitiesConfiguration{
		Enabled:        true,
		Synchronizable: false,
	})
