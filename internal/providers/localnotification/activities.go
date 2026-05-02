package localnotification

import (
	"context"

	"github.com/thand-io/agent/internal/models"
)

type localNotificationProviderActivities struct {
	provider *localNotificationProvider
}

func (a *localNotificationProviderActivities) SendNotificationActivity(
	ctx context.Context,
	notification models.NotificationRequest,
) error {
	return a.provider.SendNotification(ctx, notification)
}
