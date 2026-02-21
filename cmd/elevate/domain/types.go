package domain

import "time"

// Local helper protocol frame types. These are intentionally private to cmd/elevate.
const (
	FrameTypeRequest        = "request"
	FrameTypeChallenge      = "challenge"
	FrameTypeSignedResponse = "signed_response"
	FrameTypeResult         = "result"
)

type RequestFrame struct {
	Type            string `json:"type"`
	Action          string `json:"action"`
	WorkflowID      string `json:"workflow_id"`
	RequestID       string `json:"request_id"`
	Username        string `json:"username"`
	DurationSeconds int64  `json:"duration_seconds,omitempty"`
}

type ChallengeFrame struct {
	Type  string `json:"type"`
	Nonce string `json:"nonce"`
}

type SignedResponseFrame struct {
	Type          string `json:"type"`
	KeyID         string `json:"key_id"`
	Signature     string `json:"signature"`
	SignedPayload string `json:"signed_payload"`
}

type ResultFrame struct {
	Type      string `json:"type"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

type GrantRequest struct {
	RequestID       string
	WorkflowID      string
	Username        string
	DurationSeconds int64
}

type RevokeRequest struct {
	RequestID  string
	WorkflowID string
	Username   string
}

type GrantResult struct {
	RequestID string
	Username  string
	Expiry    time.Time
}

type GrantState struct {
	RequestID            string    `json:"request_id"`
	WorkflowID           string    `json:"workflow_id"`
	Username             string    `json:"username"`
	GrantedAtWallUTC     time.Time `json:"granted_at_wall_utc"`
	GrantedAtMonoNS      int64     `json:"granted_at_mono_ns"`
	DurationSeconds      int64     `json:"duration_seconds"`
	WasAlreadyPrivileged bool      `json:"was_already_privileged"`
}
