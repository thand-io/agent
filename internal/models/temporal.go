package models

import (
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
)

var TemporalEmptyRunId = ""

var TemporalExecuteElevationWorkflowName = "ExecuteElevationWorkflow"

var TemporalCleanupActivityName = "cleanup"
var TemporalHttpActivityName = "http"
var TemporalGrpcActivityName = "grpc"
var TemporalAsyncionActivityName = "asyncio"
var TemporalOpenAPIActivityName = "openapi"

var TemporalResumeSignalName = "resume"
var TemporalEventSignalName = "event"
var TemporalTerminateSignalName = "terminate"

var TemporalIsApprovedQueryName = "isApproved"
var TemporalGetWorkflowTaskQueryName = "getWorkflowTask"

var TypedSearchAttributeStatus = temporal.NewSearchAttributeKeyKeyword("status")

var TypedSearchAttributeTask = temporal.NewSearchAttributeKeyString("task")
var TypedSearchAttributeUser = temporal.NewSearchAttributeKeyString("user")
var TypedSearchAttributeRole = temporal.NewSearchAttributeKeyString("role")
var TypedSearchAttributeWorkflow = temporal.NewSearchAttributeKeyString("workflow")
var TypedSearchAttributeProvider = temporal.NewSearchAttributeKeyString("provider")

var TypedSearchAttributeApproved = temporal.NewSearchAttributeKeyBool("approved")

type TemporalConfig struct {
	Host      string `mapstructure:"host" default:"localhost"`
	Port      int    `mapstructure:"port" default:"7233"`
	Namespace string `mapstructure:"namespace" default:"default"`

	ApiKey              string `mapstructure:"api_key" default:""`
	MtlsCertificate     string `mapstructure:"mtls_cert" default:""`
	MtlsCertificatePath string `mapstructure:"mtls_cert_path" default:""`
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
}

type TemporalTerminationRequest struct {
	Reason      string    `json:"reason,omitempty"`
	ScheduledAt time.Time `json:"scheduled_at,omitempty"`
}
