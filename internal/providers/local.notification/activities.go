package localnotification

import (
	"context"

	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

type localNotificationProviderActivities struct {
	provider *localNotificationProvider
}

func (p *localNotificationProvider) RegisterActivities(runtime sdkConstants.Mode) any {
	return &localNotificationProviderActivities{provider: p}
}

func (a *localNotificationProviderActivities) SendNotificationActivity(
	ctx context.Context,
	notification models.NotificationRequest,
) error {
	return a.provider.SendNotification(ctx, notification)
}
