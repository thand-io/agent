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

// Helper function to create a BaseProvider with resource support
func newTestProviderWithResources() *BaseProvider {
	provider := Provider{
		Name:        "test-provider",
		Description: "Test Provider",
		Provider:    "test",
	}

	return NewBaseProvider("test-provider-id", provider,
		NewProviderCapabilities().WithDefaultResourcesConfiguration())
}

func TestBaseProvider_SetResources(t *testing.T) {
	p := newTestProviderWithResources()

	resources := []ProviderResource{
		{ID: "res1", Name: "Database-Prod", Type: "database", Description: "Production database"},
		{ID: "res2", Name: "Storage-Dev", Type: "storage", Description: "Development storage"},
		{ID: "res3", Name: "Compute-Staging", Type: "compute", Description: "Staging compute"},
	}

	// Test SetResources
	p.SetResources(resources)

	// Give time for async index building to start
	time.Sleep(10 * time.Millisecond)

	// Verify resources are set
	p.rbac.mu.RLock()
	assert.Len(t, p.rbac.resources, 3)
	assert.Len(t, p.rbac.resourcesMap, 6) // Each resource has 2 keys (ID and Name)
	p.rbac.mu.RUnlock()

	// Verify map contains all expected keys (lowercase)
	p.rbac.mu.RLock()
	assert.Contains(t, p.rbac.resourcesMap, "res1")
	assert.Contains(t, p.rbac.resourcesMap, "database-prod")
	assert.Contains(t, p.rbac.resourcesMap, "res2")
	assert.Contains(t, p.rbac.resourcesMap, "storage-dev")
	assert.Contains(t, p.rbac.resourcesMap, "res3")
	assert.Contains(t, p.rbac.resourcesMap, "compute-staging")
	p.rbac.mu.RUnlock()

	// Verify map points to correct resources
	p.rbac.mu.RLock()
	assert.Equal(t, "res1", p.rbac.resourcesMap["res1"].ID)
	assert.Equal(t, "Database-Prod", p.rbac.resourcesMap["database-prod"].Name)
	assert.Equal(t, "Storage-Dev", p.rbac.resourcesMap["storage-dev"].Name)
	p.rbac.mu.RUnlock()
}

func TestBaseProvider_SetResources_ReplacesExisting(t *testing.T) {
	p := newTestProviderWithResources()

	// Set initial resources
	initialResources := []ProviderResource{
		{ID: "res1", Name: "Resource One"},
		{ID: "res2", Name: "Resource Two"},
	}
	p.SetResources(initialResources)

	// Replace with new resources
	newResources := []ProviderResource{
		{ID: "res3", Name: "Resource Three"},
	}
	p.SetResources(newResources)

	// Verify old resources are replaced
	p.rbac.mu.RLock()
	assert.Len(t, p.rbac.resources, 1)
	assert.Equal(t, "res3", p.rbac.resources[0].ID)
	assert.NotContains(t, p.rbac.resourcesMap, "res1")
	assert.NotContains(t, p.rbac.resourcesMap, "res2")
	assert.Contains(t, p.rbac.resourcesMap, "res3")
	p.rbac.mu.RUnlock()
}

func TestBaseProvider_AddResources(t *testing.T) {
	p := newTestProviderWithResources()

	// Set initial resources
	initialResources := []ProviderResource{
		{ID: "res1", Name: "Resource One"},
	}
	p.SetResources(initialResources)
	time.Sleep(10 * time.Millisecond)

	// Add new resources
	p.AddResources(
		ProviderResource{ID: "res2", Name: "Resource Two"},
		ProviderResource{ID: "res3", Name: "Resource Three"},
	)
	time.Sleep(10 * time.Millisecond)

	// Verify all resources are present
	p.rbac.mu.RLock()
	assert.Len(t, p.rbac.resources, 3)
	assert.Contains(t, p.rbac.resourcesMap, "res1")
	assert.Contains(t, p.rbac.resourcesMap, "res2")
	assert.Contains(t, p.rbac.resourcesMap, "res3")
	p.rbac.mu.RUnlock()
}

