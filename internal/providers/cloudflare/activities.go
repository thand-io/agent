package cloudflare

import (
	"context"

	"github.com/thand-io/agent/internal/models"
)

func (b *cloudflareProvider) RegisterActivities(temporalClient models.TemporalImpl) error {
	return models.RegisterActivities(temporalClient, models.NewProviderActivities(b))
}

// Cloudflare uses static roles and permissions so we don't need to fetch them.
// Instead we will just return these in the synchronize call.
func (p *cloudflareProvider) Synchronize(
	ctx context.Context,
	temporalService models.TemporalImpl,
	req *models.SynchronizeRequest,
) error {
	return models.Synchronize(ctx, temporalService, p, req)
}
