package oauth2

import (
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
)

const Oauth2ProviderName = "oauth2"

// oauth2Provider implements the ProviderImpl interface for OAuth2
type oauth2Provider struct {
	*models.BaseProvider
}

func (p *oauth2Provider) Initialize(identifier string, provider models.ProviderConfig) error {
	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		OAuth2Capabilities,
	)
	// TODO: Implement OAuth2 initialization logic
	return nil
}

func init() {
	providers.Register(Oauth2ProviderName, &oauth2Provider{}, OAuth2Capabilities, &ConfigSchema{})
}
