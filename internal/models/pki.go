package models

import (
	"context"
	"crypto"
	"crypto/x509"
	"time"
)

// PublicKeyInfrastructureConfig defines the PKI service configuration
type PublicKeyInfrastructureConfig struct {
	Provider string       `mapstructure:"provider"` // aws|gcp|azure|local (defaults to environment platform)
	Config   *BasicConfig `mapstructure:",remain"`  // Provider-specific configuration
}

func (e *PublicKeyInfrastructureConfig) GetProvider() string {
	if e == nil || len(e.Provider) == 0 {
		return "local"
	}
	return e.Provider
}

// PublicKeyInfrastructure provides PKI and certificate management capabilities
// across multiple cloud providers and local implementations
type PublicKeyInfrastructure interface {
	// crypto.Signer provides HSM-backed signing operations for KMS integration
	crypto.Signer

	// Initialize sets up the PKI service with encryption and vault dependencies
	Initialize(
		encryption EncryptionImpl,
		vault VaultImpl,
	) error

	// Shutdown closes connections and cleans up resources
	Shutdown() error

	// LoadCertificate loads an existing certificate from various sources
	// (inline PEM, file, vault secret, or HSM-backed key)
	LoadCertificate(config *CertificateConfig) (*LoadedCertificate, error)

	// IssueCertificate issues a new certificate based on the request
	// Supports both CSR-based and direct certificate generation
	IssueCertificate(ctx context.Context, req *CertificateRequest) (*IssuedCertificate, error)

	// GetCertificate retrieves a certificate by its identifier
	GetCertificate(ctx context.Context, certID string) (*IssuedCertificate, error)

	// RevokeCertificate revokes a certificate with the specified reason
	RevokeCertificate(ctx context.Context, certID string, reason RevocationReason) error

	// CreateCA creates a new Certificate Authority (AWS ACM PCA, GCP CA Service only)
	// Returns error for providers that don't support CA creation (Azure, local)
	CreateCA(ctx context.Context, req *CARequest) (*CertificateAuthority, error)

	// GetCA retrieves information about a Certificate Authority
	GetCA(ctx context.Context, caID string) (*CertificateAuthority, error)
}

// CertificateRequest represents a request to issue a certificate
type CertificateRequest struct {
	// CSR-based issuance (preferred for production)
	CSR []byte // PEM-encoded Certificate Signing Request (optional)

	// Direct certificate generation (if CSR not provided)
	CommonName string   // Subject Common Name
	SANs       []string // Subject Alternative Names (DNS names, IPs, URIs)

	// Certificate properties
	Validity    time.Duration      // Certificate validity period
	KeyUsage    x509.KeyUsage      // Key usage flags (e.g., DigitalSignature, KeyEncipherment)
	ExtKeyUsage []x509.ExtKeyUsage // Extended key usage (e.g., ServerAuth, ClientAuth)

	// Provider-specific metadata
	Metadata map[string]string
}

// IssuedCertificate represents a certificate issued by the PKI service
type IssuedCertificate struct {
	ID          string         // Provider-specific identifier (ARN, resource name, or vault key)
	Certificate []byte         // PEM-encoded certificate
	PrivateKey  []byte         // PEM-encoded private key (if exportable)
	Chain       [][]byte       // PEM-encoded certificate chain
	ExpiresAt   time.Time      // Certificate expiration time
	Metadata    map[string]any // Provider-specific metadata
}

// CARequest represents a request to create a Certificate Authority
type CARequest struct {
	CommonName string        // CA subject common name
	Validity   time.Duration // CA validity period

	// CA type
	Type CAType // Root or Subordinate

	// For subordinate CAs
	ParentCAID string // Parent CA identifier (required for subordinate)

	// Provider-specific settings
	Metadata map[string]string
}

// CertificateAuthority represents a Certificate Authority
type CertificateAuthority struct {
	ID          string         // Provider-specific identifier (ARN, pool name, etc.)
	Certificate []byte         // PEM-encoded CA certificate
	Type        CAType         // Root or Subordinate
	ExpiresAt   time.Time      // CA expiration time
	Status      CAStatus       // CA operational status
	Metadata    map[string]any // Provider-specific metadata
}

// CAType represents the type of Certificate Authority
type CAType string

const (
	CATypeRoot        CAType = "root"
	CATypeSubordinate CAType = "subordinate"
)

// CAStatus represents the operational status of a CA
type CAStatus string

const (
	CAStatusActive   CAStatus = "active"
	CAStatusDisabled CAStatus = "disabled"
	CAStatusDeleted  CAStatus = "deleted"
	CAStatusPending  CAStatus = "pending"
)

// RevocationReason represents why a certificate was revoked
type RevocationReason int

const (
	ReasonUnspecified RevocationReason = iota
	ReasonKeyCompromise
	ReasonCACompromise
	ReasonAffiliationChanged
	ReasonSuperseded
	ReasonCessationOfOperation
	ReasonCertificateHold
	ReasonRemoveFromCRL
	ReasonPrivilegeWithdrawn
	ReasonAACompromise
)
