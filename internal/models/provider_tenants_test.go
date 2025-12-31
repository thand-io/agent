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

// Helper function to create a BaseProvider with tenant support
func newTestProviderWithTenants() *BaseProvider {
	provider := Provider{
		Name:        "test-provider",
		Description: "Test Provider",
		Provider:    "test",
	}

	return NewBaseProvider("test-provider-id", provider,
		NewProviderCapabilities().WithDefaultTenantsConfiguration())
}

func TestBaseProvider_SetTenants(t *testing.T) {
	p := newTestProviderWithTenants()

	tenants := []ProviderTenant{
		{ID: "tenant1", Name: "Tenant One", Type: "account"},
		{ID: "tenant2", Name: "Tenant Two", Type: "account"},
		{ID: "tenant3", Name: "Tenant Three", Type: "folder", Parent: "tenant1"},
	}

	// Test SetTenants
	p.SetTenants(tenants)

	// Give time for async index building to start (though we're not testing that here)
	time.Sleep(10 * time.Millisecond)

	// Verify tenants are set
	p.tenants.mu.RLock()
	assert.Len(t, p.tenants.tenants, 3)
	assert.Len(t, p.tenants.tenantsMap, 6) // Each tenant has 2 keys (ID and Name)
	p.tenants.mu.RUnlock()

	// Verify map contains all expected keys (lowercase)
	p.tenants.mu.RLock()
	assert.Contains(t, p.tenants.tenantsMap, "tenant1")
	assert.Contains(t, p.tenants.tenantsMap, "tenant one")
	assert.Contains(t, p.tenants.tenantsMap, "tenant2")
	assert.Contains(t, p.tenants.tenantsMap, "tenant two")
	assert.Contains(t, p.tenants.tenantsMap, "tenant3")
	assert.Contains(t, p.tenants.tenantsMap, "tenant three")
	p.tenants.mu.RUnlock()

	// Verify map points to correct tenants
	p.tenants.mu.RLock()
	assert.Equal(t, "tenant1", p.tenants.tenantsMap["tenant1"].ID)
	assert.Equal(t, "Tenant One", p.tenants.tenantsMap["tenant one"].Name)
	p.tenants.mu.RUnlock()
}

func TestBaseProvider_SetTenants_ReplacesExisting(t *testing.T) {
	p := newTestProviderWithTenants()

	// Set initial tenants
	initialTenants := []ProviderTenant{
		{ID: "tenant1", Name: "Tenant One"},
		{ID: "tenant2", Name: "Tenant Two"},
	}
	p.SetTenants(initialTenants)

	// Replace with new tenants
	newTenants := []ProviderTenant{
		{ID: "tenant3", Name: "Tenant Three"},
	}
	p.SetTenants(newTenants)

	// Verify old tenants are replaced
	p.tenants.mu.RLock()
	assert.Len(t, p.tenants.tenants, 1)
	assert.Equal(t, "tenant3", p.tenants.tenants[0].ID)
	assert.NotContains(t, p.tenants.tenantsMap, "tenant1")
	assert.NotContains(t, p.tenants.tenantsMap, "tenant2")
	assert.Contains(t, p.tenants.tenantsMap, "tenant3")
	p.tenants.mu.RUnlock()
}

func TestBaseProvider_AddTenants(t *testing.T) {
	p := newTestProviderWithTenants()

	// Set initial tenants
	initialTenants := []ProviderTenant{
		{ID: "tenant1", Name: "Tenant One"},
	}
	p.SetTenants(initialTenants)
	time.Sleep(10 * time.Millisecond)

	// Add new tenants
	p.AddTenants(
		ProviderTenant{ID: "tenant2", Name: "Tenant Two"},
		ProviderTenant{ID: "tenant3", Name: "Tenant Three"},
	)
	time.Sleep(10 * time.Millisecond)

	// Verify all tenants are present
	p.tenants.mu.RLock()
	assert.Len(t, p.tenants.tenants, 3)
	assert.Contains(t, p.tenants.tenantsMap, "tenant1")
	assert.Contains(t, p.tenants.tenantsMap, "tenant2")
	assert.Contains(t, p.tenants.tenantsMap, "tenant3")
	p.tenants.mu.RUnlock()
}

