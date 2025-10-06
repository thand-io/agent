package salesforce

import (
	"context"
	"fmt"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

// Salesforce roles are actually "profiles" in Salesforce terminology.
// However, for the sake of this implementation, we will refer to them as roles.
// https://developer.salesforce.com/docs/atlas.en-us.object_reference.meta/object_reference/sforce_api_objects_profile.htm
func (p *salesForceProvider) LoadRoles() ([]models.ProviderRole, error) {

	// Query Salesforce for available roles (profiles)
	query := "SELECT Id, Name, Description FROM Profile"
	result, err := p.queryWithParams(query)

	if err != nil {
		return nil, err
	}

	// Create in-memory Bleve index for roles
	mapping := bleve.NewIndexMapping()
	rolesIndex, err := bleve.NewMemOnly(mapping)
	if err != nil {
		return nil, fmt.Errorf("failed to create roles search index: %w", err)
	}

	var roles []models.ProviderRole

	// Process query results
	for _, record := range result.Records {
		role := models.ProviderRole{
			Id:          record.StringField("Id"),
			Name:        record.StringField("Name"),
			Description: record.StringField("Description"),
		}
		roles = append(roles, role)
	}

	p.roles = roles
	p.rolesIndex = rolesIndex

	logrus.WithFields(logrus.Fields{
		"roles": len(roles),
	}).Debug("Loaded and indexed Salesforce roles")

	return roles, nil

}

func (p *salesForceProvider) GetRole(ctx context.Context, role string) (*models.ProviderRole, error) {

	// loop over and match role by name
	for _, r := range p.roles {
		if strings.Compare(r.Name, role) == 0 {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("role not found")
}

func (p *salesForceProvider) ListRoles(ctx context.Context, filters ...string) ([]models.ProviderRole, error) {
	if len(filters) == 0 {
		// return all roles
		return p.roles, nil
	}

	// Build search query from filters
	queryString := strings.TrimSpace(strings.Join(filters, " "))

	if len(queryString) == 0 {
		// return all roles if query is empty after trimming
		return p.roles, nil
	}

	query := bleve.NewQueryStringQuery(queryString)
	searchRequest := bleve.NewSearchRequest(query)
	searchRequest.Size = len(p.roles) // Return all matches

	searchResults, err := p.rolesIndex.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("roles search failed: %w", err)
	}

	// Convert search results back to roles
	var matched []models.ProviderRole
	for _, hit := range searchResults.Hits {
		for _, role := range p.roles {
			if role.Name == hit.ID {
				matched = append(matched, role)
				break
			}
		}
	}

	return matched, nil
}
