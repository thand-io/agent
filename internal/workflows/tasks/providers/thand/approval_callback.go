package thand

import (
	"github.com/thand-io/agent/internal/models"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
)

// approvalEventSource is the cloudevent source string used for approval events
// emitted by the agent. It matches the value used by createCallbackUrl in
// approvals_slack.go / approvals_email.go so that listener task filters and
// downstream consumers see a consistent source regardless of which channel
// (slack, email, local presence) produced the signal.
const approvalEventSource = "urn:thand:agent"

// newPresenceApprovalSignalTarget builds the SignalTarget that the
// local-presence provider should use to deliver the approve/deny outcome
// back to the originating workflow once the macOS broker validates the
// Touch ID prompt. The values mirror the slack/email URL flow which signals
// `TemporalEventSignalName` with a `ThandApprovalEventType` cloudevent — so
// the existing approve task ListenTaskHandler can consume it without any
// listener-side changes.
//
// Returns nil when the workflow task does not have a workflow id; in that
// case the presence challenge degrades to fire-and-forget (current
// pre-signal behaviour).
func newPresenceApprovalSignalTarget(
	workflowTask *models.ElevateWorkflowTask,
	toIdentity *models.Identity,
) *models.LocalPresenceSignalTarget {
	if workflowTask == nil {
		return nil
	}
	workflowID := workflowTask.GetWorkflowID()
	if len(workflowID) == 0 {
		return nil
	}
	var userBase64 string
	if toIdentity != nil {
		userBase64 = toIdentity.EncodeBase64()
	}
	return &models.LocalPresenceSignalTarget{
		WorkflowID:  workflowID,
		RunID:       sdkWorkflowsModel.TemporalEmptyRunId,
		SignalName:  sdkWorkflowsModel.TemporalEventSignalName,
		EventType:   ThandApprovalEventType,
		EventSource: approvalEventSource,
		UserBase64:  userBase64,
	}
}
