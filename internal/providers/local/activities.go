package local

import (
	"context"
	"errors"

	"github.com/thand-io/agent/internal/localbroker"
	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
	"go.temporal.io/sdk/temporal"
)

const (
	AuthorizeRoleActivityName = "AuthorizeRoleActivity"
	RevokeRoleActivityName    = "RevokeRoleActivity"
)

type localProviderActivities struct {
	provider *localProvider
}

func (p *localProvider) RegisterActivities(runtime sdkConstants.Mode) any {
	return &localProviderActivities{provider: p}
}

// Override
func (a *localProviderActivities) AuthorizeRoleActivity(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
) (*models.AuthorizeRoleResponse, error) {
	resp, err := a.provider.authorizeRoleDirect(ctx, req)
	if err != nil {
		return nil, wrapLocalProviderActivityError(err)
	}

	return resp, nil
}

// Override
func (a *localProviderActivities) RevokeRoleActivity(
	ctx context.Context,
	req *models.RevokeRoleRequest,
) (*models.RevokeRoleResponse, error) {
	resp, err := a.provider.revokeRoleDirect(ctx, req)
	if err != nil {
		return nil, wrapLocalProviderActivityError(err)
	}

	return resp, nil
}

func wrapLocalProviderActivityError(err error) error {
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
		"LocalProviderActivityError",
		err,
	)
}
