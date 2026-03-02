package models

import internal "github.com/thand-io/agent/internal/models"

type Workflow = internal.Workflow

// WorkflowExecutionInfo is an alias for the internal WorkflowExecutionInfo type.
// It contains metadata and status information about a workflow execution,
// including timing, approval status, and associated identities.
// See internal/models.WorkflowExecutionInfo for full documentation.
type WorkflowExecutionInfo = internal.WorkflowExecutionInfo

// WorkflowsResponse is an alias for the internal WorkflowsResponse type.
type WorkflowsResponse = internal.WorkflowsResponse

// WorkflowResponse is an alias for the internal WorkflowResponse type.
type WorkflowResponse = internal.WorkflowResponse

// WorkflowDefinitions is an alias for the internal WorkflowDefinitions type.
type WorkflowDefinitions = internal.WorkflowDefinitions

// ElevateWorkflowTask is the in-flight state of an elevation workflow, carrying
// the workflow DSL, context, session, and approval status. Pass it between the
// Elevate and Resume calls on sdk/api.Service.
type ElevateWorkflowTask = internal.ElevateWorkflowTask

// WorkflowRequest is returned by sdk/api.Service.Elevate. It contains the
// workflow task and the next URL for the client to follow (auth redirect or
// resume callback).
type WorkflowRequest = internal.WorkflowRequest
