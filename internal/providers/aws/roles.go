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

type awsManagedPolicies struct {
	Policies []awsManagedPolicy `json:"policies"`
}

type awsManagedPolicy struct {
	Name string `json:"name"`
}

func (p *awsProvider) LoadRoles() error {
	var docs awsManagedPolicies
	if err := json.Unmarshal(third_party.GetEc2Roles(), &docs); err != nil {
		return fmt.Errorf("failed to unmarshal EC2 roles: %w", err)
	}

	var roles []models.ProviderRole

	// Create in-memory Bleve index for roles
	mapping := bleve.NewIndexMapping()
	rolesIndex, err := bleve.NewMemOnly(mapping)
	if err != nil {
		return fmt.Errorf("failed to create roles search index: %w", err)
	}

	// Index roles
	for _, policy := range docs.Policies {
		role := models.ProviderRole{
			Name: policy.Name,
		}
		roles = append(roles, role)

		// Index the role for full-text search
		if err := rolesIndex.Index(policy.Name, role); err != nil {
			return fmt.Errorf("failed to index role %s: %w", policy.Name, err)
		}
	}

	p.roles = roles
	p.rolesIndex = rolesIndex

	logrus.WithFields(logrus.Fields{
		"roles": len(roles),
	}).Debug("Loaded and indexed EC2 roles")

	return nil
}

func (p *awsProvider) GetRole(ctx context.Context, role string) (*models.ProviderRole, error) {

	// loop over and match role by name
	for _, r := range p.roles {
		if strings.Compare(r.Name, role) == 0 {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("role not found")
}

func (p *awsProvider) ListRoles(ctx context.Context, filters ...string) ([]models.ProviderRole, error) {

	return common.BleveListSearch(ctx, p.rolesIndex, func(a *search.DocumentMatch, b models.ProviderRole) bool {
		return strings.Compare(a.ID, b.Name) == 0
	}, p.roles, filters...)

}
