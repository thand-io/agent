package models

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a BaseProvider with role support
func newTestProviderWithRoles() *BaseProvider {
	provider := Provider{
		Name:        "test-provider",
		Description: "Test Provider",
		Provider:    "test",
	}

	return NewBaseProvider("test-provider-id", provider,
		NewProviderCapabilities().WithDefaultRolesConfiguration())
}

func TestBaseProvider_SetRoles(t *testing.T) {
	p := newTestProviderWithRoles()

	roles := []ProviderRole{
		{ID: "role1", Name: "Administrator", Title: "Admin", Description: "Full access"},
		{ID: "role2", Name: "Developer", Title: "Dev", Description: "Development access"},
		{ID: "role3", Name: "ReadOnly", Title: "Reader", Description: "Read-only access"},
	}

	// Test SetRoles
	p.SetRoles(roles)

	// Give time for async index building to start
	time.Sleep(10 * time.Millisecond)

	// Verify roles are set
	p.rbac.mu.RLock()
	assert.Len(t, p.rbac.roles, 3)
	assert.Len(t, p.rbac.rolesMap, 6) // Each role has 2 keys (ID and Name)
	p.rbac.mu.RUnlock()

	// Verify map contains all expected keys (lowercase)
	p.rbac.mu.RLock()
	assert.Contains(t, p.rbac.rolesMap, "role1")
	assert.Contains(t, p.rbac.rolesMap, "administrator")
	assert.Contains(t, p.rbac.rolesMap, "role2")
	assert.Contains(t, p.rbac.rolesMap, "developer")
	assert.Contains(t, p.rbac.rolesMap, "role3")
	assert.Contains(t, p.rbac.rolesMap, "readonly")
	p.rbac.mu.RUnlock()

	// Verify map points to correct roles
	p.rbac.mu.RLock()
	assert.Equal(t, "role1", p.rbac.rolesMap["role1"].ID)
	assert.Equal(t, "Administrator", p.rbac.rolesMap["administrator"].Name)
	assert.Equal(t, "Developer", p.rbac.rolesMap["developer"].Name)
	p.rbac.mu.RUnlock()
}

func TestBaseProvider_SetRoles_ReplacesExisting(t *testing.T) {
	p := newTestProviderWithRoles()

	// Set initial roles
	initialRoles := []ProviderRole{
		{ID: "role1", Name: "Role One"},
		{ID: "role2", Name: "Role Two"},
	}
	p.SetRoles(initialRoles)

	// Replace with new roles
	newRoles := []ProviderRole{
		{ID: "role3", Name: "Role Three"},
	}
	p.SetRoles(newRoles)

	// Verify old roles are replaced
	p.rbac.mu.RLock()
	assert.Len(t, p.rbac.roles, 1)
	assert.Equal(t, "role3", p.rbac.roles[0].ID)
	assert.NotContains(t, p.rbac.rolesMap, "role1")
	assert.NotContains(t, p.rbac.rolesMap, "role2")
	assert.Contains(t, p.rbac.rolesMap, "role3")
	p.rbac.mu.RUnlock()
}

func TestBaseProvider_AddRoles(t *testing.T) {
	p := newTestProviderWithRoles()

	// Set initial roles
	initialRoles := []ProviderRole{
		{ID: "role1", Name: "Role One"},
	}
	p.SetRoles(initialRoles)
	time.Sleep(10 * time.Millisecond)

	// Add new roles
	p.AddRoles(
		ProviderRole{ID: "role2", Name: "Role Two"},
		ProviderRole{ID: "role3", Name: "Role Three"},
	)
	time.Sleep(10 * time.Millisecond)

	// Verify all roles are present
	p.rbac.mu.RLock()
	assert.Len(t, p.rbac.roles, 3)
	assert.Contains(t, p.rbac.rolesMap, "role1")
	assert.Contains(t, p.rbac.rolesMap, "role2")
	assert.Contains(t, p.rbac.rolesMap, "role3")
	p.rbac.mu.RUnlock()
}

