package temporal

import (
	"crypto/tls"
	"fmt"

	"github.com/sirupsen/logrus"
)

// configureMTLSInline configures mTLS using inline PEM certificates
func (a *TemporalClient) configureMTLSInline() (*tls.Config, error) {
	logrus.Debug("Configuring mTLS with inline certificates")

	// Parse certificate and key from PEM
	cert, err := tls.X509KeyPair([]byte(a.config.MtlsCert), []byte(a.config.MtlsKey))
	if err != nil {
		return nil, fmt.Errorf("failed to parse inline mTLS certificate and key: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	logrus.Info("Successfully configured mTLS with inline certificates")
	return tlsConfig, nil
}
