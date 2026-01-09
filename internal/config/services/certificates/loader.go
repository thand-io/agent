package certificates

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"reflect"

	"github.com/thand-io/agent/internal/models"
)

// certificateLoader implements the CertificateLoaderImpl interface
type certificateLoader struct {
	vault models.VaultImpl
}

// NewCertificateLoader creates a new certificate loader that can load certificates
// from inline PEM strings, file paths, or CSP vault secrets.
//
// The vault parameter can be nil if only inline or file-based certificate loading is needed.
func NewCertificateLoader(vault models.VaultImpl) models.CertificateLoaderImpl {
	return &certificateLoader{
		vault: vault,
	}
}

// LoadCertificate loads a certificate from the configured source (inline, file, or vault)
func (l *certificateLoader) LoadCertificate(config *models.CertificateConfig) (*models.LoadedCertificate, error) {
	if config == nil {
		return nil, fmt.Errorf("certificate config is nil")
	}

	// Validate configuration first (checks for multiple sources)
	source, err := l.determineAndValidateSource(config)
	if err != nil {
		return nil, err
	}

	// Additional validation for the specific source
	if err := l.validateSourceConfig(config, source); err != nil {
		return nil, err
	}

	// Load certificate and key based on source
	var cert tls.Certificate

	switch source {
	case models.CertSourceInline:
		cert, err = l.loadInlineCert(config)
	case models.CertSourceFile:
		cert, err = l.loadFileCert(config)
	case models.CertSourceVault:
		cert, err = l.loadVaultCert(config)
	case models.CertSourceHSM:
		cert, err = l.loadHSMCert(config)
	default:
		return nil, fmt.Errorf("unsupported certificate source: %s", source)
	}

	if err != nil {
		return nil, err
	}

	// Load CA certificate if configured
	caPool, err := l.loadCAIfConfigured(config)
	if err != nil {
		return nil, fmt.Errorf("failed to load CA certificate: %w", err)
	}

	return &models.LoadedCertificate{
		Certificate: cert,
		CAPool:      caPool,
		Source:      source,
	}, nil
}

// determineAndValidateSource identifies which certificate source is configured and validates it
func (l *certificateLoader) determineAndValidateSource(config *models.CertificateConfig) (models.CertificateSource, error) {
	// Count how many sources are configured
	sources := []string{}
	detectedSource := models.CertSourceUnknown

	if len(config.CertPEM) > 0 || len(config.KeyPEM) > 0 {
		sources = append(sources, "inline cert/key")
		detectedSource = models.CertSourceInline
	}

	if len(config.CertFile) > 0 || len(config.KeyFile) > 0 {
		sources = append(sources, "file paths")
		if detectedSource == models.CertSourceUnknown {
			detectedSource = models.CertSourceFile
		}
	}

	if len(config.CertKeySecret) > 0 {
		sources = append(sources, "vault secret (combined cert+key)")
		if detectedSource == models.CertSourceUnknown {
			detectedSource = models.CertSourceVault
		}
	}

	// Check for HSM pattern: cert in secret + HSM key reference
	if len(config.CertSecret) > 0 || len(config.HSMKeyID) > 0 {
		sources = append(sources, "HSM-backed key (cert secret + HSM key)")
		if detectedSource == models.CertSourceUnknown {
			detectedSource = models.CertSourceHSM
		}
	}

	// Error if multiple sources configured
	if len(sources) > 1 {
		return models.CertSourceUnknown, fmt.Errorf("multiple certificate sources configured (%s); please configure only one certificate source", joinSources(sources))
	}

	// Error if no source configured
	if len(sources) == 0 {
		return models.CertSourceUnknown, fmt.Errorf("no certificate source configured (inline, file, vault secret, or HSM required)")
	}

	return detectedSource, nil
}

// validateSourceConfig ensures configuration is valid for the specific source type
func (l *certificateLoader) validateSourceConfig(config *models.CertificateConfig, source models.CertificateSource) error {
	// Validate source-specific requirements
	switch source {
	case models.CertSourceInline:
		if len(config.CertPEM) == 0 {
			return fmt.Errorf("inline certificate PEM is required when using inline source")
		}
		if len(config.KeyPEM) == 0 {
			return fmt.Errorf("inline private key PEM is required when using inline source")
		}

	case models.CertSourceFile:
		if len(config.CertFile) == 0 {
			return fmt.Errorf("certificate file path is required when using file source")
		}
		if len(config.KeyFile) == 0 {
			return fmt.Errorf("private key file path is required when using file source")
		}

	case models.CertSourceVault:
		if len(config.CertKeySecret) == 0 {
			return fmt.Errorf("vault secret name is required when using vault source")
		}
		if l.vault == nil || !l.isVaultAvailable() {
			return fmt.Errorf("vault secret configured (%s) but vault service is not available", config.CertKeySecret)
		}

	case models.CertSourceHSM:
		if len(config.CertSecret) == 0 {
			return fmt.Errorf("certificate secret is required when using HSM-backed key (cert_secret)")
		}
		if len(config.HSMKeyID) == 0 {
			return fmt.Errorf("HSM key identifier is required when using HSM-backed key (hsm_key_id)")
		}
		if l.vault == nil || !l.isVaultAvailable() {
			return fmt.Errorf("HSM pattern requires vault service for certificate retrieval")
		}
		if config.PlatformConfig == nil {
			return fmt.Errorf("platform configuration is required for HSM signer initialization")
		}
	}

	// Validate CA configuration (ensure only one CA source)
	caSources := []string{}

	if len(config.CAPEM) > 0 {
		caSources = append(caSources, "inline CA")
	}

	if len(config.CAFile) > 0 {
		caSources = append(caSources, "CA file")
	}

	if len(config.CASecret) > 0 {
		caSources = append(caSources, "CA vault secret")
		if l.vault == nil {
			return fmt.Errorf("CA vault secret configured (%s) but vault service is not available", config.CASecret)
		}
	}

	if len(caSources) > 1 {
		return fmt.Errorf("multiple CA certificate sources configured (%s); please configure only one CA source", joinSources(caSources))
	}

	return nil
}

