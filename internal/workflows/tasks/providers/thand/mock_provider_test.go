package thand

import (
	"context"
	"fmt"

	"github.com/thand-io/agent/internal/models"
)

type mockProviderImpl struct {
	identities map[string]*models.Identity
}

func (m *mockProviderImpl) Initialize(identifier string, provider models.Provider) error { return nil }
func (m *mockProviderImpl) GetConfig() *models.BasicConfig                               { return nil }
func (m *mockProviderImpl) GetIdentifier() string                                        { return "mock" }
func (m *mockProviderImpl) GetName() string                                              { return "mock" }
func (m *mockProviderImpl) GetDescription() string                                       { return "mock provider" }
func (m *mockProviderImpl) GetProvider() string                                          { return "mock" }
func (m *mockProviderImpl) Synchronize(ctx context.Context, temporalClient models.TemporalImpl, req *models.SynchronizeRequest) error {
	return nil
}
func (m *mockProviderImpl) RegisterWorkflows(temporalClient models.TemporalImpl) error  { return nil }
func (m *mockProviderImpl) RegisterActivities(temporalClient models.TemporalImpl) error { return nil }
func (m *mockProviderImpl) GetCapabilities() []models.ProviderCapability                { return nil }
func (m *mockProviderImpl) HasCapability(capability models.ProviderCapability) bool     { return false }
func (m *mockProviderImpl) HasAnyCapability(capabilities ...models.ProviderCapability) bool {
	return false
}
func (m *mockProviderImpl) CanSynchronizeRoles() bool       { return false }
func (m *mockProviderImpl) CanSynchronizePermissions() bool { return false }
func (m *mockProviderImpl) CanSynchronizeResources() bool   { return false }
func (m *mockProviderImpl) CanSynchronizeIdentities() bool  { return true }
func (m *mockProviderImpl) CanSynchronizeUsers() bool       { return false }
func (m *mockProviderImpl) CanSynchronizeGroups() bool      { return false }

// ProviderIdentities
func (m *mockProviderImpl) GetIdentity(ctx context.Context, identity string) (*models.Identity, error) {
	if val, ok := m.identities[identity]; ok {
		return val, nil
	}
	return nil, fmt.Errorf("identity not found")
}
func (m *mockProviderImpl) ListIdentities(ctx context.Context, searchRequest *models.SearchRequest) ([]models.SearchResult[models.Identity], error) {
	return nil, nil
}
func (m *mockProviderImpl) SetIdentities(identities []models.Identity) {}
func (m *mockProviderImpl) AddIdentities(identities ...models.Identity) {}
func (m *mockProviderImpl) SynchronizeIdentities(ctx context.Context, req *models.SynchronizeIdentitiesRequest) (*models.SynchronizeIdentitiesResponse, error) {
	return nil, nil
}
func (m *mockProviderImpl) SynchronizeUsers(ctx context.Context, req *models.SynchronizeUsersRequest) (*models.SynchronizeUsersResponse, error) {
	return nil, nil
}
func (m *mockProviderImpl) SynchronizeGroups(ctx context.Context, req *models.SynchronizeGroupsRequest) (*models.SynchronizeGroupsResponse, error) {
	return nil, nil
}

// ProviderNotifier
func (m *mockProviderImpl) SendNotification(ctx context.Context, notification models.NotificationRequest) error {
	return nil
}

// ProviderAuthorizor
func (m *mockProviderImpl) AuthorizeSession(ctx context.Context, auth *models.AuthorizeUser) (*models.AuthorizeSessionResponse, error) {
	return nil, nil
}
func (m *mockProviderImpl) CreateSession(ctx context.Context, auth *models.AuthorizeUser) (*models.Session, error) {
	return nil, nil
}
func (m *mockProviderImpl) ValidateSession(ctx context.Context, session *models.Session) error {
	return nil
}
func (m *mockProviderImpl) RenewSession(ctx context.Context, session *models.Session) (*models.Session, error) {
	return nil, nil
}

// ProviderRoleBasedAccessControl
func (m *mockProviderImpl) SynchronizeRoles(ctx context.Context, req *models.SynchronizeRolesRequest) (*models.SynchronizeRolesResponse, error) {
	return nil, nil
}
func (m *mockProviderImpl) SynchronizePermissions(ctx context.Context, req *models.SynchronizePermissionsRequest) (*models.SynchronizePermissionsResponse, error) {
	return nil, nil
}
func (m *mockProviderImpl) SynchronizeResources(ctx context.Context, req *models.SynchronizeResourcesRequest) (*models.SynchronizeResourcesResponse, error) {
	return nil, nil
}
func (m *mockProviderImpl) SetRoles(roles []models.ProviderRole)             {}
func (m *mockProviderImpl) AddRoles(role ...models.ProviderRole)             {}
func (m *mockProviderImpl) SetPermissions(permissions []models.ProviderPermission) {}
func (m *mockProviderImpl) AddPermissions(permission ...models.ProviderPermission) {}
func (m *mockProviderImpl) SetResources(resources []models.ProviderResource)       {}
func (m *mockProviderImpl) AddResources(resource ...models.ProviderResource)       {}
func (m *mockProviderImpl) GetPermission(ctx context.Context, permission string) (*models.ProviderPermission, error) {
	return nil, nil
}
func (m *mockProviderImpl) ListPermissions(ctx context.Context, searchRequest *models.SearchRequest) ([]models.SearchResult[models.ProviderPermission], error) {
	return nil, nil
}
func (m *mockProviderImpl) GetResource(ctx context.Context, resource string) (*models.ProviderResource, error) {
	return nil, nil
}
func (m *mockProviderImpl) ListResources(ctx context.Context, searchRequest *models.SearchRequest) ([]models.SearchResult[models.ProviderResource], error) {
	return nil, nil
}
func (m *mockProviderImpl) GetRole(ctx context.Context, role string) (*models.ProviderRole, error) {
	return nil, nil
}
func (m *mockProviderImpl) ListRoles(ctx context.Context, searchRequest *models.SearchRequest) ([]models.SearchResult[models.ProviderRole], error) {
	return nil, nil
}
func (m *mockProviderImpl) ValidateRole(ctx context.Context, identity *models.Identity, role *models.Role) (map[string]any, error) {
	return nil, nil
}
func (m *mockProviderImpl) AuthorizeRole(ctx context.Context, req *models.AuthorizeRoleRequest) (*models.AuthorizeRoleResponse, error) {
	return nil, nil
}
func (m *mockProviderImpl) RevokeRole(ctx context.Context, req *models.RevokeRoleRequest) (*models.RevokeRoleResponse, error) {
	return nil, nil
}
func (m *mockProviderImpl) GetAuthorizedAccessUrl(ctx context.Context, req *models.AuthorizeRoleRequest, resp *models.AuthorizeRoleResponse) string {
	return ""
}
