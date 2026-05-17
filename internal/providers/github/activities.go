package github

import (
	"context"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

func (b *githubProvider) RegisterActivities(runtime sdkConstants.Mode) any {
	return &githubProviderActivities{provider: b}
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

	logrus.Infoln("GitHub shared data set for provider: " + p.GetIdentifier())

	return models.Synchronize(ctx, temporalService, p, req)
}
