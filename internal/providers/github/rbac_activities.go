package github

import (
	"context"

	"github.com/thand-io/agent/internal/models"
)

// githubProviderActivities exposes granular GitHub provider operations as
// individual Temporal activities, one per resource target.
type githubProviderActivities struct {
	provider *githubProvider
}

// ─────────────────────────────────────────────────────────────────────────────
// Request / response types
// ─────────────────────────────────────────────────────────────────────────────

type AuthorizeResourceRequest struct {
	Username string       `json:"username"`
	Resource string       `json:"resource"`
	Role     *models.Role `json:"role"`
}

type RevokeResourceRequest struct {
	Username string `json:"username"`
	Resource string `json:"resource"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Activity implementations
// ─────────────────────────────────────────────────────────────────────────────

// AuthorizeResource grants the user access to a single GitHub resource
// (org membership, team membership, or repository collaboration).
func (a *githubProviderActivities) AuthorizeResource(
	ctx context.Context,
	req *AuthorizeResourceRequest,
) error {
	return a.provider.authorizeResource(ctx, req.Username, req.Resource, req.Role)
}

// RevokeResource removes the user's access from a single GitHub resource.
func (a *githubProviderActivities) RevokeResource(
	ctx context.Context,
	req *RevokeResourceRequest,
) error {
	return a.provider.revokeResource(ctx, req.Username, req.Resource)
}
