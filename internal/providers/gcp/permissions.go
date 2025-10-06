package gcp

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// Get embedded permissions.json can't use third_party
// as it does not include the stage information or
// whether the permission is deprecated etc

//go:embed permissions.json
var gcpPermissions []byte

func GetGcpPermissions() []byte {
	return gcpPermissions
}

/*
"apiDisabled": true,
"description": "Create annotation specs",
"name": "aiplatform.annotationSpecs.create",
"stage": "GA",
"title": "Create annotation specs"
*/
type gcpPermissionMap []struct {
	ApiDisabled           bool   `json:"apiDisabled,omitempty"`
	Description           string `json:"description,omitempty"`
	Name                  string `json:"name,omitempty"`
	Stage                 string `json:"stage,omitempty"`
	Title                 string `json:"title,omitempty"`
	OnlyInPredefinedRoles bool   `json:"onlyInPredefinedRoles,omitempty"`
}

func (p *gcpProvider) LoadPermissions(stage string) error {
	var permissionMap gcpPermissionMap

	// Load GCP Permissions
	if err := json.Unmarshal(GetGcpPermissions(), &permissionMap); err != nil {
		return fmt.Errorf("failed to unmarshal GCP permissions: %w", err)
	}

	var permissions []models.ProviderPermission

	// Create in-memory Bleve index
	mapping := bleve.NewIndexMapping()
	index, err := bleve.NewMemOnly(mapping)
	if err != nil {
		return fmt.Errorf("failed to create search index: %w", err)
	}

	if len(stage) == 0 {
		stage = DefaultStage
	}

	for _, perm := range permissionMap {

		if perm.OnlyInPredefinedRoles {
			continue
		}

		if strings.Compare(perm.Stage, stage) != 0 {
			continue
		}

		permissions = append(permissions, models.ProviderPermission{
			Name:        perm.Name,
			Title:       perm.Title,
			Description: perm.Description,
		})
	}

	p.permissions = permissions
	p.permissionsIndex = index

	logrus.WithFields(logrus.Fields{
		"permissions": len(permissions),
	}).Debug("Loaded GCP permissions")

	return nil
}

func (p *gcpProvider) GetPermission(ctx context.Context, permission string) (*models.ProviderPermission, error) {
	// loop over permissions and match by name
	for _, perm := range p.permissions {
		if strings.Compare(perm.Name, permission) == 0 {
			return &perm, nil
		}
	}
	return nil, fmt.Errorf("permission not found")
}

func (p *gcpProvider) ListPermissions(ctx context.Context, filters ...string) ([]models.ProviderPermission, error) {

	return common.BleveListSearch(ctx, p.permissionsIndex, func(a *search.DocumentMatch, b models.ProviderPermission) bool {
		return strings.Compare(a.ID, b.Name) == 0
	}, p.permissions, filters...)

}