func TestBaseProvider_AddTenants_FiltersDuplicates(t *testing.T) {
	p := newTestProviderWithTenants()

	// Set initial tenants
	initialTenants := []ProviderTenant{
		{ID: "tenant1", Name: "Tenant One"},
		{ID: "tenant2", Name: "Tenant Two"},
	}
	p.SetTenants(initialTenants)
	time.Sleep(10 * time.Millisecond)

	// Try to add duplicate and new tenant
	p.AddTenants(
		ProviderTenant{ID: "tenant1", Name: "Tenant One"},  // duplicate by ID
		ProviderTenant{ID: "tenant3", Name: "Tenant Two"},  // duplicate by Name
		ProviderTenant{ID: "tenant4", Name: "Tenant Four"}, // new
	)
	time.Sleep(10 * time.Millisecond)

	// Verify duplicates are filtered
	p.tenants.mu.RLock()
	assert.Len(t, p.tenants.tenants, 3, "Should have 3 tenants (2 initial + 1 new)")
	assert.Contains(t, p.tenants.tenantsMap, "tenant1")
	assert.Contains(t, p.tenants.tenantsMap, "tenant2")
	assert.Contains(t, p.tenants.tenantsMap, "tenant4")
	p.tenants.mu.RUnlock()
}

func TestBaseProvider_GetTenant(t *testing.T) {
	p := newTestProviderWithTenants()

	tenants := []ProviderTenant{
		{ID: "tenant-1", Name: "Tenant One"},
		{ID: "tenant-2", Name: "Tenant Two"},
	}
	p.SetTenants(tenants)

	ctx := context.Background()

	// Test getting tenant by ID
	tenant, err := p.GetTenant(ctx, "tenant-1")
	require.NoError(t, err)
	require.NotNil(t, tenant)
	assert.Equal(t, "tenant-1", tenant.ID)
	assert.Equal(t, "Tenant One", tenant.Name)

	// Test getting tenant by Name (case-insensitive)
	tenant, err = p.GetTenant(ctx, "tenant one")
	require.NoError(t, err)
	require.NotNil(t, tenant)
	assert.Equal(t, "tenant-1", tenant.ID)

	// Test getting tenant by Name with different case
	tenant, err = p.GetTenant(ctx, "TENANT TWO")
	require.NoError(t, err)
	require.NotNil(t, tenant)
	assert.Equal(t, "tenant-2", tenant.ID)

	// Test getting non-existent tenant
	tenant, err = p.GetTenant(ctx, "non-existent")
	assert.Error(t, err)
	assert.Nil(t, tenant)
	assert.Contains(t, err.Error(), "tenant not found")
}

func TestBaseProvider_GetTenant_WithoutCapability(t *testing.T) {
	// Create provider without tenant capability
	p := NewBaseProvider("test", Provider{Name: "test"}, NewProviderCapabilities())

	ctx := context.Background()
	tenant, err := p.GetTenant(ctx, "tenant1")
	assert.Error(t, err)
	assert.Nil(t, tenant)
	assert.Contains(t, err.Error(), "no tenants support")
}

