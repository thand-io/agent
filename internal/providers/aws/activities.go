package aws

import (
	"context"

	"github.com/thand-io/agent/internal/models"
)

func (b *awsProvider) RegisterActivities(temporalClient models.TemporalImpl) error {
	return models.RegisterActivities(temporalClient, models.NewProviderActivities(b))
}

// Aws uses static roles and permissions so we don't need to fetch them.
// Instead we will just return these in the synchronize call.
func (p *awsProvider) Synchronize(
	ctx context.Context,
	temporalService models.TemporalImpl,
	req *models.SynchronizeRequest,
) error {

	// Before we kick off the synchronize lets update the static roles and permissions
	return PreSynchronizeActivities(ctx, temporalService, p)
}

func PreSynchronizeActivities(ctx context.Context, temporalService models.TemporalImpl, provider models.ProviderImpl) error {

	awsData, err := getSharedData()

	if err != nil {
		return err
	}

	provider.SetRoles(awsData.roles)
	provider.SetPermissions(awsData.permissions)

	return models.Synchronize(ctx, temporalService, provider, nil)
}
