package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/third_party"
)

func (p *awsProvider) LoadPermissions() error {
	var docs map[string]string

	// Load EC2 Permissions
	if err := json.Unmarshal(third_party.GetEc2Docs(), &docs); err != nil {
		return fmt.Errorf("failed to unmarshal EC2 permissions: %w", err)
	}

	var permissions []models.ProviderPermission

	// Create in-memory Bleve index
	mapping := bleve.NewIndexMapping()
	index, err := bleve.NewMemOnly(mapping)
	if err != nil {
		return fmt.Errorf("failed to create search index: %w", err)
	}

	// Index permissions
	for name, description := range docs {
		perm := models.ProviderPermission{
			Name:        name,
			Description: description,
		}
		permissions = append(permissions, perm)

		// Index the permission for full-text search
		if err := index.Index(name, perm); err != nil {
			return fmt.Errorf("failed to index permission %s: %w", name, err)
		}
	}

	p.permissions = permissions
	p.permissionsIndex = index

	logrus.WithFields(logrus.Fields{
		"permissions": len(permissions),
	}).Debug("Loaded and indexed EC2 permissions")

	return nil
}

func (p *awsProvider) GetPermission(ctx context.Context, permission string) (*models.ProviderPermission, error) {
	// loop over permissions and match by name
	for _, p := range p.permissions {
		if strings.Compare(p.Name, permission) == 0 {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("permission not found")
}

func (p *awsProvider) ListPermissions(ctx context.Context, filters ...string) ([]models.ProviderPermission, error) {

	return common.BleveListSearch(ctx, p.permissionsIndex, func(a *search.DocumentMatch, b models.ProviderPermission) bool {
		return strings.Compare(a.ID, b.Name) == 0
	}, p.permissions, filters...)

}
