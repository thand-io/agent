package models

import (
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
)

const TemporalExecuteElevationWorkflowName = "ExecuteElevationWorkflow"

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

	ApiKey              string `mapstructure:"api_key" default:""`
	MtlsCertificate     string `mapstructure:"mtls_cert" default:""`
	MtlsCertificatePath string `mapstructure:"mtls_cert_path" default:""`

	// DisableVersioning disables worker versioning/deployments for testing
	DisableVersioning bool `mapstructure:"disable_versioning" default:"false"`
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
