package models

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProviderForSync wraps a BaseProvider with custom sync methods for testing
type mockProviderForSync struct {
	*BaseProvider
	usersCalled         bool
	groupsCalled        bool
	identitiesCalled    bool
	resourcesCalled     bool
	rolesCalled         bool
	permissionsCalled   bool
	tenantsCalled       bool
	usersResponse       *SynchronizeUsersResponse
	groupsResponse      *SynchronizeGroupsResponse
	identitiesResp      *SynchronizeIdentitiesResponse
	resourcesResponse   *SynchronizeResourcesResponse
	rolesResponse       *SynchronizeRolesResponse
	permissionsResponse *SynchronizePermissionsResponse
	tenantsResponse     *SynchronizeTenantsResponse
	usersError          error
	groupsError         error
	identitiesError     error
	resourcesError      error
	rolesError          error
	permissionsError    error
	tenantsError        error
	usersCallCount      int
	usersFunc           func(ctx context.Context, req *SynchronizeUsersRequest) (*SynchronizeUsersResponse, error)
}

func newMockProviderForSync(name string) *mockProviderForSync {
	provider := ProviderConfig{
		Name:        name,
		Description: "Mock Provider for Sync Testing",
		Provider:    "mock",
		Enabled:     true,
	}

	capabilities := NewProviderCapabilities().
		WithDefaultUsersConfiguration().
		WithDefaultGroupsConfiguration()
		// Note: WithDefaultIdentitiesConfiguration() is not included by default
		// Add it explicitly in tests that need to test identity synchronization

	return &mockProviderForSync{
		BaseProvider: NewBaseProvider(name, provider, capabilities),
	}
}

func (m *mockProviderForSync) Initialize(identifier string, provider ProviderConfig) error {
	return nil
}

func (m *mockProviderForSync) SynchronizeUsers(ctx context.Context, req *SynchronizeUsersRequest) (*SynchronizeUsersResponse, error) {
	m.usersCalled = true
	m.usersCallCount++

	// Use custom function if provided
	if m.usersFunc != nil {
		return m.usersFunc(ctx, req)
	}

	if m.usersError != nil {
		return nil, m.usersError
	}
	if m.usersResponse != nil {
		return m.usersResponse, nil
	}
	// Default response
	return &SynchronizeUsersResponse{
		Identities: []Identity{
			{
				ID:    "user1",
				Label: "Test User 1",
				User: &User{
					ID:       "user1",
					Username: "testuser1",
					Email:    "user1@test.com",
					Name:     "Test User 1",
					Source:   "mock",
				},
			},
			{
				ID:    "user2",
				Label: "Test User 2",
				User: &User{
					ID:       "user2",
					Username: "testuser2",
					Email:    "user2@test.com",
					Name:     "Test User 2",
					Source:   "mock",
				},
			},
		},
	}, nil
}

func (m *mockProviderForSync) SynchronizeGroups(ctx context.Context, req *SynchronizeGroupsRequest) (*SynchronizeGroupsResponse, error) {
	m.groupsCalled = true
	if m.groupsError != nil {
		return nil, m.groupsError
	}
	if m.groupsResponse != nil {
		return m.groupsResponse, nil
	}
	// Default response
	return &SynchronizeGroupsResponse{
		Identities: []Identity{
			{
				ID:    "group1",
				Label: "Test Group 1",
				Group: &Group{
					ID:    "group1",
					Name:  "Test Group 1",
					Email: "group1@test.com",
				},
			},
			{
				ID:    "group2",
				Label: "Test Group 2",
				Group: &Group{
					ID:    "group2",
					Name:  "Test Group 2",
					Email: "group2@test.com",
				},
			},
		},
	}, nil
}

