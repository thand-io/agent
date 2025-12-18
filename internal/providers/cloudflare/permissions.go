package cloudflare

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

// SynchronizePermissions fetches and caches permissions from Cloudflare
func (p *cloudflareProvider) SynchronizePermissions(ctx context.Context, req *models.SynchronizePermissionsRequest) (*models.SynchronizePermissionsResponse, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Refreshed Cloudflare permissions in %s", elapsed)
	}()

	accountRC := cloudflare.AccountIdentifier(p.accountID)

	if req.Pagination == nil {
		req.Pagination = &models.PaginationOptions{
			Page:     1,
			PageSize: 100,
		}
	}

	// List account roles with pagination
	roles, err := p.client.ListAccountRoles(ctx, accountRC, cloudflare.ListAccountRolesParams{
		ResultInfo: cloudflare.ResultInfo{
			Page:    req.Pagination.Page,
			PerPage: req.Pagination.PageSize,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list account permissions: %w", err)
	}

	var providerPermissions []models.ProviderPermission

	for _, role := range roles {
		providerPermissions = append(providerPermissions, models.ProviderPermission{
			ID:          role.ID,
			Name:        role.Name,
			Description: role.Description,
			Permission:  role,
		})
	}

	logrus.WithFields(logrus.Fields{
		"permissions": len(providerPermissions),
	}).Debug("Refreshed Cloudflare permissions")

	return &models.SynchronizePermissionsResponse{
		Pagination: &models.PaginationOptions{
			Page:     req.Pagination.Page + 1,
			PageSize: req.Pagination.PageSize,
		},
		Permissions: providerPermissions,
	}, nil
}
