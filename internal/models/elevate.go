package models

import (
	"net/url"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	"github.com/thand-io/agent/internal/common"
)

// ElevateRequest represents the request payload for /elevate endpoint
type ElevateStaticRequest struct {
	// Mode     string `json:"mode" form:"mode" binding:"required,oneof=static dynamic llm"`
	Role     string `json:"role" form:"role"`
	Provider string `json:"provider" form:"provider"`
	Reason   string `json:"reason" form:"reason" binding:"required"`
	Duration string `json:"duration,omitempty" form:"duration,omitempty"` // Duration in ISO 8601 format

	// Protected session
	Session *LocalSession `json:"session,omitempty" form:"session,omitempty"`
}

func (r *ElevateStaticRequest) GetUrlParams() url.Values {
	params := url.Values{
		// "mode":     {r.Mode},
		"reason":   {r.Reason},
		"role":     {r.Role},
		"duration": {r.Duration},
		"provider": {r.Provider},
		"session":  {r.GetEncodedSession()}, // TODO provide the current auth session
	}
	return params
}

func (r *ElevateStaticRequest) GetEncodedSession() string {
	return r.Session.GetEncodedLocalSession()
}

func (r *ElevateStaticRequest) GetSession() *LocalSession {
	return r.Session
}

// ElevateResponse represents the response for /elevate endpoint
type ElevateResponse struct {
	Status ctx.StatusPhase `json:"status"`
	Output map[string]any  `json:"output,omitempty"`
}

type ElevateRequest struct {
	Role     *Role         `json:"role"`
	Provider string        `json:"provider"`
	Reason   string        `json:"reason"`
	Duration string        `json:"duration,omitempty"` // Duration in ISO 8601 format
	Session  *LocalSession `json:"session,omitempty"`
}

func (e *ElevateRequest) IsValid() bool {
	return !(e.Role == nil || len(e.Provider) == 0 || len(e.Reason) == 0)
}

func (e *ElevateRequest) AsDuration() (time.Duration, error) {
	return common.ValidateDuration(e.Duration)
}

type ElevateRequestInternal struct {
	ElevateRequest

	// Protected user
	User         *User      `json:"user,omitempty"`
	AuthorizedAt *time.Time `json:"authorized_at,omitempty"`
}

type ElevateDynamicRequest struct {
	Workflow    string   `form:"workflow" json:"workflow" binding:"required"`
	Reason      string   `form:"reason" json:"reason" binding:"required"`
	Duration    string   `form:"duration" json:"duration" binding:"required"` // Duration in ISO 8601 format
	Providers   []string `form:"providers" json:"providers" binding:"required"`
	Inherits    []string `form:"inherits" json:"inherits"`
	Permissions []string `form:"permissions" json:"permissions"` // Comma-separated permissions
	Resources   []string `form:"resources" json:"resources"`     // Comma-separated resources
	Groups      []string `form:"groups" json:"groups"`
	Users       []string `form:"users" json:"users"`
}

type ElevateLLMRequest struct {
	Reason string `json:"reason"`
}
