package terraform

import (
	"fmt"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/thand-io/agent/internal/models"
	sdkWorkflowsRunner "github.com/thand-io/agent/sdk/workflows/runner"
	"go.temporal.io/sdk/workflow"
)

// Authorize grants access for a user to a role
func (p *terraformProvider) AuthorizeRole(
	task models.ProviderContext,
	req *models.AuthorizeRoleRequest,
) (*models.AuthorizeRoleResponse, error) {
	if task.HasTemporalContext() {
		return p.authorizeRoleTemporal(task.GetTemporalContext(), task.GetTaskQueue(), req)
	}
	ctx := task.GetContext()

	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to authorize terraform role")
	}

	user := req.GetUser()
	role := req.GetRole()

	// Collect all targets from permission statements as workspace IDs
	var workspaceIDs []string
	for _, stmt := range role.Permissions.Allow {
		workspaceIDs = append(workspaceIDs, stmt.Targets...)
	}

	if len(workspaceIDs) == 0 {
		return nil, fmt.Errorf("no workspace IDs found in role.Permissions.Allow[].Targets")
	}

	// Authorize user for each workspace
	for _, workspaceID := range workspaceIDs {
		// Create team access for the user on the specified workspace
		teamAccess := &tfe.TeamAccessAddOptions{
			Access:    tfe.Access(tfe.AccessType(role.Name)), // Use role name as access level
			Team:      &tfe.Team{ID: user.ID},                // Assuming user ID maps to team ID
			Workspace: &tfe.Workspace{ID: workspaceID},
		}

		_, err := p.client.TeamAccess.Add(ctx, *teamAccess)
		if err != nil {
			return nil, fmt.Errorf("failed to authorize user %s for role %s on workspace %s: %w",
				user.ID, role.Name, workspaceID, err)
		}
	}

	return nil, nil
}

// Revoke removes access for a user from a role
func (p *terraformProvider) RevokeRole(
	task models.ProviderContext,
	req *models.RevokeRoleRequest,
) (*models.RevokeRoleResponse, error) {
	if task.HasTemporalContext() {
		return p.revokeRoleTemporal(task.GetTemporalContext(), task.GetTaskQueue(), req)
	}
	ctx := task.GetContext()

	user := req.GetUser()
	role := req.GetRole()

	// Collect all targets from permission statements as workspace IDs
	var workspaceIDs []string
	for _, stmt := range role.Permissions.Allow {
		workspaceIDs = append(workspaceIDs, stmt.Targets...)
	}

	if len(workspaceIDs) == 0 {
		return nil, fmt.Errorf("no workspace IDs found in role.Permissions.Allow[].Targets")
	}

	// Revoke user access for each workspace
	for _, workspaceID := range workspaceIDs {
		// List team accesses for the workspace to find the one to remove
		listOptions := &tfe.TeamAccessListOptions{
			WorkspaceID: workspaceID,
		}

		teamAccesses, err := p.client.TeamAccess.List(ctx, listOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to list team accesses for workspace %s: %w", workspaceID, err)
		}

		// Find and remove the team access for this user/team
		found := false
		for _, ta := range teamAccesses.Items {
			if ta.Team.ID == user.ID { // Assuming user ID maps to team ID
				err := p.client.TeamAccess.Remove(ctx, ta.ID)
				if err != nil {
					return nil, fmt.Errorf("failed to revoke access for user %s on workspace %s: %w",
						user.ID, workspaceID, err)
				}
				found = true
				break
			}
		}

		if !found {
			return nil, fmt.Errorf("no team access found for user %s on workspace %s", user.ID, workspaceID)
		}
	}

	return nil, nil
}

// authorizeRoleTemporal dispatches an AddTeamAccess activity per workspace in parallel.
func (p *terraformProvider) authorizeRoleTemporal(
	wfCtx workflow.Context,
	taskQueue string,
	req *models.AuthorizeRoleRequest,
) (*models.AuthorizeRoleResponse, error) {
	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to authorize terraform role")
	}

	identifier := p.GetIdentifier()
	ao := workflow.ActivityOptions{
		TaskQueue:           taskQueue,
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
	}
	wfCtx = workflow.WithActivityOptions(wfCtx, ao)

	user := req.GetUser()
	role := req.GetRole()

	var workspaceIDs []string
	for _, stmt := range role.Permissions.Allow {
		workspaceIDs = append(workspaceIDs, stmt.Targets...)
	}
	if len(workspaceIDs) == 0 {
		return nil, fmt.Errorf("no workspace IDs found in role.Permissions.Allow[].Targets")
	}

	for _, wsID := range workspaceIDs {
		if err := workflow.ExecuteActivity(
			wfCtx,
			models.CreateTemporalProviderWorkflowName(identifier, "AddTeamAccess"),
			&AddTeamAccessRequest{
				User:        user,
				RoleName:    role.Name,
				WorkspaceID: wsID,
			},
		).Get(wfCtx, nil); err != nil {
			return nil, fmt.Errorf("AddTeamAccess activity failed for workspace %s: %w", wsID, err)
		}
	}

	return nil, nil
}

// revokeRoleTemporal dispatches a RemoveTeamAccess activity per workspace in parallel.
func (p *terraformProvider) revokeRoleTemporal(
	wfCtx workflow.Context,
	taskQueue string,
	req *models.RevokeRoleRequest,
) (*models.RevokeRoleResponse, error) {
	identifier := p.GetIdentifier()
	ao := workflow.ActivityOptions{
		TaskQueue:           taskQueue,
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
	}
	wfCtx = workflow.WithActivityOptions(wfCtx, ao)

	user := req.GetUser()
	role := req.GetRole()

	var workspaceIDs []string
	for _, stmt := range role.Permissions.Allow {
		workspaceIDs = append(workspaceIDs, stmt.Targets...)
	}
	if len(workspaceIDs) == 0 {
		return nil, fmt.Errorf("no workspace IDs found in role.Permissions.Allow[].Targets")
	}

	for _, wsID := range workspaceIDs {
		if err := workflow.ExecuteActivity(
			wfCtx,
			models.CreateTemporalProviderWorkflowName(identifier, "RemoveTeamAccess"),
			&RemoveTeamAccessRequest{
				UserID:      user.ID,
				WorkspaceID: wsID,
			},
		).Get(wfCtx, nil); err != nil {
			return nil, fmt.Errorf("RemoveTeamAccess activity failed for workspace %s: %w", wsID, err)
		}
	}

	return nil, nil
}