func TestBaseProvider_ListTenants(t *testing.T) {
	p := newTestProviderWithTenants()

	tenants := []ProviderTenant{
		{ID: "aws-123", Name: "Production Account"},
		{ID: "aws-456", Name: "Development Account"},
		{ID: "aws-789", Name: "Staging Account"},
	}
	p.SetTenants(tenants)

	ctx := context.Background()

	// Test listing all tenants (no filter)
	results, err := p.ListTenants(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Test listing with empty search request
	results, err = p.ListTenants(ctx, &SearchRequest{})
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Test listing with filter
	results, err = p.ListTenants(ctx, &SearchRequest{
		Terms: []string{"production"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "aws-123", results[0].Result.ID)

	// Test listing with case-insensitive filter
	results, err = p.ListTenants(ctx, &SearchRequest{
		Terms: []string{"DEVELOPMENT"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "aws-456", results[0].Result.ID)

	// Test listing with partial match
	results, err = p.ListTenants(ctx, &SearchRequest{
		Terms: []string{"account"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

func TestBaseProvider_ListTenants_WithoutCapability(t *testing.T) {
	p := NewBaseProvider("test", Provider{Name: "test"}, NewProviderCapabilities())

	ctx := context.Background()
	results, err := p.ListTenants(ctx, nil)
	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "no tenants support")
}

func TestBaseProvider_ListTenants_Search(t *testing.T) {
	p := NewBaseProvider("test", Provider{
		Name: "Test Provider",
	}, NewProviderCapabilities().WithDefaultTenantsConfiguration())

	tenants := []ProviderTenant{
		{
			ID:   "aws-123",
			Name: "Production Account",
			Type: "account",
		},
		{
			ID:   "aws-456",
			Name: "Development Account",
			Type: "account",
		},
	}

	p.SetTenants(tenants)

	// Wait for index to be built
	time.Sleep(500 * time.Millisecond)

	ctx := context.Background()
	searchReq := &SearchRequest{
		Query: "Production*",
		Terms: []string{"Production"},
	}

	results, err := p.ListTenants(ctx, searchReq)
	require.NoError(t, err)
	assert.NotEmpty(t, results, "Should find tenant by name")

	if len(results) > 0 {
		assert.Equal(t, "aws-123", results[0].Result.ID)
	}
}

// CONCURRENCY TESTS

func TestBaseProvider_SetTenants_Concurrency(t *testing.T) {
	p := newTestProviderWithTenants()

	var wg sync.WaitGroup
	goroutines := 100
	tenantsPerGoroutine := 10

	// Concurrently set tenants from multiple goroutines
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			tenants := make([]ProviderTenant, tenantsPerGoroutine)
			for j := 0; j < tenantsPerGoroutine; j++ {
				tenants[j] = ProviderTenant{
					ID:   fmt.Sprintf("tenant-%d-%d", id, j),
					Name: fmt.Sprintf("Tenant %d-%d", id, j),
				}
			}
			p.SetTenants(tenants)
		}(i)
	}

	wg.Wait()
	time.Sleep(50 * time.Millisecond) // Let async operations settle

	// Verify no race conditions occurred and data is consistent
	p.tenants.mu.RLock()
	assert.NotNil(t, p.tenants.tenants)
	assert.NotNil(t, p.tenants.tenantsMap)
	tenantCount := len(p.tenants.tenants)
	mapCount := len(p.tenants.tenantsMap)
	p.tenants.mu.RUnlock()

	// The last SetTenants wins, should have tenantsPerGoroutine items
	assert.Equal(t, tenantsPerGoroutine, tenantCount)
	// Each tenant has 2 keys (ID and Name)
	assert.Equal(t, tenantsPerGoroutine*2, mapCount)
}

func TestBaseProvider_AddTenants_Concurrency(t *testing.T) {
	p := newTestProviderWithTenants()

	// Set initial state
	p.SetTenants([]ProviderTenant{
		{ID: "initial-1", Name: "Initial One"},
	})
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	goroutines := 50
	tenantsPerGoroutine := 2

	// Concurrently add tenants from multiple goroutines
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			tenants := make([]ProviderTenant, tenantsPerGoroutine)
			for j := 0; j < tenantsPerGoroutine; j++ {
				tenants[j] = ProviderTenant{
					ID:   fmt.Sprintf("tenant-%d-%d", id, j),
					Name: fmt.Sprintf("Tenant %d-%d", id, j),
				}
			}
			p.AddTenants(tenants...)
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond) // Let async operations settle

	// Verify no race conditions occurred
	p.tenants.mu.RLock()
	assert.NotNil(t, p.tenants.tenants)
	assert.NotNil(t, p.tenants.tenantsMap)
	// Should have at least the initial tenant
	assert.GreaterOrEqual(t, len(p.tenants.tenants), 1)
	p.tenants.mu.RUnlock()
}

func TestBaseProvider_GetTenant_Concurrency(t *testing.T) {
	p := newTestProviderWithTenants()

	// Set up test data
	tenants := make([]ProviderTenant, 100)
	for i := 0; i < 100; i++ {
		tenants[i] = ProviderTenant{
			ID:   fmt.Sprintf("tenant-%d", i),
			Name: fmt.Sprintf("Tenant %d", i),
		}
	}
	p.SetTenants(tenants)
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	goroutines := 100
	ctx := context.Background()

	// Concurrently read tenants from multiple goroutines
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Try to get tenant multiple times
			for j := 0; j < 10; j++ {
				tenantID := fmt.Sprintf("tenant-%d", id%100)
				tenant, err := p.GetTenant(ctx, tenantID)
				assert.NoError(t, err)
				if tenant != nil {
					assert.Equal(t, tenantID, tenant.ID)
				}
			}
		}(i)
	}

	wg.Wait()
	// If no panic occurred, the test passes
}

func TestBaseProvider_MixedTenantOperations_Concurrency(t *testing.T) {
	p := newTestProviderWithTenants()

	// Set initial state
	initialTenants := []ProviderTenant{
		{ID: "initial-1", Name: "Initial One"},
		{ID: "initial-2", Name: "Initial Two"},
	}
	p.SetTenants(initialTenants)
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	ctx := context.Background()
	iterations := 100

	// Writers: SetTenants
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				tenants := []ProviderTenant{
					{ID: fmt.Sprintf("set-%d-%d", id, j), Name: fmt.Sprintf("Set %d %d", id, j)},
				}
				p.SetTenants(tenants)
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// Writers: AddTenants
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				tenant := ProviderTenant{
					ID:   fmt.Sprintf("add-%d-%d", id, j),
					Name: fmt.Sprintf("Add %d %d", id, j),
				}
				p.AddTenants(tenant)
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// Readers: GetTenant
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Try to get initial tenants
				_, _ = p.GetTenant(ctx, "initial-1")
				_, _ = p.GetTenant(ctx, "initial-2")
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// Readers: ListTenants
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, _ = p.ListTenants(ctx, nil)
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond) // Let async operations settle

	// Verify data structures are still valid
	p.tenants.mu.RLock()
	assert.NotNil(t, p.tenants.tenants)
	assert.NotNil(t, p.tenants.tenantsMap)
	p.tenants.mu.RUnlock()
}

func TestBaseProvider_TenantRWLock_Behavior(t *testing.T) {
	p := newTestProviderWithTenants()

	tenants := []ProviderTenant{
		{ID: "tenant-1", Name: "Tenant One"},
		{ID: "tenant-2", Name: "Tenant Two"},
	}
	p.SetTenants(tenants)
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
				_, _ = p.GetTenant(ctx, "tenant-1")
				_, _ = p.ListTenants(ctx, nil)
			}
		}()
	}

	// Start a writer in the middle
	time.Sleep(10 * time.Millisecond)
	wg.Add(1)
	go func() {
		defer wg.Done()
		newTenants := []ProviderTenant{
			{ID: "tenant-3", Name: "Tenant Three"},
		}
		p.SetTenants(newTenants)
	}()

	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	// Verify the writer succeeded
	p.tenants.mu.RLock()
	found := false
	for _, t := range p.tenants.tenants {
		if t.ID == "tenant-3" {
			found = true
			break
		}
	}
	p.tenants.mu.RUnlock()
	assert.True(t, found, "Writer should have succeeded even with concurrent readers")
}

