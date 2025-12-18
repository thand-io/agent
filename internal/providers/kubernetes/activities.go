package kubernetes

import (
	"context"

	"github.com/thand-io/agent/internal/models"
)

func (b *kubernetesProvider) RegisterActivities(temporalClient models.TemporalImpl) error {
	return models.RegisterActivities(temporalClient, models.NewProviderActivities(b))
}

// Kubernetes uses static roles and permissions so we don't need to fetch them.
// Instead we will just return these in the synchronize call.
func (p *kubernetesProvider) Synchronize(
	ctx context.Context,
	temporalService models.TemporalImpl,
	req *models.SynchronizeRequest,
) error {
	return models.Synchronize(ctx, temporalService, p, req)
}
