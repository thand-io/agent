package aws_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/testing/temporaltest"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// ---------------------------------------------------------------------------
// Mock AWS provider for Temporal workflow sync tests
// ---------------------------------------------------------------------------

// mockAwsSyncProvider wraps BaseProvider with configurable sync responses that
// simulate an AWS provider returning tenants, users, and groups.
type mockAwsSyncProvider struct {
	*models.BaseProvider
	mu sync.Mutex

	// Tracking
	tenantsCalled    bool
	usersCalled      bool
	groupsCalled     bool
	tenantsCallCount int
	usersCallCount   int
	groupsCallCount  int

	// Configurable responses / errors
	tenantsResponse *models.SynchronizeTenantsResponse
	usersResponse   *models.SynchronizeUsersResponse
	groupsResponse  *models.SynchronizeGroupsResponse
	tenantsError    error
	usersError      error
	groupsError     error

	// Optional per-call functions for pagination testing
	tenantsFunc func(ctx context.Context, req *models.SynchronizeTenantsRequest) (*models.SynchronizeTenantsResponse, error)
	usersFunc   func(ctx context.Context, req *models.SynchronizeUsersRequest) (*models.SynchronizeUsersResponse, error)
	groupsFunc  func(ctx context.Context, req *models.SynchronizeGroupsRequest) (*models.SynchronizeGroupsResponse, error)
}

func newMockAwsSyncProvider(identifier string, caps *models.ProviderCapabilities) *mockAwsSyncProvider {
	cfg := models.ProviderConfig{
		Name:        identifier,
		Description: "Mock AWS provider for Temporal sync tests",
		Provider:    "aws",
		Enabled:     true,
	}
	return &mockAwsSyncProvider{
		BaseProvider: models.NewBaseProvider(identifier, cfg, caps),
	}
}

// defaultAwsCapabilities returns Tenants + Users + Groups (mirrors real AWS).
func defaultAwsCapabilities() *models.ProviderCapabilities {
	return models.NewProviderCapabilities().
		WithDefaultTenantsConfiguration().
		WithDefaultUsersConfiguration().
		WithDefaultGroupsConfiguration()
}

// --- Default test data (realistic AWS-shaped) --------------------------------

func defaultTenants() []models.ProviderTenant {
	return []models.ProviderTenant{
		{ID: "111111111111", Type: "account", Name: "Production"},
		{ID: "222222222222", Type: "account", Name: "Staging"},
		{ID: "333333333333", Type: "account", Name: "Development"},
	}
}

func defaultUsers() []models.Identity {
	return []models.Identity{
		{
			ID:    "u-aws-001",
			Label: "Alice Admin",
			User: &models.User{
				ID:       "u-aws-001",
				Username: "alice.admin",
				Email:    "alice@example.com",
				Name:     "Alice Admin",
				Source:   "aws-identity-store",
			},
		},
		{
			ID:    "u-aws-002",
			Label: "Bob Builder",
			User: &models.User{
				ID:       "u-aws-002",
				Username: "bob.builder",
				Email:    "bob@example.com",
				Name:     "Bob Builder",
				Source:   "aws-identity-store",
			},
		},
		{
			ID:    "u-aws-003",
			Label: "Carol Cloud",
			User: &models.User{
				ID:       "u-aws-003",
				Username: "carol.cloud",
				Email:    "carol@example.com",
				Name:     "Carol Cloud",
				Source:   "aws-identity-store",
			},
		},
	}
}

func defaultGroups() []models.Identity {
	return []models.Identity{
		{
			ID:    "g-aws-001",
			Label: "Admins",
			Group: &models.Group{
				ID:    "g-aws-001",
				Name:  "Admins",
				Email: "admins@example.com",
			},
		},
		{
			ID:    "g-aws-002",
			Label: "Developers",
			Group: &models.Group{
				ID:    "g-aws-002",
				Name:  "Developers",
				Email: "developers@example.com",
			},
		},
	}
}

// --- Sync methods (override BaseProvider defaults) ---------------------------

func (m *mockAwsSyncProvider) SynchronizeTenants(ctx context.Context, req *models.SynchronizeTenantsRequest) (*models.SynchronizeTenantsResponse, error) {
	m.mu.Lock()
	m.tenantsCalled = true
	m.tenantsCallCount++
	m.mu.Unlock()

	if m.tenantsFunc != nil {
		return m.tenantsFunc(ctx, req)
	}
	if m.tenantsError != nil {
		return nil, m.tenantsError
	}
	if m.tenantsResponse != nil {
		return m.tenantsResponse, nil
	}
	return &models.SynchronizeTenantsResponse{Tenants: defaultTenants()}, nil
}

