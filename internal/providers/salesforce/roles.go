package salesforce

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

// SynchronizeRoles fetches and caches roles from Salesforce
func (p *salesForceProvider) SynchronizeRoles(ctx context.Context, req *models.SynchronizeRolesRequest) (*models.SynchronizeRolesResponse, error) {
	if req.Pagination == nil {
		req.Pagination = &models.PaginationOptions{
			Page:     1,
			PageSize: 100,
		}
	}

	limit := req.Pagination.PageSize
	offset := (req.Pagination.Page - 1) * req.Pagination.PageSize

	// Query Salesforce for available roles (profiles)
	query := fmt.Sprintf("SELECT Id, Name, Description FROM Profile LIMIT %d OFFSET %d", limit, offset)
	result, err := p.queryWithParams(query)

	if err != nil {
		return nil, err
	}

	var roles []models.ProviderRole

	// Process query results
	for _, record := range result.Records {
		role := models.ProviderRole{
			ID:          record.StringField("Id"),
			Name:        record.StringField("Name"),
			Description: record.StringField("Description"),
			Role:        record,
		}
		roles = append(roles, role)
	}

	logrus.WithFields(logrus.Fields{
		"roles": len(roles),
	}).Debug("Refreshed Salesforce roles")

	return &models.SynchronizeRolesResponse{
		Pagination: &models.PaginationOptions{
			Page:     req.Pagination.Page + 1,
			PageSize: req.Pagination.PageSize,
		},
		Roles: roles,
	}, nil
}
