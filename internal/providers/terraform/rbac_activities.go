package terraform

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-tfe"
	"github.com/thand-io/agent/internal/models"
)

// terraformProviderActivities exposes granular Terraform provider operations as
// individual Temporal activities, one per workspace mutation.
type terraformProviderActivities struct {
	provider *terraformProvider
}

// ─────────────────────────────────────────────────────────────────────────────
// Request / response types
// ─────────────────────────────────────────────────────────────────────────────

type AddTeamAccessRequest struct {
	User        *models.User `json:"user"`
	RoleName    string       `json:"role_name"`
	WorkspaceID string       `json:"workspace_id"`
}

type RemoveTeamAccessRequest struct {
	UserID      string `json:"user_id"`
	WorkspaceID string `json:"workspace_id"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Activity implementations
// ─────────────────────────────────────────────────────────────────────────────

// AddTeamAccess grants a team access to a single Terraform workspace.
func (a *terraformProviderActivities) AddTeamAccess(
	ctx context.Context,
	req *AddTeamAccessRequest,
) error {
	teamAccess := tfe.TeamAccessAddOptions{
		Access:    tfe.Access(tfe.AccessType(req.RoleName)),
		Team:      &tfe.Team{ID: req.User.ID},
		Workspace: &tfe.Workspace{ID: req.WorkspaceID},
	}
	_, err := a.provider.client.TeamAccess.Add(ctx, teamAccess)
	if err != nil {
		return fmt.Errorf("failed to add team access for user %s on workspace %s: %w",
			req.User.ID, req.WorkspaceID, err)
	}
	return nil
}

// RemoveTeamAccess revokes a team's access from a single Terraform workspace.
func (a *terraformProviderActivities) RemoveTeamAccess(
	ctx context.Context,
	req *RemoveTeamAccessRequest,
) error {
	listOptions := &tfe.TeamAccessListOptions{
		WorkspaceID: req.WorkspaceID,
	}
	teamAccesses, err := a.provider.client.TeamAccess.List(ctx, listOptions)
	if err != nil {
		return fmt.Errorf("failed to list team accesses for workspace %s: %w", req.WorkspaceID, err)
	}
	for _, ta := range teamAccesses.Items {
		if ta.Team.ID == req.UserID {
			if err := a.provider.client.TeamAccess.Remove(ctx, ta.ID); err != nil {
				return fmt.Errorf("failed to remove team access for user %s on workspace %s: %w",
					req.UserID, req.WorkspaceID, err)
			}
			return nil
		}
	}
	return fmt.Errorf("no team access found for user %s on workspace %s", req.UserID, req.WorkspaceID)
}
