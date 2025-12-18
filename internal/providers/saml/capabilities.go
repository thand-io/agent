package saml

import "github.com/thand-io/agent/internal/models"

var SamlCapabilities = models.NewProviderCapabilities().
	WithDefaultAuthorizerConfiguration().
	WithIdentitiesConfiguration(models.IdentitiesConfiguration{
		Enabled:        true,
		Synchronizable: false,
	})