func (m *mockProviderForSync) SynchronizeIdentities(ctx context.Context, req *SynchronizeIdentitiesRequest) (*SynchronizeIdentitiesResponse, error) {
	m.identitiesCalled = true
	if m.identitiesError != nil {
		return nil, m.identitiesError
	}
	if m.identitiesResp != nil {
		return m.identitiesResp, nil
	}
	// Default response
	return &SynchronizeIdentitiesResponse{
		Identities: []Identity{
			{
				ID:    "identity1",
				Label: "Test Identity 1",
				User: &User{
					ID:       "identity1",
					Username: "identity1",
					Email:    "identity1@test.com",
					Name:     "Test Identity 1",
					Source:   "mock",
				},
			},
		},
	}, nil
}

func (m *mockProviderForSync) SynchronizeResources(ctx context.Context, req *SynchronizeResourcesRequest) (*SynchronizeResourcesResponse, error) {
	m.resourcesCalled = true
	if m.resourcesError != nil {
		return nil, m.resourcesError
	}
	if m.resourcesResponse != nil {
		return m.resourcesResponse, nil
	}
	return &SynchronizeResourcesResponse{
		Resources: []ProviderResource{
			{ID: "resource1", Name: "Test Resource 1", Type: "bucket"},
		},
	}, nil
}

func (m *mockProviderForSync) SynchronizeRoles(ctx context.Context, req *SynchronizeRolesRequest) (*SynchronizeRolesResponse, error) {
	m.rolesCalled = true
	if m.rolesError != nil {
		return nil, m.rolesError
	}
	if m.rolesResponse != nil {
		return m.rolesResponse, nil
	}
	return &SynchronizeRolesResponse{
		Roles: []ProviderRole{
			{ID: "role1", Name: "Test Role 1"},
		},
	}, nil
}

func (m *mockProviderForSync) SynchronizePermissions(ctx context.Context, req *SynchronizePermissionsRequest) (*SynchronizePermissionsResponse, error) {
	m.permissionsCalled = true
	if m.permissionsError != nil {
		return nil, m.permissionsError
	}
	if m.permissionsResponse != nil {
		return m.permissionsResponse, nil
	}
	return &SynchronizePermissionsResponse{
		Permissions: []ProviderPermission{
			{ID: "permission1", Name: "Test Permission 1"},
		},
	}, nil
}

func (m *mockProviderForSync) SynchronizeTenants(ctx context.Context, req *SynchronizeTenantsRequest) (*SynchronizeTenantsResponse, error) {
	m.tenantsCalled = true
	if m.tenantsError != nil {
		return nil, m.tenantsError
	}
	if m.tenantsResponse != nil {
		return m.tenantsResponse, nil
	}
	return &SynchronizeTenantsResponse{
		Tenants: []ProviderTenant{
			{ID: "tenant1", Name: "Test Tenant 1"},
		},
	}, nil
}

