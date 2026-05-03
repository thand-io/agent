package local

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/thand-io/agent/internal/localbroker"
	"github.com/thand-io/agent/internal/models"
	sdkWorkflowsRunner "github.com/thand-io/agent/sdk/workflows/runner"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	AuthorizeRoleActivityName = "AuthorizeRoleActivity"
	RevokeRoleActivityName    = "RevokeRoleActivity"
)

type localProviderActivities struct {
	provider *localProvider
}

func (p *localProvider) RegisterActivities() any {
	return &localProviderActivities{provider: p}
}

func (p *localProvider) authorizeRoleTemporal(
	wfCtx workflow.Context,
	req *models.AuthorizeRoleRequest,
) (*models.AuthorizeRoleResponse, error) {
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
	}
	wfCtx = workflow.WithActivityOptions(wfCtx, ao)

	var resp models.AuthorizeRoleResponse
	if err := workflow.ExecuteActivity(
		wfCtx,
		models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), AuthorizeRoleActivityName),
		req,
	).Get(wfCtx, &resp); err != nil {
		return nil, fmt.Errorf("%s activity failed: %w", AuthorizeRoleActivityName, err)
	}

	return &resp, nil
}

func (p *localProvider) revokeRoleTemporal(
	wfCtx workflow.Context,
	req *models.RevokeRoleRequest,
) (*models.RevokeRoleResponse, error) {
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
	}
	wfCtx = workflow.WithActivityOptions(wfCtx, ao)

	var resp models.RevokeRoleResponse
	if err := workflow.ExecuteActivity(
		wfCtx,
		models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), RevokeRoleActivityName),
		req,
	).Get(wfCtx, &resp); err != nil {
		return nil, fmt.Errorf("%s activity failed: %w", RevokeRoleActivityName, err)
	}

	return &resp, nil
}

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
