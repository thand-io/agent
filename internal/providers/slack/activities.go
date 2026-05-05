package slack

import (
	"context"

	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

// slackProviderActivities exposes the Slack provider's outbound API calls as
// Temporal activities. Registered via RegisterActivities(runtime sdkConstants.Mode) so the shared
// notify workflow (and any provider workflow that dispatches via the
// SendNotificationActivityName helper) can retry/replay them safely.
type slackProviderActivities struct {
	provider *slackProvider
}

func (p *slackProvider) RegisterActivities(runtime sdkConstants.Mode) any {
	return &slackProviderActivities{provider: p}
}

// SendNotificationActivity is the Temporal activity wrapper around
// slackProvider.sendNotificationDirect. The exported method name must match
// models.SendNotificationActivityName so reflection-based registration in
// models.RegisterActivities produces the expected activity name.
func (a *slackProviderActivities) SendNotificationActivity(
	ctx context.Context,
	notification models.NotificationRequest,
) error {
	return a.provider.sendNotificationDirect(ctx, notification)
}
