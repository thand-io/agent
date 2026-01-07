package gcp

import (
	"context"

	"github.com/thand-io/agent/internal/models"
)

// gcpProviderMock is a mock implementation of gcpProvider for testing
type gcpProviderMock struct {
	*gcpProvider
}

// NewMockGcpProvider creates a new mock GCP provider for testing
func NewMockGcpProvider() *gcpProviderMock {
	return &gcpProviderMock{
		gcpProvider: &gcpProvider{},
	}
}

// Initialize loads permissions and roles without connecting to GCP
func (p *gcpProviderMock) Initialize(identifier string, provider models.ProviderConfig) error {
	// Initialize the embedded gcpProvider struct
	p.gcpProvider = &gcpProvider{}

	// Set the provider to the base provider
	p.gcpProvider.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		GcpCapabilities,
	)

	// Load GCP Permissions and Roles from shared singleton
	if err := p.Synchronize(context.Background(), nil, nil); err != nil {
		return err
	}

	return nil
}

func (p *gcpProviderMock) Synchronize(
	ctx context.Context,
	temporalService models.TemporalImpl,
	req *models.SynchronizeRequest,
) error {
	return PreSynchronizeActivities(ctx, temporalService, p, req)
}

func (p *gcpProviderMock) SynchronizeIdentities(ctx context.Context, req *models.SynchronizeIdentitiesRequest) (*models.SynchronizeIdentitiesResponse, error) {
	return &models.SynchronizeIdentitiesResponse{}, nil
}