// loadInlineCert loads certificate from inline PEM strings
func (l *certificateLoader) loadInlineCert(config *models.CertificateConfig) (tls.Certificate, error) {
	cert, err := tls.X509KeyPair([]byte(config.CertPEM), []byte(config.KeyPEM))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to parse inline certificate and key: %w", err)
	}

	return cert, nil
}

// loadFileCert loads certificate from file paths
func (l *certificateLoader) loadFileCert(config *models.CertificateConfig) (tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to load certificate from files (%s, %s): %w",
			config.CertFile, config.KeyFile, err)
	}

	return cert, nil
}

// loadVaultCert loads certificate from CSP vault secret (combined cert+key format)
func (l *certificateLoader) loadVaultCert(config *models.CertificateConfig) (tls.Certificate, error) {
	// Retrieve secret from vault
	secretData, err := l.vault.GetSecret(config.CertKeySecret)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to retrieve vault secret (%s): %w",
			config.CertKeySecret, err)
	}

	// Parse combined PEM format (cert + key in one secret)
	certPEM, keyPEM, err := ParseCombinedPEM(secretData)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to parse combined PEM from vault secret: %w", err)
	}

	// Load the certificate and key
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to parse certificate and key from vault secret: %w", err)
	}

	return cert, nil
}

// loadHSMCert loads certificate from vault and creates an HSM-backed signer for the private key
// This pattern is used when the private key is managed by a CSP HSM (AWS KMS, Azure Key Vault Key, GCP Cloud KMS)
// and never leaves the hardware security module.
func (l *certificateLoader) loadHSMCert(config *models.CertificateConfig) (tls.Certificate, error) {
	// Retrieve certificate from vault (certificate only, no private key)
	certData, err := l.vault.GetSecret(config.CertSecret)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to retrieve certificate from vault secret (%s): %w",
			config.CertSecret, err)
	}

	// Parse certificate PEM
	block, _ := pem.Decode(certData)
	if block == nil || block.Type != "CERTIFICATE" {
		return tls.Certificate{}, fmt.Errorf("invalid certificate PEM in vault secret %s", config.CertSecret)
	}

	// Parse the X.509 certificate to extract metadata
	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to parse certificate from vault secret: %w", err)
	}

	// Determine HSM key type (auto-detect from platform if not specified)
	keyType := config.HSMKeyType
	if keyType == "" && config.PlatformConfig != nil {
		// Auto-detect from platform
		if platform, ok := config.PlatformConfig.GetString("platform"); ok {
			keyType = platform
		}
	}

	// Create HSM signer for the private key
	hsmSigner, err := NewHSMSigner(config.HSMKeyID, keyType, keyType, config.PlatformConfig)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to create HSM signer for key %s: %w",
			config.HSMKeyID, err)
	}

	// Initialize the HSM signer (establishes connection to HSM service)
	if err := hsmSigner.Initialize(); err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to initialize HSM signer: %w", err)
	}

	// Build tls.Certificate with certificate and HSM signer
	cert := tls.Certificate{
		Certificate: [][]byte{block.Bytes},
		PrivateKey:  hsmSigner, // crypto.Signer interface
		Leaf:        x509Cert,
	}

	return cert, nil
}

// loadCAIfConfigured loads CA certificate if any CA source is configured
func (l *certificateLoader) loadCAIfConfigured(config *models.CertificateConfig) (*x509.CertPool, error) {
	var caPEM []byte
	var err error

	// Determine CA source and load accordingly
	if len(config.CAPEM) > 0 {
		caPEM = []byte(config.CAPEM)
	} else if len(config.CAFile) > 0 {
		caPEM, err = os.ReadFile(config.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate file (%s): %w", config.CAFile, err)
		}
	} else if len(config.CASecret) > 0 {
		caPEM, err = l.vault.GetSecret(config.CASecret)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve CA certificate from vault secret (%s): %w",
				config.CASecret, err)
		}
	}

	// If no CA configured, return nil (system CA bundle will be used)
	if len(caPEM) == 0 {
		return nil, nil
	}

	// Create certificate pool and add CA certificate(s)
	caPool := x509.NewCertPool()

	// Support multiple CA certificates in the PEM file
	remaining := caPEM
	certCount := 0

	for {
		block, rest := pem.Decode(remaining)
		if block == nil {
			break
		}

		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("failed to parse CA certificate block %d: %w", certCount+1, err)
			}
			caPool.AddCert(cert)
			certCount++
		}

		remaining = rest
	}

	if certCount == 0 {
		return nil, fmt.Errorf("no CA certificates found in PEM data")
	}

	return caPool, nil
}

// isVaultAvailable checks if the vault interface is actually usable (not a typed nil)
func (l *certificateLoader) isVaultAvailable() bool {
	if l.vault == nil {
		return false
	}

	// Use reflection to check if the interface's concrete value is nil
	// This handles the case where (*SomeType)(nil) is passed as an interface
	v := reflect.ValueOf(l.vault)
	if !v.IsValid() {
		return false
	}

	// Check if the interface holds a nil pointer
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return false
	}

	return true
}

// joinSources joins source names with commas for error messages
func joinSources(sources []string) string {
	if len(sources) == 0 {
		return ""
	}
	if len(sources) == 1 {
		return sources[0]
	}

	result := sources[0]
	for i := 1; i < len(sources); i++ {
		result += ", " + sources[i]
	}
	return result
}