func TestBaseProvider_TenantDataRaceDetection(t *testing.T) {
	// This test is designed to catch data races when run with -race flag
	p := newTestProviderWithTenants()

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 1000; i++ {
			p.SetTenants([]ProviderTenant{
				{ID: fmt.Sprintf("tenant-%d", i), Name: fmt.Sprintf("Tenant %d", i)},
			})
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		ctx := context.Background()
		for i := 0; i < 1000; i++ {
			_, _ = p.ListTenants(ctx, nil)
			_, _ = p.GetTenant(ctx, "tenant-0")
		}
		done <- true
	}()

	// Add goroutine
	go func() {
		for i := 0; i < 1000; i++ {
			p.AddTenants(ProviderTenant{
				ID:   fmt.Sprintf("add-tenant-%d", i),
				Name: fmt.Sprintf("Add Tenant %d", i),
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
	p.tenants.mu.RLock()
	assert.NotNil(t, p.tenants.tenants)
	assert.NotNil(t, p.tenants.tenantsMap)
	p.tenants.mu.RUnlock()
}

func TestProviderTenant_String(t *testing.T) {
	tenant := ProviderTenant{
		ID:   "tenant-123",
		Name: "My Tenant",
		Type: "account",
	}

	expected := "My Tenant (tenant-123)"
	assert.Equal(t, expected, tenant.String())
}

func TestCreateKeysFromTenants(t *testing.T) {
	tenant := ProviderTenant{
		ID:   "tenant-123",
		Name: "My Tenant",
	}

	keys := CreateKeysFromTenants(tenant)
	assert.Len(t, keys, 2)
	assert.Contains(t, keys, "tenant-123")
	assert.Contains(t, keys, "My Tenant")
}
