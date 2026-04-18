package oauth2

import (
	"net/http"
	"sync"
	"time"

	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
)

const Oauth2ProviderName = "oauth2"

var defaultOutboundHTTPClient = &http.Client{Timeout: 10 * time.Second}

func cloneDefaultOutboundHTTPClient() *http.Client {
	return &http.Client{
		Timeout:       defaultOutboundHTTPClient.Timeout,
		Transport:     defaultOutboundHTTPClient.Transport,
		CheckRedirect: defaultOutboundHTTPClient.CheckRedirect,
		Jar:           defaultOutboundHTTPClient.Jar,
	}
}

// oauth2Provider implements the ProviderImpl interface for OAuth2
type oauth2Provider struct {
	*models.BaseProvider

	httpClient       *http.Client
	resolvedConfigMu sync.RWMutex
	resolvedConfig   *resolvedOAuth2Config
}

func (p *oauth2Provider) Initialize(identifier string, provider models.ProviderConfig) error {
	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		OAuth2Capabilities,
	)
	p.httpClient = cloneDefaultOutboundHTTPClient()
	// TODO: Implement OAuth2 initialization logic
	return nil
}

func (p *oauth2Provider) getHTTPClient() *http.Client {
	if p != nil && p.httpClient != nil {
		return p.httpClient
	}

	return defaultOutboundHTTPClient
}

func init() {
	providers.Register(Oauth2ProviderName, &oauth2Provider{}, OAuth2Capabilities, &ConfigSchema{})
}
