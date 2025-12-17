package azure

import (
	"context"

	"github.com/thand-io/agent/internal/models"
)

// azureProviderMock is a mock implementation of azureProvider for testing
type azureProviderMock struct {
	*azureProvider
}

// NewMockAzureProvider creates a new mock Azure provider for testing
func NewMockAzureProvider() *azureProviderMock {
	return &azureProviderMock{
		azureProvider: &azureProvider{},
	}
}

// Initialize loads permissions and roles without connecting to Azure
func (p *azureProviderMock) Initialize(identifier string, provider models.Provider) error {
	// Initialize the embedded azureProvider struct
	p.azureProvider = &azureProvider{}

	// Set the provider to the base provider
	p.azureProvider.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		AzureCapabilities...,
	)

	// Load Azure Permissions and Roles from shared singleton
	if err := p.Synchronize(context.Background(), nil, nil); err != nil {
		return err
	}

	return nil
}

func (p *azureProviderMock) Synchronize(
	ctx context.Context,
	temporalService models.TemporalImpl,
	req *models.SynchronizeRequest,
) error {
	return PreSynchronizeActivities(ctx, temporalService, p, req)
}
