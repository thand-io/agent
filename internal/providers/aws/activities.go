package aws

import (
	"context"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

func (b *awsProvider) RegisterActivities(runtime sdkConstants.Mode) any {
	return &awsProviderActivities{provider: b}
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

func PreSynchronizeActivities(ctx context.Context, temporalService models.TemporalImpl, provider models.Provider) error {

	logrus.Infoln("Starting pre-synchronization for provider: " + provider.GetIdentifier())

	awsData, err := getSharedData()

	if err != nil {
		logrus.WithError(err).Errorln("Error getting AWS shared data for synchronization for provider: " + provider.GetIdentifier())
		return err
	}

	logrus.Infoln("AWS shared data retrieved for provider: " + provider.GetIdentifier())

	provider.SetRoles(awsData.roles)

	logrus.Infoln("AWS roles set for provider: " + provider.GetIdentifier())

	provider.SetPermissions(awsData.permissions)

	logrus.Infoln("AWS shared data set for provider: " + provider.GetIdentifier())

	return models.Synchronize(ctx, temporalService, provider, nil)
}
