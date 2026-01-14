package temporal

import (
	"crypto/tls"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
)

// configureMTLSFile configures mTLS using certificate files
func (a *TemporalClient) configureMTLSFile() (*tls.Config, error) {
	logrus.WithFields(logrus.Fields{
		"cert_file": a.config.MtlsCertFile,
		"key_file":  a.config.MtlsKeyFile,
	}).Debug("Configuring mTLS with file-based certificates")

	// Read certificate file
	certPEM, err := os.ReadFile(a.config.MtlsCertFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate file %s: %w", a.config.MtlsCertFile, err)
	}

	// Read key file
	keyPEM, err := os.ReadFile(a.config.MtlsKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file %s: %w", a.config.MtlsKeyFile, err)
	}

	// Parse certificate and key from PEM
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mTLS certificate and key from files: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	logrus.Info("Successfully configured mTLS with file-based certificates")
	return tlsConfig, nil
}
