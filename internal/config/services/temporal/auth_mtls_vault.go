package temporal

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"software.sslmate.com/src/go-pkcs12"
)

// configureMTLSVault configures mTLS using vault-stored certificate with auto-detection of format
func (a *TemporalClient) configureMTLSVault() (*tls.Config, error) {
	logrus.WithFields(logrus.Fields{
		"vault_name": a.config.MtlsVaultName,
		"vault_type": a.config.MtlsVaultType,
	}).Debug("Configuring mTLS with vault-backed certificate")

	if a.vault == nil {
		return nil, fmt.Errorf("vault is required for vault-based mTLS configuration")
	}

	// Read the certificate data from vault secret
	certData, err := a.vault.GetSecret(a.config.MtlsVaultName)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate from vault: %w", err)
	}

	// Auto-detect format or use specified type
	format := a.config.MtlsVaultType
	if len(format) == 0 {
		format = common.DetectCertificateFormat(certData)
		if len(format) == 0 {
			return nil, fmt.Errorf("failed to auto-detect certificate format")
		}
		logrus.WithField("detected_format", format).Debug("Auto-detected certificate format")
	}

	var cert tls.Certificate

	switch strings.ToLower(format) {
	case "pem":
		cert, err = a.parsePEMFormat(certData)
	case "pkcs12", "p12", "pfx":
		cert, err = a.parsePKCS12Format(certData)
	case "der":
		cert, err = a.parseDERFormat(certData)
	default:
		return nil, fmt.Errorf("unsupported certificate format: %s", format)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate from vault: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	logrus.Info("Successfully configured mTLS with vault certificate")
	return tlsConfig, nil
}

// parsePEMFormat parses certificate and key from PEM format
func (a *TemporalClient) parsePEMFormat(data []byte) (tls.Certificate, error) {
	var certPEMs []byte
	var keyPEM []byte
	var certCount, keyCount int

	remaining := data
	for {
		block, rest := pem.Decode(remaining)
		if block == nil {
			break
		}

		switch block.Type {
		case "CERTIFICATE":
			// Append certificate block (supports certificate chains)
			certPEMs = append(certPEMs, pem.EncodeToMemory(block)...)
			certCount++
		case "PRIVATE KEY", "RSA PRIVATE KEY", "EC PRIVATE KEY":
			// Private key block
			if keyCount > 0 {
				return tls.Certificate{}, fmt.Errorf("multiple private keys found in PEM data")
			}
			keyPEM = pem.EncodeToMemory(block)
			keyCount++
		}

		remaining = rest
	}

	if certCount == 0 {
		return tls.Certificate{}, fmt.Errorf("no certificate found in PEM data")
	}
	if keyCount == 0 {
		return tls.Certificate{}, fmt.Errorf("no private key found in PEM data")
	}

	logrus.WithFields(logrus.Fields{
		"certificates": certCount,
		"has_chain":    certCount > 1,
	}).Debug("Parsed PEM format from vault")

	return tls.X509KeyPair(certPEMs, keyPEM)
}

// parsePKCS12Format parses certificate and key from PKCS12 format
func (a *TemporalClient) parsePKCS12Format(data []byte) (tls.Certificate, error) {
	password := a.config.MtlsVaultPassword

	// Decode PKCS12 data
	privateKey, certificate, caCerts, err := pkcs12.DecodeChain(data, password)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to decode PKCS12 data: %w", err)
	}

	if certificate == nil {
		return tls.Certificate{}, fmt.Errorf("no certificate found in PKCS12 data")
	}

	// Build certificate chain: leaf + intermediate CAs
	certDERs := [][]byte{certificate.Raw}
	for _, caCert := range caCerts {
		certDERs = append(certDERs, caCert.Raw)
	}

	logrus.WithFields(logrus.Fields{
		"certificates": len(certDERs),
		"has_chain":    len(certDERs) > 1,
		"encrypted":    password != "",
	}).Debug("Parsed PKCS12 format from vault")

	return tls.Certificate{
		Certificate: certDERs,
		PrivateKey:  privateKey,
		Leaf:        certificate,
	}, nil
}

// parseDERFormat parses certificate from DER format (typically certificate only)
func (a *TemporalClient) parseDERFormat(data []byte) (tls.Certificate, error) {
	// DER format typically contains just the certificate, not the private key
	// This is less common for TLS client certificates
	certificate, err := x509.ParseCertificate(data)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to parse DER certificate: %w", err)
	}

	logrus.Debug("Parsed DER format from vault (certificate only, no private key)")

	// Note: This will not work for mTLS without a private key
	// Users should use PEM or PKCS12 format for complete certificate+key
	return tls.Certificate{}, fmt.Errorf("DER format does not contain private key, use PEM or PKCS12 format instead")
}