func TestBaseProvider_AddResources_FiltersDuplicates(t *testing.T) {
	p := newTestProviderWithResources()

	// Set initial resources
	initialResources := []ProviderResource{
		{ID: "res1", Name: "Resource One"},
		{ID: "res2", Name: "Resource Two"},
	}
	p.SetResources(initialResources)
	time.Sleep(10 * time.Millisecond)

	// Try to add duplicate and new resource
	p.AddResources(
		ProviderResource{ID: "res1", Name: "Resource One"},  // duplicate by ID
		ProviderResource{ID: "res3", Name: "Resource Two"},  // duplicate by Name
		ProviderResource{ID: "res4", Name: "Resource Four"}, // new
	)
	time.Sleep(10 * time.Millisecond)

	// Verify duplicates are filtered
	p.rbac.mu.RLock()
	assert.Len(t, p.rbac.resources, 3, "Should have 3 resources (2 initial + 1 new)")
	assert.Contains(t, p.rbac.resourcesMap, "res1")
	assert.Contains(t, p.rbac.resourcesMap, "res2")
	assert.Contains(t, p.rbac.resourcesMap, "res4")
	p.rbac.mu.RUnlock()
}

func TestBaseProvider_GetResource(t *testing.T) {
	p := newTestProviderWithResources()

	resources := []ProviderResource{
		{
			ID:          "res-1",
			Name:        "Production-Database",
			Type:        "database",
			Description: "Main production database",
		},
		{
			ID:          "res-2",
			Name:        "Development-Storage",
			Type:        "storage",
			Description: "Development storage bucket",
		},
	}
	p.SetResources(resources)

	ctx := context.Background()

	// Test getting resource by ID
	resource, err := p.GetResource(ctx, "res-1")
	require.NoError(t, err)
	require.NotNil(t, resource)
	assert.Equal(t, "res-1", resource.ID)
	assert.Equal(t, "Production-Database", resource.Name)

	// Test getting resource by Name (case-insensitive)
	resource, err = p.GetResource(ctx, "production-database")
	require.NoError(t, err)
	require.NotNil(t, resource)
	assert.Equal(t, "res-1", resource.ID)

	// Test getting resource by Name with different case
	resource, err = p.GetResource(ctx, "DEVELOPMENT-STORAGE")
	require.NoError(t, err)
	require.NotNil(t, resource)
	assert.Equal(t, "res-2", resource.ID)

	// Test getting non-existent resource
	resource, err = p.GetResource(ctx, "non-existent")
	assert.Error(t, err)
	assert.Nil(t, resource)
	assert.Contains(t, err.Error(), "resource not found")
}

func TestBaseProvider_GetResource_WithoutCapability(t *testing.T) {
	// Create provider without resource capability
	p := NewBaseProvider("test", Provider{Name: "test"}, NewProviderCapabilities())

	ctx := context.Background()
	resource, err := p.GetResource(ctx, "res1")
	assert.Error(t, err)
	assert.Nil(t, resource)
	assert.Contains(t, err.Error(), "no resources")
}

