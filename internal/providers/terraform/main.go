package terraform

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
)

// terraformProvider implements the ProviderImpl interface for Terraform
type terraformProvider struct {
	*models.BaseProvider
	client      *tfe.Client
	permissions []models.ProviderPermission
}

func (p *terraformProvider) Initialize(provider models.Provider) error {
	p.BaseProvider = models.NewBaseProvider(
		provider,
		models.ProviderCapabilityAuthorizor,
		models.ProviderCapabilityRBAC,
	)

	terraformConfig := p.GetConfig()

	terraformToken, foundToken := terraformConfig.GetString("token")

	if !foundToken {
		return fmt.Errorf("missing required Terraform configuration: token is required")
	}

	// Initialize Terraform Cloud client
	config := &tfe.Config{
		Token: terraformToken,
	}

	client, err := tfe.NewClient(config)
	if err != nil {
		return fmt.Errorf("failed to create Terraform client: %w", err)
	}

	p.client = client

	p.permissions = []models.ProviderPermission{{
		Name:        string(tfe.AccessAdmin),
		Description: "Admin access",
	}, {
		Name:        string(tfe.AccessRead),
		Description: "Read access",
	}, {
		Name:        string(tfe.AccessWrite),
		Description: "Write access",
	}, {
		Name:        string(tfe.AccessPlan),
		Description: "Plan access",
	}, {
		Name:        string(tfe.AccessCustom),
		Description: "Custom access",
	}}

	return nil
}

// Authorize grants access for a user to a role
func (p *terraformProvider) AuthorizeRole(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
) (map[string]any, error) {

	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to authorize terraform role")
	}

	user := req.GetUser()
	role := req.GetRole()

	// Loop over all resources in role.Resources.Allow as workspace IDs
	if len(role.Resources.Allow) == 0 {
		return nil, fmt.Errorf("no workspace IDs found in role.Resources.Allow")
	}

	// Authorize user for each workspace
	for _, workspaceID := range role.Resources.Allow {
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
	ctx context.Context,
	user *models.User,
	role *models.Role,
	metadata map[string]any,
) (map[string]any, error) {
	// Loop over all resources in role.Resources.Allow as workspace IDs
	if len(role.Resources.Allow) == 0 {
		return nil, fmt.Errorf("no workspace IDs found in role.Resources.Allow")
	}

	// Revoke user access for each workspace
	for _, workspaceID := range role.Resources.Allow {
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

func (p *terraformProvider) GetPermission(ctx context.Context, permission string) (*models.ProviderPermission, error) {
	for _, perm := range p.permissions {
		if strings.Compare(perm.Name, permission) == 0 {
			return &perm, nil
		}
	}
	return nil, fmt.Errorf("permission %s not found", permission)
}

func (p *terraformProvider) ListPermissions(ctx context.Context, filters ...string) ([]models.ProviderPermission, error) {
	return p.permissions, nil
}

func (p *terraformProvider) GetRole(ctx context.Context, role string) (*models.ProviderRole, error) {
	// TODO: Implement Terraform GetRole logic
	return nil, fmt.Errorf("Terraform has no concept of roles")
}

func (p *terraformProvider) ListRoles(ctx context.Context, filters ...string) ([]models.ProviderRole, error) {
	// TODO: Implement Terraform ListRoles logic
	return nil, fmt.Errorf("Terraform has no concept of roles")
}

func init() {
	providers.Register("terraform", &terraformProvider{})
}
