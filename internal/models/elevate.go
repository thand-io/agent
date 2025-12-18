package models

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	"github.com/thand-io/agent/internal/common"
)

// Identities can describe users or groups

// ElevateRequest represents the request payload for /elevate endpoint
type ElevateStaticRequest struct {
	Role       string   `json:"role" form:"role"`
	Provider   string   `json:"provider" form:"provider"`
	Workflow   string   `json:"workflow" form:"workflow"`
	Reason     string   `json:"reason" form:"reason" binding:"required"`
	Duration   string   `json:"duration,omitempty" form:"duration,omitempty"`     // Duration in ISO 8601 format
	Identities []string `json:"identities,omitempty" form:"identities,omitempty"` // Optional identities to elevate, if empty the requesting user is used

	// Protected session
	Session *LocalSession `json:"session,omitempty" form:"session,omitempty"`
}

func (r *ElevateStaticRequest) GetUrlParams() url.Values {
	params := url.Values{
		"reason":     {r.Reason},
		"role":       {r.Role},
		"workflow":   {r.Workflow},
		"duration":   {r.Duration},
		"provider":   {r.Provider},
		"identities": {strings.Join(r.Identities, ",")},
		"session":    {r.GetEncodedSession()}, // TODO provide the current auth session
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
	WorkflowId string          `json:"id"`
	Status     ctx.StatusPhase `json:"status"`
	Output     map[string]any  `json:"output,omitempty"`
}

type ElevateRequest struct {
	Role          *Role         `json:"role"`
	Providers     []string      `json:"providers"`     // A role can be applied to multiple providers
	Authenticator string        `json:"authenticator"` // Which provider to use for authentication
	Workflow      string        `json:"workflow"`
	Reason        string        `json:"reason"`
	Duration      string        `json:"duration,omitempty"`   // Duration in ISO 8601 format
	Identities    []string      `json:"identities,omitempty"` // Optional identities to elevate, if empty the requesting user is used
	Session       *LocalSession `json:"session,omitempty"`
}

func (e *ElevateRequest) IsValid() bool {
	return !(e.Role == nil || len(e.Providers) == 0 || len(e.Reason) == 0)
}

func (e *ElevateRequest) AsDuration() (time.Duration, error) {
	return common.ValidateDuration(e.Duration)
}

func (e *ElevateRequest) AsMap() map[string]any {
	return map[string]any{
		"role":          e.Role, // get role
		"providers":     e.Providers,
		"authenticator": e.Authenticator,
		"workflow":      e.Workflow,
		"reason":        e.Reason,
		"duration":      e.Duration,
		"identities":    e.Identities,
	}
}

func (e *ElevateRequest) GetWorkflow() string {
	if len(e.Workflow) > 0 {
		return e.Workflow
	}
	if e.Role != nil && len(e.Role.Workflows) > 0 {
		return e.Role.Workflows[0]
	}
	return ""
}

// ResolveIdentities resolves and returns the list of identities for elevation
func (e *ElevateRequest) ResolveIdentities(ctx context.Context, providers map[string]Provider) map[string]*Identity {

	resolved := make(map[string]*Identity)

	if len(e.Identities) == 0 {
		return resolved
	}

	// Loop through identities and resolve them
	for _, identityName := range e.Identities {

		// If prefixed with provider, split it
		parts := strings.SplitN(identityName, ":", 2)
		if len(parts) == 2 {
			providerName := parts[0]
			unprefixedIdentity := parts[1]
			if provider, ok := providers[providerName]; ok {
				providerClient := provider.GetClient()
				if providerClient == nil {
					continue
				}
				identity, err := providerClient.GetIdentity(ctx, unprefixedIdentity)
				if err == nil && identity != nil {
					resolved[unprefixedIdentity] = identity
				}
			}
		}
		// Also fallback to unprefixed identity
		// Try to resolve across all providers
		fallbackIdentityName := parts[0]
		for _, provider := range providers {
			providerClient := provider.GetClient()
			if providerClient == nil {
				continue
			}
			identity, err := providerClient.GetIdentity(ctx, fallbackIdentityName)
			if err == nil && identity != nil {
				resolved[fallbackIdentityName] = identity
				break // Stop after first match
			}
		}
	}

	return resolved
}

type ElevateRequestInternal struct {
	ElevateRequest

	// Protected user
	User         *User      `json:"user,omitempty"`
	AuthorizedAt *time.Time `json:"authorized_at,omitempty"`
}

// ElevateDynamicRequestScopes represents the nested scopes structure for dynamic elevation
type ElevateDynamicRequestScopes struct {
	Groups  []string `form:"groups" json:"groups"`
	Users   []string `form:"users" json:"users"`
	Domains []string `form:"domains" json:"domains"`
}

type ElevateDynamicRequest struct {
	Authenticator string   `form:"authenticator" json:"authenticator"` // If not provided, use the users default auth context
	Workflow      string   `form:"workflow" json:"workflow" binding:"required"`
	Reason        string   `form:"reason" json:"reason" binding:"required"`
	Duration      string   `form:"duration" json:"duration" binding:"required"` // Duration in ISO 8601 format
	Identities    []string `form:"identities" json:"identities"`
	Providers     []string `form:"providers" json:"providers" binding:"required"`
	Inherits      []string `form:"inherits" json:"inherits"`
	Permissions   []string `form:"permissions" json:"permissions"` // Comma-separated permissions
	Groups        []string `form:"groups" json:"groups"`           // Comma-separated groups
	Resources     []string `form:"resources" json:"resources"`     // Comma-separated resources

	// Scopes - nested structure supporting both form bracket notation and JSON
	Scopes ElevateDynamicRequestScopes `form:"scopes" json:"scopes"`
}

type ElevateLLMRequest struct {
	Reason string `json:"reason"`
}