func TestBaseProvider_ListResources(t *testing.T) {
	p := newTestProviderWithResources()

	resources := []ProviderResource{
		{ID: "db-prod", Name: "Production Database"},
		{ID: "storage-dev", Name: "Development Storage"},
		{ID: "compute-staging", Name: "Staging Compute"},
	}
	p.SetResources(resources)

	ctx := context.Background()

	// Test listing all resources (no filter)
	results, err := p.ListResources(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Test listing with empty search request
	results, err = p.ListResources(ctx, &SearchRequest{})
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Test listing with filter
	results, err = p.ListResources(ctx, &SearchRequest{
		Terms: []string{"production"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "db-prod", results[0].Result.ID)

	// Test listing with case-insensitive filter
	results, err = p.ListResources(ctx, &SearchRequest{
		Terms: []string{"DEVELOPMENT"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "storage-dev", results[0].Result.ID)

	// Test partial match
	results, err = p.ListResources(ctx, &SearchRequest{
		Terms: []string{"storage"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestBaseProvider_ListResources_WithoutCapability(t *testing.T) {
	p := NewBaseProvider("test", Provider{Name: "test"}, NewProviderCapabilities())

	ctx := context.Background()
	results, err := p.ListResources(ctx, nil)
	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "no resources")
}

func TestBaseProvider_ListResources_Search(t *testing.T) {
	p := NewBaseProvider("test", Provider{
		Name: "Test Provider",
	}, NewProviderCapabilities().WithDefaultResourcesConfiguration())

	resourceName := "ProductionDatabase"
	resources := []ProviderResource{
		{
			ID:   "db-prod",
			Name: resourceName,
			Type: "database",
		},
		{
			ID:   "storage-dev",
			Name: "DevelopmentStorage",
			Type: "storage",
		},
	}

	p.SetResources(resources)

	// Wait for index to be built
	time.Sleep(500 * time.Millisecond)

	ctx := context.Background()
	searchReq := &SearchRequest{
		Terms: []string{resourceName},
	}

	results, err := p.ListResources(ctx, searchReq)
	require.NoError(t, err)
	assert.NotEmpty(t, results, "Should find resource by name")

	if len(results) > 0 {
		assert.Equal(t, "db-prod", results[0].Result.ID)
	}
}

// CONCURRENCY TESTS

func TestBaseProvider_SetResources_Concurrency(t *testing.T) {
	p := newTestProviderWithResources()

	var wg sync.WaitGroup
	goroutines := 100
	resourcesPerGoroutine := 10

	// Concurrently set resources from multiple goroutines
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			resources := make([]ProviderResource, resourcesPerGoroutine)
			for j := 0; j < resourcesPerGoroutine; j++ {
				resources[j] = ProviderResource{
					ID:   fmt.Sprintf("res-%d-%d", id, j),
					Name: fmt.Sprintf("Resource %d-%d", id, j),
				}
			}
			p.SetResources(resources)
		}(i)
	}

	wg.Wait()
	time.Sleep(50 * time.Millisecond) // Let async operations settle

	// Verify no race conditions occurred and data is consistent
	p.rbac.mu.RLock()
	assert.NotNil(t, p.rbac.resources)
	assert.NotNil(t, p.rbac.resourcesMap)
	resourceCount := len(p.rbac.resources)
	mapCount := len(p.rbac.resourcesMap)
	p.rbac.mu.RUnlock()

	// The last SetResources wins, should have resourcesPerGoroutine items
	assert.Equal(t, resourcesPerGoroutine, resourceCount)
	// Each resource has 2 keys (ID and Name)
	assert.Equal(t, resourcesPerGoroutine*2, mapCount)
}

func TestBaseProvider_AddResources_Concurrency(t *testing.T) {
	p := newTestProviderWithResources()

	// Set initial state
	p.SetResources([]ProviderResource{
		{ID: "initial-1", Name: "Initial One"},
	})
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	goroutines := 50
	resourcesPerGoroutine := 2

	// Concurrently add resources from multiple goroutines
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			resources := make([]ProviderResource, resourcesPerGoroutine)
			for j := 0; j < resourcesPerGoroutine; j++ {
				resources[j] = ProviderResource{
					ID:   fmt.Sprintf("res-%d-%d", id, j),
					Name: fmt.Sprintf("Resource %d-%d", id, j),
				}
			}
			p.AddResources(resources...)
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond) // Let async operations settle

	// Verify no race conditions occurred
	p.rbac.mu.RLock()
	assert.NotNil(t, p.rbac.resources)
	assert.NotNil(t, p.rbac.resourcesMap)
	// Should have at least the initial resource
	assert.GreaterOrEqual(t, len(p.rbac.resources), 1)
	p.rbac.mu.RUnlock()
}

func TestBaseProvider_GetResource_Concurrency(t *testing.T) {
	p := newTestProviderWithResources()

	// Set up test data
	resources := make([]ProviderResource, 100)
	for i := 0; i < 100; i++ {
		resources[i] = ProviderResource{
			ID:   fmt.Sprintf("res-%d", i),
			Name: fmt.Sprintf("Resource %d", i),
		}
	}
	p.SetResources(resources)
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	goroutines := 100
	ctx := context.Background()

	// Concurrently read resources from multiple goroutines
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Try to get resource multiple times
			for j := 0; j < 10; j++ {
				resourceID := fmt.Sprintf("res-%d", id%100)
				resource, err := p.GetResource(ctx, resourceID)
				assert.NoError(t, err)
				if resource != nil {
					assert.Equal(t, resourceID, resource.ID)
				}
			}
		}(i)
	}

	wg.Wait()
	// If no panic occurred, the test passes
}

func TestBaseProvider_MixedResourceOperations_Concurrency(t *testing.T) {
	p := newTestProviderWithResources()

	// Set initial state
	initialResources := []ProviderResource{
		{ID: "initial-1", Name: "Initial One"},
		{ID: "initial-2", Name: "Initial Two"},
	}
	p.SetResources(initialResources)
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	ctx := context.Background()
	iterations := 100

	// Writers: SetResources
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				resources := []ProviderResource{
					{ID: fmt.Sprintf("set-%d-%d", id, j), Name: fmt.Sprintf("Set %d %d", id, j)},
				}
				p.SetResources(resources)
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// Writers: AddResources
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				resource := ProviderResource{
					ID:   fmt.Sprintf("add-%d-%d", id, j),
					Name: fmt.Sprintf("Add %d %d", id, j),
				}
				p.AddResources(resource)
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// Readers: GetResource
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Try to get initial resources
				_, _ = p.GetResource(ctx, "initial-1")
				_, _ = p.GetResource(ctx, "initial-2")
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// Readers: ListResources
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, _ = p.ListResources(ctx, nil)
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond) // Let async operations settle

	// Verify data structures are still valid
	p.rbac.mu.RLock()
	assert.NotNil(t, p.rbac.resources)
	assert.NotNil(t, p.rbac.resourcesMap)
	p.rbac.mu.RUnlock()
}

func TestBaseProvider_ResourceRWLock_Behavior(t *testing.T) {
	p := newTestProviderWithResources()

	resources := []ProviderResource{
		{ID: "res-1", Name: "Resource One"},
		{ID: "res-2", Name: "Resource Two"},
	}
	p.SetResources(resources)
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
				_, _ = p.GetResource(ctx, "res-1")
				_, _ = p.ListResources(ctx, nil)
			}
		}()
	}

	// Start a writer in the middle
	time.Sleep(10 * time.Millisecond)
	wg.Add(1)
	go func() {
		defer wg.Done()
		newResources := []ProviderResource{
			{ID: "res-3", Name: "Resource Three"},
		}
		p.SetResources(newResources)
	}()

	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	// Verify the writer succeeded
	p.rbac.mu.RLock()
	found := false
	for _, r := range p.rbac.resources {
		if r.ID == "res-3" {
			found = true
			break
		}
	}
	p.rbac.mu.RUnlock()
	assert.True(t, found, "Writer should have succeeded even with concurrent readers")
}

