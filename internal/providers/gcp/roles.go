package gcp

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

func (p *gcpProvider) LoadRoles(stage string) error {

	var predefinedRoles []gcpPredefinedRole
	if err := json.Unmarshal(third_party.GetGcpRoles(), &predefinedRoles); err != nil {
		return fmt.Errorf("failed to unmarshal GCP roles: %w", err)
	}

	var roles []models.ProviderRole

	// Create in-memory Bleve index for roles
	mapping := bleve.NewIndexMapping()
	rolesIndex, err := bleve.NewMemOnly(mapping)
	if err != nil {
		return fmt.Errorf("failed to create roles search index: %w", err)
	}

	if len(stage) == 0 {
		stage = DefaultStage
	}

	for _, gcpRole := range predefinedRoles {

		if strings.Compare(gcpRole.Stage, stage) != 0 {
			continue
		}

		roles = append(roles, models.ProviderRole{
			Name:        gcpRole.Name,
			Title:       gcpRole.Title,
			Description: gcpRole.Description,
		})
	}

	p.roles = roles
	p.rolesIndex = rolesIndex

	logrus.WithFields(logrus.Fields{
		"roles": len(roles),
	}).Debug("Loaded GCP roles")

	return nil
}

func (p *gcpProvider) GetRole(ctx context.Context, role string) (*models.ProviderRole, error) {
	// loop over and match role by name
	for _, r := range p.roles {
		if strings.Compare(r.Name, role) == 0 {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("role not found")
}

func (p *gcpProvider) ListRoles(ctx context.Context, filters ...string) ([]models.ProviderRole, error) {

	return common.BleveListSearch(ctx, p.rolesIndex, func(a *search.DocumentMatch, b models.ProviderRole) bool {
		return strings.Compare(a.ID, b.Name) == 0
	}, p.roles, filters...)

}
