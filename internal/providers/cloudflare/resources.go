package cloudflare

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

const resourceTypeZone = "zone"
const resourceTypeAccount = "account"

// SynchronizeResources loads Cloudflare resources (zones, accounts) from the API
func (p *cloudflareProvider) SynchronizeResources(ctx context.Context, req *models.SynchronizeResourcesRequest) (*models.SynchronizeResourcesResponse, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Loaded Cloudflare resources in %s", elapsed)
	}()

	var resourcesData []models.ProviderResource

	// Load zones
	zoneResources, err := p.loadZoneResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load zone resources: %w", err)
	}
	resourcesData = append(resourcesData, zoneResources...)

	// Load accounts
	accountResources, err := p.loadAccountResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load account resources: %w", err)
	}
	resourcesData = append(resourcesData, accountResources...)

	logrus.WithFields(logrus.Fields{
		"resources": len(resourcesData),
		"zones":     len(zoneResources),
		"accounts":  len(accountResources),
	}).Debug("Loaded Cloudflare resources")

	return &models.SynchronizeResourcesResponse{
		Resources: resourcesData,
	}, nil
}

// loadZoneResources loads zone resources from the Cloudflare API
// and stores full zone details in the Resource field for later use
func (p *cloudflareProvider) loadZoneResources(ctx context.Context) ([]models.ProviderResource, error) {
	zones, err := p.client.ListZones(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list zones: %w", err)
	}

	var resources []models.ProviderResource
	for _, zone := range zones {
		resource := models.ProviderResource{
			ID:          zone.ID,
			Type:        resourceTypeZone,
			Name:        zone.Name,
			Description: fmt.Sprintf("Zone: %s (Status: %s)", zone.Name, zone.Status),
			Resource:    zone, // In-memory only: stores the full zone object to avoid ZoneDetails API calls later; not persisted due to json:"-" tag
		}
		resources = append(resources, resource)
	}

	return resources, nil
}

// loadAccountResources loads account resources from the Cloudflare API
// and stores full account details in the Resource field for later use
func (p *cloudflareProvider) loadAccountResources(ctx context.Context) ([]models.ProviderResource, error) {
	accounts, _, err := p.client.Accounts(ctx, cloudflare.AccountsListParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}

	var resources []models.ProviderResource
	for _, account := range accounts {
		resource := models.ProviderResource{
			ID:          account.ID,
			Type:        resourceTypeAccount,
			Name:        account.Name,
			Description: fmt.Sprintf("Account: %s (Type: %s)", account.Name, account.Type),
			Resource:    account, // Store the full account object to avoid Account API calls later
		}
		resources = append(resources, resource)
	}

	return resources, nil
}