func TestBaseProvider_ResourceDataRaceDetection(t *testing.T) {
	// This test is designed to catch data races when run with -race flag
	p := newTestProviderWithResources()

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 1000; i++ {
			p.SetResources([]ProviderResource{
				{ID: fmt.Sprintf("res-%d", i), Name: fmt.Sprintf("Resource %d", i)},
			})
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		ctx := context.Background()
		for i := 0; i < 1000; i++ {
			_, _ = p.ListResources(ctx, nil)
			_, _ = p.GetResource(ctx, "res-0")
		}
		done <- true
	}()

	// Add goroutine
	go func() {
		for i := 0; i < 1000; i++ {
			p.AddResources(ProviderResource{
				ID:   fmt.Sprintf("add-res-%d", i),
				Name: fmt.Sprintf("Add Resource %d", i),
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
	assert.NotNil(t, p.rbac.resources)
	assert.NotNil(t, p.rbac.resourcesMap)
	p.rbac.mu.RUnlock()
}

func TestCreateKeysFromResources(t *testing.T) {
	resource := ProviderResource{
		ID:   "res-123",
		Name: "Production-Database",
	}

	keys := CreateKeysFromResources(resource)
	assert.Len(t, keys, 2)
	assert.Contains(t, keys, "res-123")
	assert.Contains(t, keys, "Production-Database")
}
