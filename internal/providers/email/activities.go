package email

import (
	"context"

	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

// emailProviderActivities exposes the email provider's outbound API call as a
// Temporal activity. Registered via RegisterActivities(runtime sdkConstants.Mode) so the shared notify
// workflow can dispatch SendNotification with retry/replay determinism.
type emailProviderActivities struct {
	provider *emailProvider
}

func (p *emailProvider) RegisterActivities(runtime sdkConstants.Mode) any {
	return &emailProviderActivities{provider: p}
}

// SendNotificationActivity is the Temporal activity wrapper around
// emailProvider.sendNotificationDirect. The exported method name must match
// models.SendNotificationActivityName so reflection-based registration in
// models.RegisterActivities produces the expected activity name.
func (a *emailProviderActivities) SendNotificationActivity(
	ctx context.Context,
	notification models.NotificationRequest,
) error {
	return a.provider.sendNotificationDirect(ctx, notification)
}