func TestSynchronize_WithUsersAndGroups(t *testing.T) {
	ctx := context.Background()

	t.Run("synchronizes users and groups when capabilities are enabled", func(t *testing.T) {
		provider := newMockProviderForSync("test-provider")

		// Call Synchronize without temporal (pure Go implementation)
		err := Synchronize(ctx, nil, provider, nil)
		require.NoError(t, err)

		// Verify both sync methods were called
		assert.True(t, provider.usersCalled, "SynchronizeUsers should have been called")
		assert.True(t, provider.groupsCalled, "SynchronizeGroups should have been called")

		// Verify identities were added to the provider
		identities, err := provider.ListIdentities(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, identities, 4, "Should have 2 users + 2 groups = 4 identities")

		// Verify we have both users and groups
		var userCount, groupCount int
		for _, identity := range identities {
			if identity.Result.User != nil {
				userCount++
			}
			if identity.Result.Group != nil {
				groupCount++
			}
		}
		assert.Equal(t, 2, userCount, "Should have 2 users")
		assert.Equal(t, 2, groupCount, "Should have 2 groups")
	})

	t.Run("synchronizes only users when only users capability is enabled", func(t *testing.T) {
		provider := ProviderConfig{
			Name:        "users-only-provider",
			Description: "Mock Provider with Users Only",
			Provider:    "mock",
			Enabled:     true,
		}

		capabilities := NewProviderCapabilities().
			WithDefaultUsersConfiguration()

		mockProvider := &mockProviderForSync{
			BaseProvider: NewBaseProvider("users-only", provider, capabilities),
		}

		err := Synchronize(ctx, nil, mockProvider, nil)
		require.NoError(t, err)

		assert.True(t, mockProvider.usersCalled, "SynchronizeUsers should have been called")
		assert.False(t, mockProvider.groupsCalled, "SynchronizeGroups should NOT have been called")

		// Verify only user identities were added
		identities, err := mockProvider.ListIdentities(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, identities, 2, "Should have only 2 user identities")

		for _, identity := range identities {
			assert.NotNil(t, identity.Result.User, "All identities should be users")
			assert.Nil(t, identity.Result.Group, "No group identities should exist")
		}
	})

	t.Run("synchronizes only groups when only groups capability is enabled", func(t *testing.T) {
		provider := ProviderConfig{
			Name:        "groups-only-provider",
			Description: "Mock Provider with Groups Only",
			Provider:    "mock",
			Enabled:     true,
		}

		capabilities := NewProviderCapabilities().
			WithDefaultGroupsConfiguration()

		mockProvider := &mockProviderForSync{
			BaseProvider: NewBaseProvider("groups-only", provider, capabilities),
		}

		err := Synchronize(ctx, nil, mockProvider, nil)
		require.NoError(t, err)

		assert.False(t, mockProvider.usersCalled, "SynchronizeUsers should NOT have been called")
		assert.True(t, mockProvider.groupsCalled, "SynchronizeGroups should have been called")

		// Verify only group identities were added
		identities, err := mockProvider.ListIdentities(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, identities, 2, "Should have only 2 group identities")

		for _, identity := range identities {
			assert.Nil(t, identity.Result.User, "No user identities should exist")
			assert.NotNil(t, identity.Result.Group, "All identities should be groups")
		}
	})

	t.Run("handles pagination correctly", func(t *testing.T) {
		provider := newMockProviderForSync("pagination-provider")

		// Set up paginated response using custom function
		provider.usersFunc = func(ctx context.Context, req *SynchronizeUsersRequest) (*SynchronizeUsersResponse, error) {
			if provider.usersCallCount == 1 {
				// First call - return page with pagination token
				return &SynchronizeUsersResponse{
					Identities: []Identity{
						{
							ID:    "user1",
							Label: "User 1",
							User:  &User{ID: "user1", Username: "user1", Email: "user1@test.com"},
						},
					},
					Pagination: &PaginationOptions{
						Token:    "next-page-token",
						PageSize: 1,
					},
				}, nil
			}
			// Second call - no more pages
			return &SynchronizeUsersResponse{
				Identities: []Identity{
					{
						ID:    "user2",
						Label: "User 2",
						User:  &User{ID: "user2", Username: "user2", Email: "user2@test.com"},
					},
				},
			}, nil
		}

		err := Synchronize(ctx, nil, provider, nil)
		require.NoError(t, err)

		// Should have called SynchronizeUsers twice due to pagination
		assert.Equal(t, 2, provider.usersCallCount, "Should have made 2 calls for pagination")
	})

	t.Run("returns error when sync fails", func(t *testing.T) {
		provider := newMockProviderForSync("error-provider")
		provider.usersError = assert.AnError

		err := Synchronize(ctx, nil, provider, nil)
		assert.Error(t, err, "Should return error when sync fails")
	})

	t.Run("skips sync when provider has no capabilities", func(t *testing.T) {
		provider := ProviderConfig{
			Name:        "no-capability-provider",
			Description: "Mock Provider with No Sync Capabilities",
			Provider:    "mock",
			Enabled:     true,
		}

		// No sync capabilities
		capabilities := NewProviderCapabilities()

		mockProvider := &mockProviderForSync{
			BaseProvider: NewBaseProvider("no-capabilities", provider, capabilities),
		}

		err := Synchronize(ctx, nil, mockProvider, nil)
		require.NoError(t, err)

		assert.False(t, mockProvider.usersCalled, "SynchronizeUsers should NOT have been called")
		assert.False(t, mockProvider.groupsCalled, "SynchronizeGroups should NOT have been called")
	})

	t.Run("handles specific sync request", func(t *testing.T) {
		provider := newMockProviderForSync("specific-sync-provider")

		// Request only users sync
		syncRequest := &SynchronizeRequest{
			ProviderIdentifier: "specific-sync-provider",
			Requests:           []SynchronizeCapability{SynchronizeUsers},
		}

		err := Synchronize(ctx, nil, provider, syncRequest)
		require.NoError(t, err)

		assert.True(t, provider.usersCalled, "SynchronizeUsers should have been called")
		assert.False(t, provider.groupsCalled, "SynchronizeGroups should NOT have been called")
		assert.False(t, provider.identitiesCalled, "SynchronizeIdentities should NOT have been called")
	})

	t.Run("handles identities capability separately", func(t *testing.T) {
		provider := ProviderConfig{
			Name:        "identities-provider",
			Description: "Mock Provider with Identities Capability",
			Provider:    "mock",
			Enabled:     true,
		}

		capabilities := NewProviderCapabilities().
			WithDefaultIdentitiesConfiguration()

		mockProvider := &mockProviderForSync{
			BaseProvider: NewBaseProvider("identities-only", provider, capabilities),
		}

		err := Synchronize(ctx, nil, mockProvider, nil)
		require.NoError(t, err)

		assert.True(t, mockProvider.identitiesCalled, "SynchronizeIdentities should have been called")
		assert.False(t, mockProvider.usersCalled, "SynchronizeUsers should NOT have been called")
		assert.False(t, mockProvider.groupsCalled, "SynchronizeGroups should NOT have been called")

		// Verify identity was added
		identities, err := mockProvider.ListIdentities(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, identities, 1, "Should have 1 identity")
	})
}

