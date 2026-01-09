package models_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// Helper function to create a BaseProvider with resource support
func newTestProviderWithResources() *models.BaseProvider {
	provider := models.ProviderConfig{
		Name:        "test-provider",
		Description: "Test Provider",
		Provider:    "test",
	}

	return models.NewBaseProvider("test-provider-id", provider,
		models.NewProviderCapabilities().WithDefaultResourcesConfiguration())
}

func TestBaseProvider_SetResources(t *testing.T) {
	p := newTestProviderWithResources()

	resources := []models.ProviderResource{
		{ID: "res1", Name: "Database-Prod", Type: "database", Description: "Production database"},
		{ID: "res2", Name: "Storage-Dev", Type: "storage", Description: "Development storage"},
		{ID: "res3", Name: "Compute-Staging", Type: "compute", Description: "Staging compute"},
	}

	// Test SetResources
	p.SetResources(resources)

	// Give time for async index building to start
	time.Sleep(10 * time.Millisecond)

	// Verify resources are set
	results, err := p.ListResources(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Verify all resources can be retrieved by ID
	res1, err := p.GetResource(context.Background(), "res1")
	require.NoError(t, err)
	assert.Equal(t, "res1", res1.ID)
	assert.Equal(t, "Database-Prod", res1.Name)

	res2, err := p.GetResource(context.Background(), "res2")
	require.NoError(t, err)
	assert.Equal(t, "res2", res2.ID)

	res3, err := p.GetResource(context.Background(), "res3")
	require.NoError(t, err)
	assert.Equal(t, "res3", res3.ID)

	// Verify resources can be retrieved by name (case-insensitive)
	res1ByName, err := p.GetResource(context.Background(), "database-prod")
	require.NoError(t, err)
	assert.Equal(t, "res1", res1ByName.ID)
	assert.Equal(t, "Database-Prod", res1ByName.Name)

	res2ByName, err := p.GetResource(context.Background(), "storage-dev")
	require.NoError(t, err)
	assert.Equal(t, "Storage-Dev", res2ByName.Name)
}

func TestBaseProvider_SetResources_ReplacesExisting(t *testing.T) {
	p := newTestProviderWithResources()

	// Set initial resources
	initialResources := []models.ProviderResource{
		{ID: "res1", Name: "Resource One"},
		{ID: "res2", Name: "Resource Two"},
	}
	p.SetResources(initialResources)

	// Replace with new resources
	newResources := []models.ProviderResource{
		{ID: "res3", Name: "Resource Three"},
	}
	p.SetResources(newResources)

	// Verify old resources are replaced
	results, err := p.ListResources(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "res3", results[0].Result.ID)

	// Verify old resources are not accessible
	_, err = p.GetResource(context.Background(), "res1")
	assert.Error(t, err)
	_, err = p.GetResource(context.Background(), "res2")
	assert.Error(t, err)

	// Verify new resource is accessible
	res3, err := p.GetResource(context.Background(), "res3")
	require.NoError(t, err)
	assert.Equal(t, "res3", res3.ID)
}

func TestBaseProvider_AddResources(t *testing.T) {
	p := newTestProviderWithResources()

	// Set initial resources
	initialResources := []models.ProviderResource{
		{ID: "res1", Name: "Resource One"},
	}
	p.SetResources(initialResources)
	time.Sleep(10 * time.Millisecond)

	// Add new resources
	p.AddResources(
		models.ProviderResource{ID: "res2", Name: "Resource Two"},
		models.ProviderResource{ID: "res3", Name: "Resource Three"},
	)
	time.Sleep(10 * time.Millisecond)

	// Verify all resources are present
	results, err := p.ListResources(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Verify all resources are accessible
	_, err = p.GetResource(context.Background(), "res1")
	require.NoError(t, err)
	_, err = p.GetResource(context.Background(), "res2")
	require.NoError(t, err)
	_, err = p.GetResource(context.Background(), "res3")
	require.NoError(t, err)
}

func TestBaseProvider_AddResources_FiltersDuplicates(t *testing.T) {
	p := newTestProviderWithResources()

	// Set initial resources
	initialResources := []models.ProviderResource{
		{ID: "res1", Name: "Resource One"},
		{ID: "res2", Name: "Resource Two"},
	}
	p.SetResources(initialResources)
	time.Sleep(10 * time.Millisecond)

	// Try to add duplicate and new resource
	p.AddResources(
		models.ProviderResource{ID: "res1", Name: "Resource One"},  // duplicate by ID
		models.ProviderResource{ID: "res3", Name: "Resource Two"},  // duplicate by Name
		models.ProviderResource{ID: "res4", Name: "Resource Four"}, // new
	)
	time.Sleep(10 * time.Millisecond)

	// Verify duplicates are filtered
	results, err := p.ListResources(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, results, 3, "Should have 3 resources (2 initial + 1 new)")

	// Verify expected resources are accessible
	_, err = p.GetResource(context.Background(), "res1")
	require.NoError(t, err)
	_, err = p.GetResource(context.Background(), "res2")
	require.NoError(t, err)
	_, err = p.GetResource(context.Background(), "res4")
	require.NoError(t, err)
}

func TestBaseProvider_GetResource(t *testing.T) {
	p := newTestProviderWithResources()

	resources := []models.ProviderResource{
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
	p := models.NewBaseProvider("test", models.ProviderConfig{Name: "test"}, models.NewProviderCapabilities())

	ctx := context.Background()
	resource, err := p.GetResource(ctx, "res1")
	assert.Error(t, err)
	assert.Nil(t, resource)
	assert.Contains(t, err.Error(), "no resources")
}

func TestBaseProvider_ListResources(t *testing.T) {
	p := newTestProviderWithResources()

	resources := []models.ProviderResource{
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
	results, err = p.ListResources(ctx, &models.SearchRequest{})
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Test listing with filter
	results, err = p.ListResources(ctx, &models.SearchRequest{
		Terms: []string{"production"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "db-prod", results[0].Result.ID)

	// Test listing with case-insensitive filter
	results, err = p.ListResources(ctx, &models.SearchRequest{
		Terms: []string{"DEVELOPMENT"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "storage-dev", results[0].Result.ID)

	// Test partial match
	results, err = p.ListResources(ctx, &models.SearchRequest{
		Terms: []string{"storage"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestBaseProvider_ListResources_WithoutCapability(t *testing.T) {
	p := models.NewBaseProvider("test", models.ProviderConfig{Name: "test"}, models.NewProviderCapabilities())

	ctx := context.Background()
	results, err := p.ListResources(ctx, nil)
	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "no resources")
}

func TestBaseProvider_ListResources_Search(t *testing.T) {
	p := models.NewBaseProvider("test", models.ProviderConfig{
		Name: "Test Provider",
	}, models.NewProviderCapabilities().WithDefaultResourcesConfiguration())

	resourceName := "ProductionDatabase"
	resources := []models.ProviderResource{
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
	searchReq := &models.SearchRequest{
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

			resources := make([]models.ProviderResource, resourcesPerGoroutine)
			for j := 0; j < resourcesPerGoroutine; j++ {
				resources[j] = models.ProviderResource{
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
	results, err := p.ListResources(context.Background(), nil)
	require.NoError(t, err)
	assert.NotNil(t, results)

	// The last SetResources wins, should have resourcesPerGoroutine items
	assert.Equal(t, resourcesPerGoroutine, len(results))
}

func TestBaseProvider_AddResources_Concurrency(t *testing.T) {
	p := newTestProviderWithResources()

	// Set initial state
	p.SetResources([]models.ProviderResource{
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

			resources := make([]models.ProviderResource, resourcesPerGoroutine)
			for j := 0; j < resourcesPerGoroutine; j++ {
				resources[j] = models.ProviderResource{
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
	results, err := p.ListResources(context.Background(), nil)
	require.NoError(t, err)
	assert.NotNil(t, results)
	// Should have at least the initial resource
	assert.GreaterOrEqual(t, len(results), 1)
}

func TestBaseProvider_GetResource_Concurrency(t *testing.T) {
	p := newTestProviderWithResources()

	// Set up test data
	resources := make([]models.ProviderResource, 100)
	for i := 0; i < 100; i++ {
		resources[i] = models.ProviderResource{
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
	initialResources := []models.ProviderResource{
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
				resources := []models.ProviderResource{
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
				resource := models.ProviderResource{
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
	results, err := p.ListResources(context.Background(), nil)
	require.NoError(t, err)
	assert.NotNil(t, results)
}

func TestBaseProvider_ResourceRWLock_Behavior(t *testing.T) {
	p := newTestProviderWithResources()

	resources := []models.ProviderResource{
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
		newResources := []models.ProviderResource{
			{ID: "res-3", Name: "Resource Three"},
		}
		p.SetResources(newResources)
	}()

	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	// Verify the writer succeeded
	res3, err := p.GetResource(context.Background(), "res-3")
	assert.NoError(t, err, "Writer should have succeeded even with concurrent readers")
	assert.NotNil(t, res3)
}

func TestBaseProvider_ResourceDataRaceDetection(t *testing.T) {
	// This test is designed to catch data races when run with -race flag
	p := newTestProviderWithResources()

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 1000; i++ {
			p.SetResources([]models.ProviderResource{
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
			p.AddResources(models.ProviderResource{
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
	results, err := p.ListResources(context.Background(), nil)
	require.NoError(t, err)
	assert.NotNil(t, results)
}

func TestCreateKeysFromResources(t *testing.T) {
	resource := models.ProviderResource{
		ID:   "res-123",
		Name: "Production-Database",
	}

	keys := models.CreateKeysFromResources(resource)
	assert.Len(t, keys, 2)
	assert.Contains(t, keys, "res-123")
	assert.Contains(t, keys, "Production-Database")
}
