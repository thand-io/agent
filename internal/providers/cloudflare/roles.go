package cloudflare

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

// SynchronizeRoles fetches and caches roles from Cloudflare
func (p *cloudflareProvider) SynchronizeRoles(ctx context.Context, req *models.SynchronizeRolesRequest) (*models.SynchronizeRolesResponse, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Refreshed Cloudflare roles in %s", elapsed)
	}()

	accountRC := cloudflare.AccountIdentifier(p.accountID)

	if req.Pagination == nil {
		req.Pagination = &models.PaginationOptions{
			Page:     1,
			PageSize: 100,
		}
	}

	// List all account roles
	// List account roles with pagination
	roles, err := p.client.ListAccountRoles(ctx, accountRC, cloudflare.ListAccountRolesParams{
		ResultInfo: cloudflare.ResultInfo{
			Page:    req.Pagination.Page,
			PerPage: req.Pagination.PageSize,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list account roles: %w", err)
	}

	var providerRoles []models.ProviderRole

	for _, role := range roles {
		providerRoles = append(providerRoles, models.ProviderRole{
			ID:   role.ID,
			Name: role.Name,
			Role: role,
		})
	}
	logrus.WithFields(logrus.Fields{
		"roles": len(providerRoles),
	}).Debug("Refreshed Cloudflare roles")

	return &models.SynchronizeRolesResponse{
		Pagination: &models.PaginationOptions{
			Page:     req.Pagination.Page + 1,
			PageSize: req.Pagination.PageSize,
		},
		Roles: providerRoles,
	}, nil
}
