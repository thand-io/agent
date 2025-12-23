package aws

import (
	"context"

	"github.com/sirupsen/logrus"
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
	if err := PreSynchronizeActivities(ctx, temporalService, p); err != nil {
		return err
	}

	// Discover AWS accounts if explicitly enabled via config
	providerWrapper := p.GetProviderWrapper() // Get the Provider config wrapper
	logrus.Debugf("Account discovery check: providerWrapper=%v", providerWrapper != nil)
	if providerWrapper != nil {
		logrus.Debugf("DiscoverAccounts config value: %v", providerWrapper.DiscoverAccounts)
		logrus.Debugf("HasCapability(Accounts): %v", p.HasCapability(models.ProviderCapabilityAccounts))
		logrus.Debugf("ShouldDiscoverAccounts(): %v", providerWrapper.ShouldDiscoverAccounts())
	}
	if providerWrapper != nil && providerWrapper.ShouldDiscoverAccounts() {
		logrus.Info("Account discovery is enabled, refreshing AWS accounts from Organizations")
		if err := p.RefreshAccounts(ctx); err != nil {
			// Don't fail sync - account discovery is not critical
			// This allows the provider to work even if Organizations access is not available
			logrus.Warnf("Failed to discover AWS accounts: %v", err)
		}
	}

	return nil
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
