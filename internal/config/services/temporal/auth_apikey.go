package temporal

import (
	"crypto/tls"

	"github.com/sirupsen/logrus"
	"go.temporal.io/sdk/client"
)

// configureAPIKeyAuth configures Temporal client with API Key authentication
func (a *TemporalClient) configureAPIKeyAuth(options *client.Options) error {
	if len(a.config.ApiKey) == 0 {
		return nil
	}

	logrus.Info("Configuring Temporal client with API Key authentication")
	options.ConnectionOptions = client.ConnectionOptions{
		TLS: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
	options.Credentials = client.NewAPIKeyStaticCredentials(a.config.ApiKey)
	return nil
}

// hasAPIKeyAuth checks if API Key authentication is configured
func (a *TemporalClient) hasAPIKeyAuth() bool {
	return len(a.config.ApiKey) > 0
}
