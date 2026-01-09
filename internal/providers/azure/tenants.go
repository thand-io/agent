package azure

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

// SynchronizeTenants fetches subscriptions from Azure as tenants
func (p *azureProvider) SynchronizeTenants(ctx context.Context, req *models.SynchronizeTenantsRequest) (*models.SynchronizeTenantsResponse, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Refreshed Azure tenants (subscriptions) in %s", elapsed)
	}()

	// Check if subscriptions client is available
	if p.subscriptionsClient == nil {
		logrus.Warn("Subscriptions client not initialized, skipping tenant synchronization")
		return &models.SynchronizeTenantsResponse{}, nil
	}

	if req.Pagination == nil {
		req.Pagination = &models.PaginationOptions{
			PageSize: 50,
		}
	}

	var tenants []models.ProviderTenant

	// List all subscriptions the service principal has access to
	pager := p.subscriptionsClient.NewListPager(nil)

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			// If this is the initial request and we get an error, it's likely a permission issue
			if req.Pagination == nil || len(req.Pagination.Token) == 0 {
				logrus.WithError(err).Warn("Unable to list Azure subscriptions - provider may not have necessary permissions")
				// Return empty response rather than error to allow provider to initialize
				return &models.SynchronizeTenantsResponse{}, nil
			}
			return nil, fmt.Errorf("failed to list subscriptions: %w", err)
		}

		for _, subscription := range page.Value {
			if subscription == nil {
				continue
			}

			// Skip subscriptions that are not in an active state
			if subscription.State != nil {
				state := string(*subscription.State)
				if state != "Enabled" {
					continue
				}
			}

			var subscriptionId string
			var subscriptionName string
			var tenantId string

			if subscription.SubscriptionID != nil && len(*subscription.SubscriptionID) > 0 {
				subscriptionId = *subscription.SubscriptionID
			} else {
				continue
			}

			if subscription.DisplayName != nil && len(*subscription.DisplayName) > 0 {
				subscriptionName = *subscription.DisplayName
			} else {
				// Use subscription ID as name if display name is not available
				subscriptionName = subscriptionId
			}

			if subscription.TenantID != nil && len(*subscription.TenantID) > 0 {
				tenantId = *subscription.TenantID
			}

			tenant := models.ProviderTenant{
				ID:     subscriptionId,
				Parent: tenantId,
				Type:   "subscription",
				Name:   subscriptionName,
				Tenant: subscription,
			}
			tenants = append(tenants, tenant)
		}
	}

	response := &models.SynchronizeTenantsResponse{
		Tenants: tenants,
	}

	logrus.WithFields(logrus.Fields{
		"count": len(tenants),
	}).Debug("Refreshed Azure tenants (subscriptions)")

	return response, nil
}
