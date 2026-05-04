package models

import (
	"fmt"
	"strings"
	"time"
)

const (
	LocalPresenceApprovalMethod = "local_presence"
	LocalPresenceDefaultTimeout = 2 * time.Minute
)

type LocalPresenceApprovalConfig struct {
	Provider string `json:"provider,omitempty"`
	Device   string `json:"device,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
}

type LocalPresenceApprovalRequest struct {
	ChallengeID string        `json:"challenge_id,omitempty"`
	DeviceID    string        `json:"device_id,omitempty"`
	WorkflowID  string        `json:"workflow_id,omitempty"`
	TaskName    string        `json:"task_name,omitempty"`
	Prompt      string        `json:"prompt,omitempty"`
	Timeout     time.Duration `json:"timeout,omitempty"`
	RequestedBy string        `json:"requested_by,omitempty"`
	RoleName    string        `json:"role_name,omitempty"`
	Reason      string        `json:"reason,omitempty"`
	// SignalTarget, when populated, instructs the local-presence provider
	// to deliver the approve/deny outcome back to the originating workflow
	// via the registered signal-workflow activity. This mirrors the slack
	// and email approval flows where the user click signals the workflow
	// listener; for presence the broker's Touch ID result is what triggers
	// the signal.
	SignalTarget *LocalPresenceSignalTarget `json:"signal_target,omitempty"`
}

// LocalPresenceSignalTarget describes the workflow execution that should
// receive a cloudevent signal once the presence challenge has been resolved.
// SignalName defaults to the listener's expected channel and EventType to
// the approval cloudevent type when either is empty.
type LocalPresenceSignalTarget struct {
	WorkflowID  string `json:"workflow_id,omitempty"`
	RunID       string `json:"run_id,omitempty"`
	SignalName  string `json:"signal_name,omitempty"`
	EventType   string `json:"event_type,omitempty"`
	EventSource string `json:"event_source,omitempty"`
	// UserBase64 is the base64-encoded approver identity that should be
	// attached to the signaled cloudevent as the `user` extension. The
	// approve task ListenTaskHandler in approvals.go requires this
	// extension to identify who approved/denied; without it the listener
	// logs "Approval event missing user extension" and loops back.
	UserBase64 string `json:"user_base64,omitempty"`
}

func (r LocalPresenceApprovalRequest) EffectiveTimeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}
	return LocalPresenceDefaultTimeout
}

type LocalPresenceApprovalResponse struct {
	ChallengeID     string    `json:"challenge_id,omitempty"`
	DeviceID        string    `json:"device_id,omitempty"`
	Approved        bool      `json:"approved"`
	AuthenticatedAt time.Time `json:"authenticated_at,omitempty"`
	FailureReason   string    `json:"failure_reason,omitempty"`
	Method          string    `json:"method,omitempty"`
	TimedOut        bool      `json:"timed_out,omitempty"`
}

func LocalPresenceApprovalKey(deviceID, taskName string) string {
	return fmt.Sprintf("%s:%s:%s", LocalPresenceApprovalMethod, strings.TrimSpace(deviceID), strings.TrimSpace(taskName))
}

func (r LocalPresenceApprovalResponse) AsApprovalMap(now time.Time) map[string]any {
	method := strings.TrimSpace(r.Method)
	if method == "" {
		method = LocalPresenceApprovalMethod
	}

	timestamp := now.UTC()
	if r.Approved && !r.AuthenticatedAt.IsZero() {
		timestamp = r.AuthenticatedAt.UTC()
	}

	result := map[string]any{
		"approved":  r.Approved,
		"timestamp": timestamp.Format(time.RFC3339),
		"method":    method,
		"device_id": r.DeviceID,
	}
	if strings.TrimSpace(r.ChallengeID) != "" {
		result["challenge_id"] = r.ChallengeID
	}
	if strings.TrimSpace(r.FailureReason) != "" {
		result["failure_reason"] = r.FailureReason
	}
	if r.TimedOut {
		result["timed_out"] = true
	}
	return result
}
