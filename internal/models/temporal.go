package models

import (
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

const TemporalExecuteElevationWorkflowName = "ExecuteElevationWorkflow"

const TemporalIsApprovedQueryName = "isApproved"
const TemporalGetWorkflowTaskQueryName = "getWorkflowTask"

// TemporalAuthAPIKey represents API Key authentication configuration
type TemporalAuthAPIKey struct {
	ApiKey string `mapstructure:"api_key" default:""`
}

// TemporalAuthMTLSInline represents inline mTLS certificates (PEM format)
type TemporalAuthMTLSInline struct {
	MtlsCert string `mapstructure:"mtls_cert" default:""`
	MtlsKey  string `mapstructure:"mtls_key" default:""`
}

// TemporalAuthMTLSFile represents file-based mTLS certificates
type TemporalAuthMTLSFile struct {
	MtlsCertFile string `mapstructure:"mtls_cert_file" default:""`
	MtlsKeyFile  string `mapstructure:"mtls_key_file" default:""`
}

// TemporalAuthMTLSVault represents Vault-backed key with cert in secret
type TemporalAuthMTLSVault struct {
	MtlsVaultName     string `mapstructure:"mtls_vault_name" default:""` // HSM key resource ID (AWS KMS ARN, Azure Key Vault key URL, GCP KMS resource name)
	MtlsVaultType     string `mapstructure:"mtls_vault_type" default:""` // Optional: auto-detected from platform
	MtlsVaultPassword string `mapstructure:"mtls_vault_password" default:""`
}

type TemporalConfig struct {
	Host      string `mapstructure:"host" default:"localhost"`
	Port      int    `mapstructure:"port" default:"7233"`
	Namespace string `mapstructure:"namespace" default:"default"`

	// Authentication configurations - embed inline types for backward compatibility
	TemporalAuthAPIKey     `mapstructure:",squash"` // API key
	TemporalAuthMTLSInline `mapstructure:",squash"` // Inline mTLS
	TemporalAuthMTLSFile   `mapstructure:",squash"` // File-based mTLS
	TemporalAuthMTLSVault  `mapstructure:",squash"` // Vault-based mTLS

	// DisableVersioning disables worker versioning/deployments for testing
	DisableVersioning bool `mapstructure:"disable_versioning" default:"false"`
}

// HasMtlsConfig returns true if any mTLS configuration is present
func (t *TemporalConfig) HasMtlsConfig() bool {
	return len(t.MtlsCert) > 0 || len(t.MtlsKey) > 0 ||
		len(t.MtlsCertFile) > 0 || len(t.MtlsKeyFile) > 0 ||
		len(t.MtlsVaultName) > 0 || len(t.MtlsVaultType) > 0 ||
		len(t.MtlsVaultPassword) > 0
}

type TemporalImpl interface {
	Initialize() error
	Shutdown() error

	GetClient() client.Client
	HasClient() bool

	GetWorker() worker.Worker
	HasWorker() bool

	GetHostPort() string
	GetNamespace() string
	GetTaskQueue() string

	IsVersioningDisabled() bool
}

type TemporalTerminationRequest struct {
	Reason      string     `json:"reason,omitempty"`
	EntryPoint  string     `json:"entrypoint,omitempty"`
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
}
