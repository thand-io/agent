package cloudflare

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

const tenantTypeAccount = "account"

func (p *cloudflareProvider) SynchronizeTenants(ctx context.Context, req *models.SynchronizeTenantsRequest) (*models.SynchronizeTenantsResponse, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Refreshed Cloudflare tenants (accounts) in %s", elapsed)
	}()

	// Check if client is available
	if p.client == nil {
		logrus.Warn("Cloudflare client not initialized, skipping tenant synchronization")
		return &models.SynchronizeTenantsResponse{}, nil
	}

	if req.Pagination == nil {
		req.Pagination = &models.PaginationOptions{
			PageSize: 50,
		}
	}

	// Cloudflare SDK uses pagination parameters
	params := cloudflare.AccountsListParams{
		PaginationOptions: cloudflare.PaginationOptions{
			PerPage: req.Pagination.PageSize,
			Page:    1,
		},
	}

	// Parse page number from token if provided
	if len(req.Pagination.Token) != 0 {
		var page int
		if _, err := fmt.Sscanf(req.Pagination.Token, "%d", &page); err == nil {
			params.PaginationOptions.Page = page
		}
	}

	accounts, resultInfo, err := p.client.Accounts(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}

	var tenants []models.ProviderTenant
	for _, account := range accounts {
		tenant := models.ProviderTenant{
			ID:     account.ID,
			Type:   tenantTypeAccount,
			Name:   account.Name,
			Tenant: account, // Store the full account object for later use
		}
		tenants = append(tenants, tenant)
	}

	response := &models.SynchronizeTenantsResponse{
		Tenants: tenants,
	}

	// Check if there are more pages
	if resultInfo.Page < resultInfo.TotalPages {
		response.Pagination = &models.PaginationOptions{
			Token:    fmt.Sprintf("%d", resultInfo.Page+1),
			PageSize: req.Pagination.PageSize,
		}
	}

	logrus.WithFields(logrus.Fields{
		"count": len(tenants),
	}).Debug("Refreshed Cloudflare tenants (accounts)")

	return response, nil
}
