package aws

import (
	"context"

	"github.com/thand-io/agent/internal/models"
)

type awsProviderMock struct {
	*awsProvider
}

// NewMockAwsProvider creates a new mock AWS provider
func NewMockAwsProvider() *awsProviderMock {

	// Start by getting a copy of the base awsProvider
	return &awsProviderMock{
		awsProvider: &awsProvider{},
	}
}

func (p *awsProviderMock) Initialize(identifier string, provider models.Provider) error {
	// Initialize the embedded awsProvider struct first
	p.awsProvider = &awsProvider{}
	p.awsProvider.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		AwsCapabilities,
	)

	// Load AWS Permissions and Roles from shared singleton
	if err := p.Synchronize(context.Background(), nil, nil); err != nil {
		return err
	}

	return nil
}

func (p *awsProviderMock) Synchronize(
	ctx context.Context,
	temporalService models.TemporalImpl,
	req *models.SynchronizeRequest,
) error {
	return PreSynchronizeActivities(ctx, temporalService, p)
}

// This inherits the awsProvider methods but overrides the synchronization methods to return empty results
func (p *awsProviderMock) SynchronizeGroups(ctx context.Context, req *models.SynchronizeGroupsRequest) (*models.SynchronizeGroupsResponse, error) {
	return &models.SynchronizeGroupsResponse{}, nil
}

func (p *awsProviderMock) SynchronizeUsers(ctx context.Context, req *models.SynchronizeUsersRequest) (*models.SynchronizeUsersResponse, error) {
	return &models.SynchronizeUsersResponse{}, nil
}

func (p *awsProviderMock) SynchronizeIdentities(ctx context.Context, req *models.SynchronizeIdentitiesRequest) (*models.SynchronizeIdentitiesResponse, error) {
	return &models.SynchronizeIdentitiesResponse{}, nil
}

func (p *awsProviderMock) SynchronizeRoles(ctx context.Context, req *models.SynchronizeRolesRequest) (*models.SynchronizeRolesResponse, error) {
	return &models.SynchronizeRolesResponse{}, nil
}

func (p *awsProviderMock) SynchronizePermissions(ctx context.Context, req *models.SynchronizePermissionsRequest) (*models.SynchronizePermissionsResponse, error) {
	return &models.SynchronizePermissionsResponse{}, nil
}

func (p *awsProviderMock) SynchronizeResources(ctx context.Context, req *models.SynchronizeResourcesRequest) (*models.SynchronizeResourcesResponse, error) {
	return &models.SynchronizeResourcesResponse{}, nil
}
