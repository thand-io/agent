package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/thand-io/agent/internal/models"
)

var GitHubPermissions = []models.ProviderPermission{{
	Name:        "read",
	Description: "Read access",
}}

func (p *githubProvider) GetPermission(ctx context.Context, permission string) (*models.ProviderPermission, error) {
	for _, perm := range GitHubPermissions {
		if strings.Compare(perm.Name, permission) == 0 {
			return &perm, nil
		}
	}
	return nil, fmt.Errorf("permission not found: %s", permission)
}

func (p *githubProvider) ListPermissions(ctx context.Context, filters ...string) ([]models.ProviderPermission, error) {
	// TODO: Implement GitHub ListPermissions logic
	return GitHubPermissions, nil
}