func TestBaseProvider_AddRoles_FiltersDuplicates(t *testing.T) {
	p := newTestProviderWithRoles()

	// Set initial roles
	initialRoles := []ProviderRole{
		{ID: "role1", Name: "Role One"},
		{ID: "role2", Name: "Role Two"},
	}
	p.SetRoles(initialRoles)
	time.Sleep(10 * time.Millisecond)

	// Try to add duplicate and new role
	p.AddRoles(
		ProviderRole{ID: "role1", Name: "Role One"},  // duplicate by ID
		ProviderRole{ID: "role3", Name: "Role Two"},  // duplicate by Name
		ProviderRole{ID: "role4", Name: "Role Four"}, // new
	)
	time.Sleep(10 * time.Millisecond)

	// Verify duplicates are filtered
	p.rbac.mu.RLock()
	assert.Len(t, p.rbac.roles, 3, "Should have 3 roles (2 initial + 1 new)")
	assert.Contains(t, p.rbac.rolesMap, "role1")
	assert.Contains(t, p.rbac.rolesMap, "role2")
	assert.Contains(t, p.rbac.rolesMap, "role4")
	p.rbac.mu.RUnlock()
}

func TestBaseProvider_GetRole(t *testing.T) {
	p := newTestProviderWithRoles()

	roles := []ProviderRole{
		{
			ID:          "role-1",
			Name:        "Administrator",
			Title:       "Admin",
			Description: "Full system access",
		},
		{
			ID:          "role-2",
			Name:        "Developer",
			Title:       "Dev",
			Description: "Development access",
		},
	}
	p.SetRoles(roles)

	ctx := context.Background()

	// Test getting role by ID
	role, err := p.GetRole(ctx, "role-1")
	require.NoError(t, err)
	require.NotNil(t, role)
	assert.Equal(t, "role-1", role.ID)
	assert.Equal(t, "Administrator", role.Name)

	// Test getting role by Name (case-insensitive)
	role, err = p.GetRole(ctx, "administrator")
	require.NoError(t, err)
	require.NotNil(t, role)
	assert.Equal(t, "role-1", role.ID)

	// Test getting role by Name with different case
	role, err = p.GetRole(ctx, "DEVELOPER")
	require.NoError(t, err)
	require.NotNil(t, role)
	assert.Equal(t, "role-2", role.ID)

	// Test AWS ARN prefix stripping
	role, err = p.GetRole(ctx, "arn:aws:iam::aws:policy/Administrator")
	require.NoError(t, err)
	require.NotNil(t, role)
	assert.Equal(t, "role-1", role.ID)

	// Test getting non-existent role
	role, err = p.GetRole(ctx, "non-existent")
	assert.Error(t, err)
	assert.Nil(t, role)
	assert.Contains(t, err.Error(), "role not found")
}

func TestBaseProvider_GetRole_WithoutCapability(t *testing.T) {
	// Create provider without role capability
	p := NewBaseProvider("test", Provider{Name: "test"}, NewProviderCapabilities())

	ctx := context.Background()
	role, err := p.GetRole(ctx, "role1")
	assert.Error(t, err)
	assert.Nil(t, role)
	assert.Contains(t, err.Error(), "no roles")
}

