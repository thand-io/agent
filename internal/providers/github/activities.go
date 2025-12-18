package github

import (
	"context"

	"github.com/thand-io/agent/internal/models"
)

func (b *githubProvider) RegisterActivities(temporalClient models.TemporalImpl) error {
	return models.RegisterActivities(temporalClient, models.NewProviderActivities(b))
}

// GitHub uses static roles and permissions so we don't need to fetch them.
// Instead we will just return these in the synchronize call.
func (p *githubProvider) Synchronize(
	ctx context.Context,
	temporalService models.TemporalImpl,
	req *models.SynchronizeRequest,
) error {

	// Before we kick off the synchronize lets update the static roles and permissions

	p.SetRoles(GitHubOrganisationRoles)

	return models.Synchronize(ctx, temporalService, p, req)
}
