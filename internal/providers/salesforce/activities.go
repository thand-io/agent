package salesforce

import (
	"context"

	"github.com/thand-io/agent/internal/models"
)

func (b *salesForceProvider) RegisterActivities() any {
	return &salesForceProviderActivities{provider: b}
}

// Salesforce uses static roles and permissions so we don't need to fetch them.
// Instead we will just return these in the synchronize call.
func (p *salesForceProvider) Synchronize(
	ctx context.Context,
	temporalService models.TemporalImpl,
	req *models.SynchronizeRequest,
) error {
	return models.Synchronize(ctx, temporalService, p, req)
}
