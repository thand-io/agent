package models

import (
	"crypto/tls"
	"crypto/x509"
)

// CertificateSource represents the origin of a certificate
type CertificateSource string

const (
	// CertSourceInline indicates certificate and key are provided as PEM strings
	CertSourceInline CertificateSource = "inline"

	// CertSourceFile indicates certificate and key are loaded from file paths
	CertSourceFile CertificateSource = "file"

	// CertSourceVault indicates certificate and key are loaded from CSP vault secret (combined)
	CertSourceVault CertificateSource = "vault"

	// CertSourceHSM indicates certificate is in vault and key is HSM-backed (AWS KMS, Azure Key Vault Key, GCP Cloud KMS)
	CertSourceHSM CertificateSource = "hsm"

	// CertSourceUnknown indicates no certificate source was configured
	CertSourceUnknown CertificateSource = "unknown"
)

// CertificateConfig contains all possible sources for loading a certificate
// and its optional CA certificate. Only one certificate source should be configured.
type CertificateConfig struct {
	// Inline PEM-encoded certificate and key
	CertPEM string `mapstructure:"cert"`
	KeyPEM  string `mapstructure:"key"`

	// File paths for certificate and key
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`

	// CSP vault secret containing combined cert+key in PEM format
	CertKeySecret string `mapstructure:"cert_key_secret"`

	// HSM-backed key pattern: Certificate in secret store, private key in HSM
	// (AWS KMS, Azure Key Vault Key, GCP Cloud KMS)
	CertSecret string `mapstructure:"cert_secret"`  // Secret containing only the certificate
	HSMKeyID   string `mapstructure:"hsm_key_id"`   // HSM key resource identifier (ARN/URL/resource name)
	HSMKeyType string `mapstructure:"hsm_key_type"` // Optional: "aws-kms", "azure-keyvault", "gcp-kms" (auto-detected from platform if not set)

	// Platform configuration (needed for HSM signer initialization)
	PlatformConfig *BasicConfig `mapstructure:"-"` // Not loaded from config, set by caller

	// Optional CA certificate (for server verification)
	CAPEM    string `mapstructure:"ca"`
	CAFile   string `mapstructure:"ca_file"`
	CASecret string `mapstructure:"ca_secret"`
}

// LoadedCertificate represents a successfully loaded certificate with optional CA pool
type LoadedCertificate struct {
	// Certificate contains the loaded client certificate and private key
	Certificate tls.Certificate

	// CAPool contains the CA certificate(s) for server verification (optional)
	// If nil, system default CA bundle will be used
	CAPool *x509.CertPool

	// Source indicates which source the certificate was loaded from
	Source CertificateSource
}

// CertificateLoaderImpl defines the interface for loading certificates from various sources
type CertificateLoaderImpl interface {
	// LoadCertificate loads a certificate from the configured source
	// Returns an error if:
	// - Multiple certificate sources are configured
	// - Vault secret is configured but vault service is unavailable
	// - Certificate data is invalid or cannot be parsed
	// - Private key is missing or invalid
	LoadCertificate(config *CertificateConfig) (*LoadedCertificate, error)
}
