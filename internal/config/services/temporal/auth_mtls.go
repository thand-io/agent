package temporal

import (
	"crypto/tls"
	"fmt"

	"github.com/sirupsen/logrus"
	"go.temporal.io/sdk/client"
)

// configureMTLSAuth configures Temporal client with mTLS authentication
// It checks for each mTLS mode in order of precedence
func (a *TemporalClient) configureMTLSAuth(options *client.Options) error {
	if !a.config.HasMtlsConfig() {
		return nil
	}

	logrus.Info("Configuring Temporal client with mTLS authentication")

	var tlsConfig *tls.Config
	var err error

	// Check each mTLS mode in order of precedence
	switch {
	case a.hasMTLSInline():
		tlsConfig, err = a.configureMTLSInline()
	case a.hasMTLSFile():
		tlsConfig, err = a.configureMTLSFile()
	case a.hasMTLSVault():
		tlsConfig, err = a.configureMTLSVault()
	default:
		return fmt.Errorf("mTLS configuration detected but no valid mode found")
	}

	if err != nil {
		return fmt.Errorf("failed to configure mTLS: %w", err)
	}

	options.ConnectionOptions = client.ConnectionOptions{
		TLS: tlsConfig,
	}

	return nil
}

// hasMTLSInline checks if inline mTLS configuration is present
func (a *TemporalClient) hasMTLSInline() bool {
	return len(a.config.MtlsCert) > 0 && len(a.config.MtlsKey) > 0
}

// hasMTLSFile checks if file-based mTLS configuration is present
func (a *TemporalClient) hasMTLSFile() bool {
	return len(a.config.MtlsCertFile) > 0 && len(a.config.MtlsKeyFile) > 0
}

// hasMTLSVault checks if vault-based mTLS configuration is present
func (a *TemporalClient) hasMTLSVault() bool {
	return len(a.config.MtlsVaultName) > 0
}