func (m *mockAwsSyncProvider) SynchronizeUsers(ctx context.Context, req *models.SynchronizeUsersRequest) (*models.SynchronizeUsersResponse, error) {
	m.mu.Lock()
	m.usersCalled = true
	m.usersCallCount++
	m.mu.Unlock()

	if m.usersFunc != nil {
		return m.usersFunc(ctx, req)
	}
	if m.usersError != nil {
		return nil, m.usersError
	}
	if m.usersResponse != nil {
		return m.usersResponse, nil
	}
	return &models.SynchronizeUsersResponse{Identities: defaultUsers()}, nil
}

func (m *mockAwsSyncProvider) SynchronizeGroups(ctx context.Context, req *models.SynchronizeGroupsRequest) (*models.SynchronizeGroupsResponse, error) {
	m.mu.Lock()
	m.groupsCalled = true
	m.groupsCallCount++
	m.mu.Unlock()

	if m.groupsFunc != nil {
		return m.groupsFunc(ctx, req)
	}
	if m.groupsError != nil {
		return nil, m.groupsError
	}
	if m.groupsResponse != nil {
		return m.groupsResponse, nil
	}
	return &models.SynchronizeGroupsResponse{Identities: defaultGroups()}, nil
}

// ---------------------------------------------------------------------------
// Temporal test-environment helpers
// ---------------------------------------------------------------------------

// patchUpstreamNoOp is the dummy patch-provider-upstream activity matching the
// signature from internal/config/temporal_activities.go.
func patchUpstreamNoOp(
	_ context.Context,
	_ models.SynchronizeCapability,
	_ string,
	_ any,
) error {
	return nil
}

// registerSyncActivities registers the provider's local activities and the
// upstream-patch no-op on a Temporal test workflow environment.
func registerSyncActivities(env *testsuite.TestWorkflowEnvironment, provider *mockAwsSyncProvider) {
	identifier := provider.GetIdentifier()
	pa := models.NewProviderActivities(provider)

	// Register each sync activity under its canonical name.
	type namedActivity struct {
		fn   any
		name string
	}
	for _, na := range []namedActivity{
		{pa.SynchronizeTenants, "SynchronizeTenants"},
		{pa.SynchronizeUsers, "SynchronizeUsers"},
		{pa.SynchronizeGroups, "SynchronizeGroups"},
		// Non-implemented capabilities are still registered so runSyncLoop
		// can invoke them — the activity wrapper returns a non-retryable error
		// which paginatedSync treats as ErrNotImplemented.
		{pa.SynchronizeIdentities, "SynchronizeIdentities"},
		{pa.SynchronizeResources, "SynchronizeResources"},
		{pa.SynchronizeRoles, "SynchronizeRoles"},
		{pa.SynchronizePermissions, "SynchronizePermissions"},
	} {
		env.RegisterActivityWithOptions(na.fn, activity.RegisterOptions{
			Name: models.CreateTemporalProviderWorkflowName(identifier, na.name),
		})
	}

	// Register the upstream-patch activity as a no-op.
	env.RegisterActivityWithOptions(patchUpstreamNoOp, activity.RegisterOptions{
		Name: models.TemporalPatchProviderUpstreamActivityName,
	})
}

