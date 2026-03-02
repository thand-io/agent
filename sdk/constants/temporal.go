package constants

import "go.temporal.io/sdk/temporal"

const TemporalDeploymentName = "thand-agent"

const TemporalCleanupActivityName = "cleanup"
const TemporalHttpActivityName = "http"
const TemporalGrpcActivityName = "grpc"
const TemporalAsyncionActivityName = "asyncio"
const TemporalOpenAPIActivityName = "openapi"
const TemporalSignalWorkflowActivityName = "signal-workflow"

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
