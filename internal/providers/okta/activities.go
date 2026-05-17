package okta

import (
	"context"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

func (b *oktaProvider) RegisterActivities(runtime sdkConstants.Mode) any {
	return &oktaProviderActivities{provider: b}
}

// Okta uses static roles and permissions so we don't need to fetch them.
// Instead we will just return these in the synchronize call.
func (p *oktaProvider) Synchronize(
	ctx context.Context,
	temporalService models.TemporalImpl,
	req *models.SynchronizeRequest,
) error {
	return preSynchronizeActivities(ctx, temporalService, p, req)
}

func preSynchronizeActivities(
	ctx context.Context,
	temporalService models.TemporalImpl,
	p *oktaProvider,
	req *models.SynchronizeRequest,
) error {
	// Load static roles first so they are available before the PreSync job runs
	p.SetRoles(p.getStaticRoles())
	p.SetPermissions(p.getStaticPermissions())

	logrus.Infoln("Okta shared data set for provider: " + p.GetIdentifier())

	// Carry on with the normal synchronization flow, which will run the PreSync job and then the main Sync job
	return models.Synchronize(ctx, temporalService, p, req)
}
