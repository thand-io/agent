package okta

import (
	"context"

	"github.com/thand-io/agent/internal/models"
)

func (b *oktaProvider) RegisterActivities() any {
	return &oktaProviderActivities{provider: b}
}

// Okta uses static roles and permissions so we don't need to fetch them.
// Instead we will just return these in the synchronize call.
func (p *oktaProvider) Synchronize(
	ctx context.Context,
	temporalService models.TemporalImpl,
	req *models.SynchronizeRequest,
) error {
	return PreSynchronizeActivities(ctx, temporalService, p, req)
}

func PreSynchronizeActivities(
	ctx context.Context,
	temporalService models.TemporalImpl,
	provider models.Provider,
	req *models.SynchronizeRequest,
) error {
	p := provider.(*oktaProvider)

	// Load static roles first so they are available before the PreSync job runs
	provider.SetRoles(p.getStaticRoles())
	provider.SetPermissions(p.getStaticPermissions())

	// Carry on with the normal synchronization flow, which will run the PreSync job and then the main Sync job
	return models.Synchronize(ctx, temporalService, provider, req)
}
