package cloudflare

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

const resourceTypeZone = "zone"

// SynchronizeResources loads Cloudflare resources (zones) from the API
// Note: Cloudflare accounts are now managed as tenants, not resources
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

	logrus.WithFields(logrus.Fields{
		"resources": len(resourcesData),
		"zones":     len(zoneResources),
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
