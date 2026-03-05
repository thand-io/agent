package azure

import (
	"context"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

func (b *azureProvider) RegisterActivities() any {
	return &azureProviderActivities{provider: b}
}

// Azure uses static roles and permissions so we don't need to fetch them.
// Instead we will just return these in the synchronize call.
func (p *azureProvider) Synchronize(
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
	provider models.Provider,
	req *models.SynchronizeRequest,
) error {

	azureData, err := getSharedData()

	if err != nil {
		logrus.WithError(err).Errorln("Error getting Azure shared data for synchronization for provider: " + provider.GetIdentifier())
		return err
	}

	provider.SetRoles(azureData.roles)
	provider.SetPermissions(azureData.permissions)

	logrus.Infoln("Azure shared data set for provider: " + provider.GetIdentifier())

	return models.Synchronize(ctx, temporalService, provider, req)
}
