package localpresence

import (
	"context"
	"errors"

	"github.com/thand-io/agent/internal/localbroker"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/temporal"
)

type localPresenceProviderActivities struct {
	provider *localPresenceProvider
}

func (p *localPresenceProvider) RegisterActivities() any {
	return &localPresenceProviderActivities{provider: p}
}

func (a *localPresenceProviderActivities) CheckLocalPresenceActivity(
	ctx context.Context,
	req *models.LocalPresenceApprovalRequest,
) (*models.LocalPresenceApprovalResponse, error) {
	resp, err := a.provider.checkLocalPresenceDirect(ctx, req)
	if err != nil {
		return nil, wrapLocalPresenceProviderActivityError(err)
	}

	return resp, nil
}

// SendNotificationActivity is the Temporal activity wrapper around
// localPresenceProvider.sendNotificationDirect. The exported method name must
// match models.SendNotificationActivityName so reflection-based registration
// in the worker locates it.
func (a *localPresenceProviderActivities) SendNotificationActivity(
	ctx context.Context,
	notification models.NotificationRequest,
) error {
	return a.provider.sendNotificationDirect(ctx, notification)
}

func wrapLocalPresenceProviderActivityError(err error) error {
	if err == nil {
		return nil
	}

	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) {
		return err
	}

	if localbroker.IsRetryableError(err) {
		return err
	}

	return temporal.NewNonRetryableApplicationError(
		err.Error(),
		localPresenceProviderErrorType,
		err,
	)
}
