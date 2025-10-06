package salesforce

import (
	"context"
	"fmt"

	"github.com/thand-io/agent/internal/models"
)

func (p *salesForceProvider) ListPermissions(ctx context.Context, filters ...string) ([]models.ProviderPermission, error) {
	return []models.ProviderPermission{}, nil
}

func (p *salesForceProvider) GetPermission(ctx context.Context, permission string) (*models.ProviderPermission, error) {
	return nil, fmt.Errorf("permission not found")
}