func TestBaseProvider_ListRoles(t *testing.T) {
	p := newTestProviderWithRoles()

	roles := []ProviderRole{
		{ID: "admin-role", Name: "Production Administrator"},
		{ID: "dev-role", Name: "Development User"},
		{ID: "staging-role", Name: "Staging Operator"},
	}
	p.SetRoles(roles)

	ctx := context.Background()

	// Test listing all roles (no filter)
	results, err := p.ListRoles(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Test listing with empty search request
	results, err = p.ListRoles(ctx, &SearchRequest{})
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Test listing with filter
	results, err = p.ListRoles(ctx, &SearchRequest{
		Terms: []string{"production"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "admin-role", results[0].Result.ID)

	// Test listing with case-insensitive filter
	results, err = p.ListRoles(ctx, &SearchRequest{
		Terms: []string{"DEVELOPMENT"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "dev-role", results[0].Result.ID)

	// Test partial match
	results, err = p.ListRoles(ctx, &SearchRequest{
		Terms: []string{"user"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestBaseProvider_ListRoles_WithoutCapability(t *testing.T) {
	p := NewBaseProvider("test", Provider{Name: "test"}, NewProviderCapabilities())

	ctx := context.Background()
	results, err := p.ListRoles(ctx, nil)
	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "no roles")
}

func TestBaseProvider_ListRoles_Search(t *testing.T) {
	p := NewBaseProvider("test", Provider{
		Name: "Test Provider",
	}, NewProviderCapabilities().WithDefaultRolesConfiguration())

	roles := []ProviderRole{
		{
			ID:          "admin-role",
			Name:        "Administrator",
			Description: "Full system access",
		},
		{
			ID:          "dev-role",
			Name:        "Developer",
			Description: "Development environment access",
		},
	}

	p.SetRoles(roles)

	// Wait for index to be built
	time.Sleep(500 * time.Millisecond)

	ctx := context.Background()
	searchReq := &SearchRequest{
		Query: "Administrator*",
		Terms: []string{"Administrator"},
	}

	results, err := p.ListRoles(ctx, searchReq)
	require.NoError(t, err)
	assert.NotEmpty(t, results, "Should find role by name")

	if len(results) > 0 {
		assert.Equal(t, "admin-role", results[0].Result.ID)
	}
}

// CONCURRENCY TESTS

func TestBaseProvider_SetRoles_Concurrency(t *testing.T) {
	p := newTestProviderWithRoles()

	var wg sync.WaitGroup
	goroutines := 100
	rolesPerGoroutine := 10

	// Concurrently set roles from multiple goroutines
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			roles := make([]ProviderRole, rolesPerGoroutine)
			for j := 0; j < rolesPerGoroutine; j++ {
				roles[j] = ProviderRole{
					ID:   fmt.Sprintf("role-%d-%d", id, j),
					Name: fmt.Sprintf("Role %d-%d", id, j),
				}
			}
			p.SetRoles(roles)
		}(i)
	}

	wg.Wait()
	time.Sleep(50 * time.Millisecond) // Let async operations settle

	// Verify no race conditions occurred and data is consistent
	p.rbac.mu.RLock()
	assert.NotNil(t, p.rbac.roles)
	assert.NotNil(t, p.rbac.rolesMap)
	roleCount := len(p.rbac.roles)
	mapCount := len(p.rbac.rolesMap)
	p.rbac.mu.RUnlock()

	// The last SetRoles wins, should have rolesPerGoroutine items
	assert.Equal(t, rolesPerGoroutine, roleCount)
	// Each role has 2 keys (ID and Name)
	assert.Equal(t, rolesPerGoroutine*2, mapCount)
}

func TestBaseProvider_AddRoles_Concurrency(t *testing.T) {
	p := newTestProviderWithRoles()

	// Set initial state
	p.SetRoles([]ProviderRole{
		{ID: "initial-1", Name: "Initial One"},
	})
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	goroutines := 50
	rolesPerGoroutine := 2

	// Concurrently add roles from multiple goroutines
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			roles := make([]ProviderRole, rolesPerGoroutine)
			for j := 0; j < rolesPerGoroutine; j++ {
				roles[j] = ProviderRole{
					ID:   fmt.Sprintf("role-%d-%d", id, j),
					Name: fmt.Sprintf("Role %d-%d", id, j),
				}
			}
			p.AddRoles(roles...)
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond) // Let async operations settle

	// Verify no race conditions occurred
	p.rbac.mu.RLock()
	assert.NotNil(t, p.rbac.roles)
	assert.NotNil(t, p.rbac.rolesMap)
	// Should have at least the initial role
	assert.GreaterOrEqual(t, len(p.rbac.roles), 1)
	p.rbac.mu.RUnlock()
}

func TestBaseProvider_GetRole_Concurrency(t *testing.T) {
	p := newTestProviderWithRoles()

	// Set up test data
	roles := make([]ProviderRole, 100)
	for i := 0; i < 100; i++ {
		roles[i] = ProviderRole{
			ID:   fmt.Sprintf("role-%d", i),
			Name: fmt.Sprintf("Role %d", i),
		}
	}
	p.SetRoles(roles)
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	goroutines := 100
	ctx := context.Background()

	// Concurrently read roles from multiple goroutines
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Try to get role multiple times
			for j := 0; j < 10; j++ {
				roleID := fmt.Sprintf("role-%d", id%100)
				role, err := p.GetRole(ctx, roleID)
				assert.NoError(t, err)
				if role != nil {
					assert.Equal(t, roleID, role.ID)
				}
			}
		}(i)
	}

	wg.Wait()
	// If no panic occurred, the test passes
}

func TestBaseProvider_MixedRoleOperations_Concurrency(t *testing.T) {
	p := newTestProviderWithRoles()

	// Set initial state
	initialRoles := []ProviderRole{
		{ID: "initial-1", Name: "Initial One"},
		{ID: "initial-2", Name: "Initial Two"},
	}
	p.SetRoles(initialRoles)
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	ctx := context.Background()
	iterations := 100

	// Writers: SetRoles
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				roles := []ProviderRole{
					{ID: fmt.Sprintf("set-%d-%d", id, j), Name: fmt.Sprintf("Set %d %d", id, j)},
				}
				p.SetRoles(roles)
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// Writers: AddRoles
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				role := ProviderRole{
					ID:   fmt.Sprintf("add-%d-%d", id, j),
					Name: fmt.Sprintf("Add %d %d", id, j),
				}
				p.AddRoles(role)
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// Readers: GetRole
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Try to get initial roles
				_, _ = p.GetRole(ctx, "initial-1")
				_, _ = p.GetRole(ctx, "initial-2")
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// Readers: ListRoles
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, _ = p.ListRoles(ctx, nil)
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond) // Let async operations settle

	// Verify data structures are still valid
	p.rbac.mu.RLock()
	assert.NotNil(t, p.rbac.roles)
	assert.NotNil(t, p.rbac.rolesMap)
	p.rbac.mu.RUnlock()
}

func TestBaseProvider_RoleRWLock_Behavior(t *testing.T) {
	p := newTestProviderWithRoles()

	roles := []ProviderRole{
		{ID: "role-1", Name: "Role One"},
		{ID: "role-2", Name: "Role Two"},
	}
	p.SetRoles(roles)
	time.Sleep(10 * time.Millisecond)

	ctx := context.Background()
	var wg sync.WaitGroup

	// Start many readers
	readers := 100
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = p.GetRole(ctx, "role-1")
				_, _ = p.ListRoles(ctx, nil)
			}
		}()
	}

	// Start a writer in the middle
	time.Sleep(10 * time.Millisecond)
	wg.Add(1)
	go func() {
		defer wg.Done()
		newRoles := []ProviderRole{
			{ID: "role-3", Name: "Role Three"},
		}
		p.SetRoles(newRoles)
	}()

	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	// Verify the writer succeeded
	p.rbac.mu.RLock()
	found := false
	for _, r := range p.rbac.roles {
		if r.ID == "role-3" {
			found = true
			break
		}
	}
	p.rbac.mu.RUnlock()
	assert.True(t, found, "Writer should have succeeded even with concurrent readers")
}

