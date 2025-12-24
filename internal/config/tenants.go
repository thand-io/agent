package config

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

// GetTenant looks up a tenant by its identifier.
// The tenant string can optionally include a provider prefix (e.g., "aws-prod:tenant-id").
// If a prefix is provided, it queries only that specific provider.
// Otherwise, it queries all tenant providers and returns the first match.
func (c *Config) GetTenant(tenant string) (*models.ProviderTenant, error) {
	ctx := context.Background()

	// Check if the tenant has a provider prefix (e.g., "aws-prod:tenant-id")
	var providerID string
	var tenantKey string

	if colonIdx := strings.Index(tenant, ":"); colonIdx != -1 {
		// Has provider prefix
		providerID = tenant[:colonIdx]
		tenantKey = tenant[colonIdx+1:]
	} else {
		// No prefix, use the full tenant
		tenantKey = tenant
	}

	// If we have a specific provider, query only that one
	if len(providerID) != 0 {
		provider, err := c.GetProviderByName(providerID)
		if err != nil {
			return nil, fmt.Errorf("provider '%s' not found: %w", providerID, err)
		}

		result, err := provider.GetClient().GetTenant(ctx, tenantKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get tenant '%s' from provider '%s': %w", tenantKey, providerID, err)
		}

		return result, nil
	}

	// No provider prefix - query all tenant providers
	providerMap := c.GetProvidersByCapability(models.ProviderCapabilityTenants)

	if len(providerMap) == 0 {
		return nil, fmt.Errorf("tenant not found: %s (no tenant providers configured)", tenant)
	}

	// Query all providers in parallel and return the first match
	var wg sync.WaitGroup
	resultChan := make(chan *models.ProviderTenant, len(providerMap))
	doneChan := make(chan struct{})

	for _, provider := range providerMap {
		wg.Add(1)
		go func(p models.Provider) {
			defer wg.Done()

			result, err := p.GetClient().GetTenant(ctx, tenantKey)
			if err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"provider": p.Name,
					"tenant":   tenantKey,
				}).Debug("Failed to get tenant from provider")
				return
			}

			if result != nil {
				select {
				case resultChan <- result:
				case <-doneChan:
					// Another goroutine already found a result
				}
			}
		}(provider)
	}

	// Wait for all goroutines to complete and then close the result channel
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Try to get a result from the channel
	// If channel is closed and empty, result will be nil and ok will be false
	for result := range resultChan {
		if result != nil {
			close(doneChan)
			return result, nil
		}
	}

	// All goroutines finished without finding a result
	return nil, fmt.Errorf("tenant not found: %s", tenantKey)
}