// executeSyncWorkflow creates a fresh test env, registers activities, executes
// the sync workflow, and returns the env for assertions.
func executeSyncWorkflow(
	t *testing.T,
	provider *mockAwsSyncProvider,
	req models.SynchronizeRequest,
) *testsuite.TestWorkflowEnvironment {
	t.Helper()

	temporaltest.SeedBinaryChecksum()
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	registerSyncActivities(env, provider)

	wf := models.CreateProviderSynchronizeWorkflow(provider)
	env.RegisterWorkflowWithOptions(wf, workflow.RegisterOptions{
		Name: models.CreateTemporalProviderWorkflowName(req.ProviderIdentifier, models.TemporalSynchronizeWorkflowName),
	})

	env.ExecuteWorkflow(wf, req)

	return env
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestProviderSyncWorkflow_AllCapabilities(t *testing.T) {
	provider := newMockAwsSyncProvider("mock-aws", defaultAwsCapabilities())
	ctx := context.Background()

	env := executeSyncWorkflow(t, provider, models.SynchronizeRequest{
		ProviderIdentifier: "mock-aws",
	})

	require.True(t, env.IsWorkflowCompleted(), "workflow should complete")
	require.NoError(t, env.GetWorkflowError(), "workflow should succeed")

	// Verify all three sync activities were invoked.
	assert.True(t, provider.tenantsCalled, "SynchronizeTenants should have been called")
	assert.True(t, provider.usersCalled, "SynchronizeUsers should have been called")
	assert.True(t, provider.groupsCalled, "SynchronizeGroups should have been called")

	// Verify tenants were stored.
	tenants, err := provider.ListTenants(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, tenants, 3, "should have 3 AWS account tenants")
	tenantNames := make(map[string]bool)
	for _, tr := range tenants {
		tenantNames[tr.Result.Name] = true
	}
	assert.True(t, tenantNames["Production"])
	assert.True(t, tenantNames["Staging"])
	assert.True(t, tenantNames["Development"])

	// Verify identities (users + groups) were stored.
	identities, err := provider.ListIdentities(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, identities, 5, "should have 3 users + 2 groups = 5 identities")

	var userCount, groupCount int
	for _, id := range identities {
		if id.Result.User != nil {
			userCount++
		}
		if id.Result.Group != nil {
			groupCount++
		}
	}
	assert.Equal(t, 3, userCount, "should have 3 users")
	assert.Equal(t, 2, groupCount, "should have 2 groups")
}

func TestProviderSyncWorkflow_TenantsOnly(t *testing.T) {
	provider := newMockAwsSyncProvider("mock-aws", defaultAwsCapabilities())
	ctx := context.Background()

	env := executeSyncWorkflow(t, provider, models.SynchronizeRequest{
		ProviderIdentifier: "mock-aws",
		Requests:           []models.SynchronizeCapability{models.SynchronizeTenants},
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	assert.True(t, provider.tenantsCalled, "SynchronizeTenants should have been called")
	assert.False(t, provider.usersCalled, "SynchronizeUsers should NOT have been called")
	assert.False(t, provider.groupsCalled, "SynchronizeGroups should NOT have been called")

	tenants, err := provider.ListTenants(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, tenants, 3)
}

func TestProviderSyncWorkflow_UsersAndGroupsOnly(t *testing.T) {
	provider := newMockAwsSyncProvider("mock-aws", defaultAwsCapabilities())
	ctx := context.Background()

	env := executeSyncWorkflow(t, provider, models.SynchronizeRequest{
		ProviderIdentifier: "mock-aws",
		Requests: []models.SynchronizeCapability{
			models.SynchronizeUsers,
			models.SynchronizeGroups,
		},
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	assert.False(t, provider.tenantsCalled, "SynchronizeTenants should NOT have been called")
	assert.True(t, provider.usersCalled, "SynchronizeUsers should have been called")
	assert.True(t, provider.groupsCalled, "SynchronizeGroups should have been called")

	// Identities should be populated (users + groups).
	identities, err := provider.ListIdentities(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, identities, 5, "3 users + 2 groups")

	// Tenants should NOT have been populated.
	tenants, err := provider.ListTenants(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, tenants)
}

func TestProviderSyncWorkflow_Pagination(t *testing.T) {
	provider := newMockAwsSyncProvider("mock-aws", defaultAwsCapabilities())
	ctx := context.Background()

	// Configure paginated user responses: page 1 returns 2 users + token,
	// page 2 returns 1 user + no token (done).
	provider.usersFunc = func(_ context.Context, req *models.SynchronizeUsersRequest) (*models.SynchronizeUsersResponse, error) {
		provider.mu.Lock()
		callNum := provider.usersCallCount // already incremented by SynchronizeUsers
		provider.mu.Unlock()

		if callNum == 1 {
			return &models.SynchronizeUsersResponse{
				Identities: []models.Identity{
					{ID: "u-page1-1", Label: "Page1 User1", User: &models.User{ID: "u-page1-1", Username: "page1user1", Email: "p1u1@example.com", Source: "aws"}},
					{ID: "u-page1-2", Label: "Page1 User2", User: &models.User{ID: "u-page1-2", Username: "page1user2", Email: "p1u2@example.com", Source: "aws"}},
				},
				Pagination: &models.PaginationOptions{Token: "next-page"},
			}, nil
		}
		return &models.SynchronizeUsersResponse{
			Identities: []models.Identity{
				{ID: "u-page2-1", Label: "Page2 User1", User: &models.User{ID: "u-page2-1", Username: "page2user1", Email: "p2u1@example.com", Source: "aws"}},
			},
		}, nil
	}

	env := executeSyncWorkflow(t, provider, models.SynchronizeRequest{
		ProviderIdentifier: "mock-aws",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	assert.Equal(t, 2, provider.usersCallCount, "SynchronizeUsers should have been called twice for pagination")

	identities, err := provider.ListIdentities(ctx, nil)
	require.NoError(t, err)

	// 3 paginated users + 2 default groups = 5
	var userCount int
	for _, id := range identities {
		if id.Result.User != nil {
			userCount++
		}
	}
	assert.Equal(t, 3, userCount, "should have 3 users across 2 pages")
}

func TestProviderSyncWorkflow_ErrorHandling(t *testing.T) {
	provider := newMockAwsSyncProvider("mock-aws", defaultAwsCapabilities())

	provider.usersError = assert.AnError

	env := executeSyncWorkflow(t, provider, models.SynchronizeRequest{
		ProviderIdentifier: "mock-aws",
	})

	require.True(t, env.IsWorkflowCompleted())

	err := env.GetWorkflowError()
	require.Error(t, err, "workflow should fail when a sync activity errors")
	assert.Contains(t, err.Error(), "synchronization failed")
}

func TestProviderSyncWorkflow_NotImplementedSkipped(t *testing.T) {
	// Enable identities capability but let BaseProvider's default
	// return ErrNotImplemented — the workflow should still complete.
	caps := models.NewProviderCapabilities().
		WithDefaultTenantsConfiguration().
		WithDefaultUsersConfiguration().
		WithDefaultGroupsConfiguration().
		WithDefaultIdentitiesConfiguration()

	provider := newMockAwsSyncProvider("mock-aws", caps)
	ctx := context.Background()

	env := executeSyncWorkflow(t, provider, models.SynchronizeRequest{
		ProviderIdentifier: "mock-aws",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError(), "ErrNotImplemented activities should be silently skipped")

	// The implemented activities should still have run.
	assert.True(t, provider.tenantsCalled)
	assert.True(t, provider.usersCalled)
	assert.True(t, provider.groupsCalled)

	tenants, err := provider.ListTenants(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, tenants, 3)

	identities, err := provider.ListIdentities(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, identities, 5)
}

func TestProviderSyncWorkflow_EmptyProviderIdentifier(t *testing.T) {
	provider := newMockAwsSyncProvider("mock-aws", defaultAwsCapabilities())

	env := executeSyncWorkflow(t, provider, models.SynchronizeRequest{
		ProviderIdentifier: "", // empty — should fail
	})

	require.True(t, env.IsWorkflowCompleted())
	err := env.GetWorkflowError()
	require.Error(t, err, "empty provider identifier should fail")
	assert.Contains(t, err.Error(), "provider identifier is required")
}

func TestProviderSyncWorkflow_MultiPageTenants(t *testing.T) {
	provider := newMockAwsSyncProvider("mock-aws", defaultAwsCapabilities())
	ctx := context.Background()

	// Simulate AWS Organizations pagination for tenants.
	provider.tenantsFunc = func(_ context.Context, req *models.SynchronizeTenantsRequest) (*models.SynchronizeTenantsResponse, error) {
		provider.mu.Lock()
		callNum := provider.tenantsCallCount
		provider.mu.Unlock()

		if callNum == 1 {
			return &models.SynchronizeTenantsResponse{
				Tenants: []models.ProviderTenant{
					{ID: "111111111111", Type: "account", Name: "Account1"},
					{ID: "222222222222", Type: "account", Name: "Account2"},
				},
				Pagination: &models.PaginationOptions{Token: "page2"},
			}, nil
		}
		return &models.SynchronizeTenantsResponse{
			Tenants: []models.ProviderTenant{
				{ID: "333333333333", Type: "account", Name: "Account3"},
			},
		}, nil
	}

	env := executeSyncWorkflow(t, provider, models.SynchronizeRequest{
		ProviderIdentifier: "mock-aws",
		Requests:           []models.SynchronizeCapability{models.SynchronizeTenants},
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	assert.Equal(t, 2, provider.tenantsCallCount, "SynchronizeTenants should be called twice")

	tenants, err := provider.ListTenants(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, tenants, 3, "all 3 accounts across 2 pages should be collected")
}