func TestBaseProvider_RoleDataRaceDetection(t *testing.T) {
	// This test is designed to catch data races when run with -race flag
	p := newTestProviderWithRoles()

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 1000; i++ {
			p.SetRoles([]ProviderRole{
				{ID: fmt.Sprintf("role-%d", i), Name: fmt.Sprintf("Role %d", i)},
			})
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		ctx := context.Background()
		for i := 0; i < 1000; i++ {
			_, _ = p.ListRoles(ctx, nil)
			_, _ = p.GetRole(ctx, "role-0")
		}
		done <- true
	}()

	// Add goroutine
	go func() {
		for i := 0; i < 1000; i++ {
			p.AddRoles(ProviderRole{
				ID:   fmt.Sprintf("add-role-%d", i),
				Name: fmt.Sprintf("Add Role %d", i),
			})
		}
		done <- true
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done

	time.Sleep(100 * time.Millisecond)

	// Final verification
	p.rbac.mu.RLock()
	assert.NotNil(t, p.rbac.roles)
	assert.NotNil(t, p.rbac.rolesMap)
	p.rbac.mu.RUnlock()
}

func TestCreateKeysFromRoles(t *testing.T) {
	role := ProviderRole{
		ID:   "role-123",
		Name: "Administrator",
	}

	keys := CreateKeysFromRoles(role)
	assert.Len(t, keys, 2)
	assert.Contains(t, keys, "role-123")
	assert.Contains(t, keys, "Administrator")
}
