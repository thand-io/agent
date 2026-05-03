package local

import (
	"fmt"
	"time"

	"github.com/thand-io/agent/internal/models"
	sdkWorkflowsRunner "github.com/thand-io/agent/sdk/workflows/runner"
	"go.temporal.io/sdk/workflow"
)

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