func TestSynchronize_NilProvider(t *testing.T) {
	ctx := context.Background()
	err := Synchronize(ctx, nil, nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "provider client is nil")
}

func TestSynchronize_ConcurrentSyncs(t *testing.T) {
	ctx := context.Background()
	provider := newMockProviderForSync("concurrent-provider")

	// Run multiple syncs concurrently
	var wg sync.WaitGroup
	errors := make(chan error, 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := Synchronize(ctx, nil, provider, nil)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent sync failed: %v", err)
	}

	// Verify identities were properly synchronized
	identities, err := provider.ListIdentities(ctx, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, identities, "Should have identities after concurrent syncs")
}

func TestSynchronize_StorePopulation(t *testing.T) {
	ctx := context.Background()

	newMockWith := func(name string, caps *ProviderCapabilities) *mockProviderForSync {
		return &mockProviderForSync{
			BaseProvider: NewBaseProvider(name, ProviderConfig{
				Name:     name,
				Provider: "mock",
				Enabled:  true,
			}, caps),
		}
	}

	t.Run("resources are stored after sync", func(t *testing.T) {
		mock := newMockWith("resources-provider",
			NewProviderCapabilities().WithDefaultResourcesConfiguration())

		err := Synchronize(ctx, nil, mock, nil)
		require.NoError(t, err)

		assert.True(t, mock.resourcesCalled, "SynchronizeResources should have been called")
		assert.False(t, mock.usersCalled, "SynchronizeUsers should NOT have been called")
		assert.False(t, mock.rolesCalled, "SynchronizeRoles should NOT have been called")
		assert.False(t, mock.permissionsCalled, "SynchronizePermissions should NOT have been called")

		resources, err := mock.ListResources(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, resources, 1, "Should have 1 resource")
		assert.Equal(t, "resource1", resources[0].Result.ID)
	})

	t.Run("roles are stored after sync", func(t *testing.T) {
		mock := newMockWith("roles-provider",
			NewProviderCapabilities().WithDefaultRolesConfiguration())

		err := Synchronize(ctx, nil, mock, nil)
		require.NoError(t, err)

		assert.True(t, mock.rolesCalled, "SynchronizeRoles should have been called")
		assert.False(t, mock.usersCalled, "SynchronizeUsers should NOT have been called")
		assert.False(t, mock.resourcesCalled, "SynchronizeResources should NOT have been called")
		assert.False(t, mock.permissionsCalled, "SynchronizePermissions should NOT have been called")

		roles, err := mock.ListRoles(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, roles, 1, "Should have 1 role")
		assert.Equal(t, "role1", roles[0].Result.ID)
	})

	t.Run("permissions are stored after sync", func(t *testing.T) {
		mock := newMockWith("permissions-provider",
			NewProviderCapabilities().WithDefaultPermissionsConfiguration())

		err := Synchronize(ctx, nil, mock, nil)
		require.NoError(t, err)

		assert.True(t, mock.permissionsCalled, "SynchronizePermissions should have been called")
		assert.False(t, mock.usersCalled, "SynchronizeUsers should NOT have been called")
		assert.False(t, mock.resourcesCalled, "SynchronizeResources should NOT have been called")
		assert.False(t, mock.rolesCalled, "SynchronizeRoles should NOT have been called")

		permissions, err := mock.ListPermissions(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, permissions, 1, "Should have 1 permission")
		assert.Equal(t, "permission1", permissions[0].Result.ID)
	})

	t.Run("tenants are stored after sync", func(t *testing.T) {
		mock := newMockWith("tenants-provider",
			NewProviderCapabilities().WithDefaultTenantsConfiguration())

		err := Synchronize(ctx, nil, mock, nil)
		require.NoError(t, err)

		assert.True(t, mock.tenantsCalled, "SynchronizeTenants should have been called")
		assert.False(t, mock.usersCalled, "SynchronizeUsers should NOT have been called")
		assert.False(t, mock.resourcesCalled, "SynchronizeResources should NOT have been called")
		assert.False(t, mock.rolesCalled, "SynchronizeRoles should NOT have been called")

		tenants, err := mock.ListTenants(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, tenants, 1, "Should have 1 tenant")
		assert.Equal(t, "tenant1", tenants[0].Result.ID)
	})

	t.Run("all capabilities populate their stores independently", func(t *testing.T) {
		mock := newMockWith("all-capabilities-provider",
			NewProviderCapabilities().
				WithDefaultUsersConfiguration().
				WithDefaultGroupsConfiguration().
				WithDefaultResourcesConfiguration().
				WithDefaultRolesConfiguration().
				WithDefaultPermissionsConfiguration().
				WithDefaultTenantsConfiguration())

		err := Synchronize(ctx, nil, mock, nil)
		require.NoError(t, err)

		assert.True(t, mock.usersCalled)
		assert.True(t, mock.groupsCalled)
		assert.True(t, mock.resourcesCalled)
		assert.True(t, mock.rolesCalled)
		assert.True(t, mock.permissionsCalled)
		assert.True(t, mock.tenantsCalled)

		identities, err := mock.ListIdentities(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, identities, 4, "Should have 2 users + 2 groups")

		resources, err := mock.ListResources(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, resources, 1, "Should have 1 resource")

		roles, err := mock.ListRoles(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, roles, 1, "Should have 1 role")

		permissions, err := mock.ListPermissions(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, permissions, 1, "Should have 1 permission")

		tenants, err := mock.ListTenants(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, tenants, 1, "Should have 1 tenant")
	})
}
