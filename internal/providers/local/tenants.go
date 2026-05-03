package local

import (
	"context"

	"github.com/thand-io/agent/internal/models"
)

// The goal of this request is to query temporal to get a list
// of thand agent workflows and their attached identities.
// Tenants for local are going to be the local devices we want
// to elevate or make changes too
func (p *localProvider) SynchronizeTenants(
	ctx context.Context,
	req *models.SynchronizeTenantsRequest,
) (*models.SynchronizeTenantsResponse, error) {

	return &models.SynchronizeTenantsResponse{
		Pagination: nil,
		Tenants: []models.ProviderTenant{{
			ID:   "laptop",
			Type: "device",
			Name: "macbook (hugh@thand.io)",
		}},
	}, nil
}
