package gcp

import (
	"context"

	"github.com/thand-io/agent/internal/models"
)

func (b *gcpProvider) RegisterActivities(temporalClient models.TemporalImpl) error {
	return models.RegisterActivities(temporalClient, models.NewProviderActivities(b))
}

// GCP uses static roles and permissions so we don't need to fetch them.
// Instead we will just return these in the synchronize call.
func (p *gcpProvider) Synchronize(
	ctx context.Context,
	temporalService models.TemporalImpl,
	req *models.SynchronizeRequest,
) error {

	// Before we kick off the synchronize lets update the static roles and permissions
	return PreSynchronizeActivities(ctx, temporalService, p, req)
}

func PreSynchronizeActivities(
	ctx context.Context,
	temporalService models.TemporalImpl,
	provider models.ProviderImpl,
	req *models.SynchronizeRequest,
) error {

	config := provider.GetConfig()
	stage := config.GetStringWithDefault("stage", "GA")

	gcpData, err := getSharedData(stage)

	if err != nil {
		return err
	}

	provider.SetRoles(gcpData.roles)
	provider.SetPermissions(gcpData.permissions)

	return models.Synchronize(ctx, temporalService, provider, req)
}
