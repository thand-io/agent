package models

import (
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
)

const TemporalDeploymentName = "thand-agent"

const TemporalEmptyRunId = ""

const TemporalExecuteElevationWorkflowName = "ExecuteElevationWorkflow"

const TemporalCleanupActivityName = "cleanup"
const TemporalHttpActivityName = "http"
const TemporalGrpcActivityName = "grpc"
const TemporalAsyncionActivityName = "asyncio"
const TemporalOpenAPIActivityName = "openapi"

const TemporalResumeSignalName = "resume"
const TemporalEventSignalName = "event"
const TemporalTerminateSignalName = "terminate"

const TemporalIsApprovedQueryName = "isApproved"
const TemporalGetWorkflowTaskQueryName = "getWorkflowTask"

var TypedSearchAttributeStatus = temporal.NewSearchAttributeKeyKeyword("status")
var TypedSearchAttributeTask = temporal.NewSearchAttributeKeyKeyword("task")
var TypedSearchAttributeUser = temporal.NewSearchAttributeKeyKeyword(VarsContextUser)
var TypedSearchAttributeRole = temporal.NewSearchAttributeKeyKeyword(VarsContextRole)
var TypedSearchAttributeWorkflow = temporal.NewSearchAttributeKeyKeyword(VarsContextWorkflow)
var TypedSearchAttributeProviders = temporal.NewSearchAttributeKeyKeywordList(VarsContextProviders)
var TypedSearchAttributeReason = temporal.NewSearchAttributeKeyString("reason") // Description or reason for the workflow
var TypedSearchAttributeDuration = temporal.NewSearchAttributeKeyInt64("duration")
var TypedSearchAttributeIdentities = temporal.NewSearchAttributeKeyKeywordList("identities")
var TypedSearchAttributeApproved = temporal.NewSearchAttributeKeyBool(VarsContextApproved)

type TemporalConfig struct {
	Host      string `mapstructure:"host" default:"localhost"`
	Port      int    `mapstructure:"port" default:"7233"`
	Namespace string `mapstructure:"namespace" default:"default"`

	// API Key authentication
	ApiKey string `mapstructure:"api_key" default:""`

	// mTLS - inline certificates (PEM format)
	MtlsCert string `mapstructure:"mtls_cert" default:""`
	MtlsKey  string `mapstructure:"mtls_key" default:""`

	// mTLS - file paths
	MtlsCertFile string `mapstructure:"mtls_cert_file" default:""`
	MtlsKeyFile  string `mapstructure:"mtls_key_file" default:""`

	// mTLS - CSP vault secret (combined cert+key in PEM format)
	MtlsCertKeySecret string `mapstructure:"mtls_cert_key_secret" default:""`

	// mTLS - HSM-backed key (cert in secret, key in HSM)
	MtlsCertSecret string `mapstructure:"mtls_cert_secret" default:""`  // Vault secret containing only certificate
	MtlsHSMKeyID   string `mapstructure:"mtls_hsm_key_id" default:""`   // HSM key resource ID (AWS KMS ARN, Azure Key Vault key URL, GCP KMS resource name)
	MtlsHSMKeyType string `mapstructure:"mtls_hsm_key_type" default:""` // Optional: auto-detected from platform

	// CA certificate for server verification (optional)
	MtlsCA       string `mapstructure:"mtls_ca" default:""`
	MtlsCAFile   string `mapstructure:"mtls_ca_file" default:""`
	MtlsCASecret string `mapstructure:"mtls_ca_secret" default:""`

	// DisableVersioning disables worker versioning/deployments for testing
	DisableVersioning bool `mapstructure:"disable_versioning" default:"false"`
}

// ToCertificateConfig converts TemporalConfig mTLS fields to CertificateConfig
// for use with the certificate loader service
func (t *TemporalConfig) ToCertificateConfig(platformConfig *BasicConfig) *CertificateConfig {
	return &CertificateConfig{
		CertPEM:        t.MtlsCert,
		KeyPEM:         t.MtlsKey,
		CertFile:       t.MtlsCertFile,
		KeyFile:        t.MtlsKeyFile,
		CertKeySecret:  t.MtlsCertKeySecret,
		CertSecret:     t.MtlsCertSecret,
		HSMKeyID:       t.MtlsHSMKeyID,
		HSMKeyType:     t.MtlsHSMKeyType,
		PlatformConfig: platformConfig,
		CAPEM:          t.MtlsCA,
		CAFile:         t.MtlsCAFile,
		CASecret:       t.MtlsCASecret,
	}
}

// HasMtlsConfig returns true if any mTLS configuration is present
func (t *TemporalConfig) HasMtlsConfig() bool {
	return len(t.MtlsCert) > 0 || len(t.MtlsKey) > 0 ||
		len(t.MtlsCertFile) > 0 || len(t.MtlsKeyFile) > 0 ||
		len(t.MtlsCertKeySecret) > 0 ||
		len(t.MtlsCertSecret) > 0 || len(t.MtlsHSMKeyID) > 0
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
