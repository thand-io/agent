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

// Helper function to create a BaseProvider with identity support
func newTestProviderWithIdentities() *models.BaseProvider {
	provider := models.ProviderConfig{
		Name:        "test-provider",
		Description: "Test Provider",
		Provider:    "test",
	}

	return models.NewBaseProvider("test-provider-id", provider,
		models.NewProviderCapabilities().WithDefaultIdentitiesConfiguration())
}

func TestBaseProvider_SetIdentities(t *testing.T) {
	p := newTestProviderWithIdentities()

	identities := []models.Identity{
		{
			ID:    "user1",
			Label: "User One",
			User: &models.User{
				ID:    "user1",
				Name:  "User One",
				Email: "user1@example.com",
			},
		},
		{
			ID:    "user2",
			Label: "User Two",
			User: &models.User{
				ID:    "user2",
				Name:  "User Two",
				Email: "user2@example.com",
			},
		},
		{
			ID:    "group1",
			Label: "Group One",
			Group: &models.Group{
				ID:    "group1",
				Name:  "Group One",
				Email: "group1@example.com",
			},
		},
	}

	// Test SetIdentities
	p.SetIdentities(identities)

	// Give time for async index building to start
	time.Sleep(10 * time.Millisecond)

	// Verify identities are set
	results, err := p.ListIdentities(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Verify all identities can be retrieved by ID
	user1, err := p.GetIdentity(context.Background(), "user1")
	require.NoError(t, err)
	assert.Equal(t, "user1", user1.ID)
	assert.Equal(t, "User One", user1.Label)

	user2, err := p.GetIdentity(context.Background(), "user2")
	require.NoError(t, err)
	assert.Equal(t, "user2", user2.ID)

	group1, err := p.GetIdentity(context.Background(), "group1")
	require.NoError(t, err)
	assert.Equal(t, "group1", group1.ID)

	// Verify identities can be retrieved by label (case-insensitive)
	user1ByLabel, err := p.GetIdentity(context.Background(), "user one")
	require.NoError(t, err)
	assert.Equal(t, "user1", user1ByLabel.ID)
	assert.Equal(t, "User One", user1ByLabel.Label)

	// Verify identities can be retrieved by email
	user1ByEmail, err := p.GetIdentity(context.Background(), "user1@example.com")
	require.NoError(t, err)
	assert.Equal(t, "user1", user1ByEmail.ID)
	assert.Equal(t, "user1@example.com", user1ByEmail.User.Email)
}

func TestBaseProvider_SetIdentities_ReplacesExisting(t *testing.T) {
	p := newTestProviderWithIdentities()

	// Set initial identities
	initialIdentities := []models.Identity{
		{ID: "user1", Label: "User One"},
		{ID: "user2", Label: "User Two"},
	}
	p.SetIdentities(initialIdentities)

	// Replace with new identities
	newIdentities := []models.Identity{
		{ID: "user3", Label: "User Three"},
	}
	p.SetIdentities(newIdentities)

	// Verify old identities are replaced
	results, err := p.ListIdentities(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "user3", results[0].Result.ID)

	// Verify old identities are not accessible
	_, err = p.GetIdentity(context.Background(), "user1")
	assert.Error(t, err)
	_, err = p.GetIdentity(context.Background(), "user2")
	assert.Error(t, err)

	// Verify new identity is accessible
	user3, err := p.GetIdentity(context.Background(), "user3")
	require.NoError(t, err)
	assert.Equal(t, "user3", user3.ID)
}

func TestBaseProvider_AddIdentities(t *testing.T) {
	p := newTestProviderWithIdentities()

	// Set initial identities
	initialIdentities := []models.Identity{
		{ID: "user1", Label: "User One"},
	}
	p.SetIdentities(initialIdentities)
	time.Sleep(10 * time.Millisecond)

	// Add new identities
	p.AddIdentities(
		models.Identity{ID: "user2", Label: "User Two"},
		models.Identity{ID: "group1", Label: "Group One"},
	)
	time.Sleep(10 * time.Millisecond)

	// Verify all identities are present
	results, err := p.ListIdentities(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Verify all identities are accessible
	_, err = p.GetIdentity(context.Background(), "user1")
	require.NoError(t, err)
	_, err = p.GetIdentity(context.Background(), "user2")
	require.NoError(t, err)
	_, err = p.GetIdentity(context.Background(), "group1")
	require.NoError(t, err)
}

func TestBaseProvider_AddIdentities_FiltersDuplicates(t *testing.T) {
	p := newTestProviderWithIdentities()

	// Set initial identities
	initialIdentities := []models.Identity{
		{ID: "user1", Label: "User One"},
		{ID: "user2", Label: "User Two"},
	}
	p.SetIdentities(initialIdentities)
	time.Sleep(10 * time.Millisecond)

	// Try to add duplicate and new identity
	p.AddIdentities(
		models.Identity{ID: "user1", Label: "User One"},  // duplicate by ID
		models.Identity{ID: "user3", Label: "User Two"},  // duplicate by Label
		models.Identity{ID: "user4", Label: "User Four"}, // new
	)
	time.Sleep(10 * time.Millisecond)

	// Verify duplicates are filtered
	results, err := p.ListIdentities(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, results, 3, "Should have 3 identities (2 initial + 1 new)")

	// Verify expected identities are accessible
	_, err = p.GetIdentity(context.Background(), "user1")
	require.NoError(t, err)
	_, err = p.GetIdentity(context.Background(), "user2")
	require.NoError(t, err)
	_, err = p.GetIdentity(context.Background(), "user4")
	require.NoError(t, err)
}

func TestBaseProvider_GetIdentity(t *testing.T) {
	p := newTestProviderWithIdentities()

	identities := []models.Identity{
		{
			ID:    "user-1",
			Label: "John Doe",
			User: &models.User{
				ID:    "user-1",
				Name:  "John Doe",
				Email: "john@example.com",
			},
		},
		{
			ID:    "group-1",
			Label: "Admin Group",
			Group: &models.Group{
				ID:    "group-1",
				Name:  "Admin Group",
				Email: "admins@example.com",
			},
		},
	}
	p.SetIdentities(identities)

	ctx := context.Background()

	// Test getting identity by ID
	identity, err := p.GetIdentity(ctx, "user-1")
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, "user-1", identity.ID)
	assert.Equal(t, "John Doe", identity.Label)

	// Test getting identity by Label (case-insensitive)
	identity, err = p.GetIdentity(ctx, "john doe")
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, "user-1", identity.ID)

	// Test getting identity by Email
	identity, err = p.GetIdentity(ctx, "john@example.com")
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, "user-1", identity.ID)

	// Test getting group by different case
	identity, err = p.GetIdentity(ctx, "ADMIN GROUP")
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, "group-1", identity.ID)

	// Test getting group by email
	identity, err = p.GetIdentity(ctx, "admins@example.com")
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, "group-1", identity.ID)

	// Test getting non-existent identity
	identity, err = p.GetIdentity(ctx, "non-existent")
	assert.Error(t, err)
	assert.Nil(t, identity)
	assert.Contains(t, err.Error(), "identity not found")
}

func TestBaseProvider_GetIdentity_WithoutCapability(t *testing.T) {
	// Create provider without identity capability
	p := models.NewBaseProvider("test", models.ProviderConfig{Name: "test"}, models.NewProviderCapabilities())

	ctx := context.Background()
	identity, err := p.GetIdentity(ctx, "user1")
	assert.Error(t, err)
	assert.Nil(t, identity)
	assert.Contains(t, err.Error(), "does not support identities")
}

func TestBaseProvider_ListIdentities(t *testing.T) {
	p := newTestProviderWithIdentities()

	identities := []models.Identity{
		{
			ID:    "user1",
			Label: "Production Admin",
			User: &models.User{
				ID:    "user1",
				Name:  "Production Admin",
				Email: "admin@prod.com",
			},
		},
		{
			ID:    "user2",
			Label: "Development User",
			User: &models.User{
				ID:    "user2",
				Name:  "Development User",
				Email: "dev@test.com",
			},
		},
		{
			ID:    "group1",
			Label: "Staging Group",
			Group: &models.Group{
				ID:    "group1",
				Name:  "Staging Group",
				Email: "staging@example.com",
			},
		},
	}
	p.SetIdentities(identities)

	ctx := context.Background()

	// Test listing all identities (no filter)
	results, err := p.ListIdentities(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Test listing with empty search request
	results, err = p.ListIdentities(ctx, &models.SearchRequest{})
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Test listing with filter by label
	results, err = p.ListIdentities(ctx, &models.SearchRequest{
		Terms: []string{"production"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "user1", results[0].Result.ID)

	// Test listing with case-insensitive filter
	results, err = p.ListIdentities(ctx, &models.SearchRequest{
		Terms: []string{"DEVELOPMENT"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "user2", results[0].Result.ID)

	// Test listing with email filter
	results, err = p.ListIdentities(ctx, &models.SearchRequest{
		Terms: []string{"staging@example.com"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "group1", results[0].Result.ID)

	// Test partial match on name
	results, err = p.ListIdentities(ctx, &models.SearchRequest{
		Terms: []string{"admin"},
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 1)
}

func TestBaseProvider_ListIdentities_WithoutCapability(t *testing.T) {
	p := models.NewBaseProvider("test", models.ProviderConfig{Name: "test"}, models.NewProviderCapabilities())

	ctx := context.Background()
	results, err := p.ListIdentities(ctx, nil)
	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "does not support identities")
}

func TestBaseProvider_ListIdentities_Search(t *testing.T) {

	p := models.NewBaseProvider("test", models.ProviderConfig{
		Name: "Test Provider",
	}, models.NewProviderCapabilities().WithDefaultIdentitiesConfiguration())

	userEmail := "hugh@thand.io"
	identity := models.Identity{
		ID:    "user1",
		Label: "Hugh",
		User: &models.User{
			Email: userEmail,
			Name:  "Hugh",
		},
	}

	p.SetIdentities([]models.Identity{identity})

	// Wait for index to be built
	time.Sleep(500 * time.Millisecond)

	ctx := context.Background()
	// Simulate what identities.go does: append *
	searchReq := &models.SearchRequest{
		Query: userEmail + "*",
		Terms: []string{userEmail},
	}

	results, err := p.ListIdentities(ctx, searchReq)
	assert.NoError(t, err)
	assert.NotEmpty(t, results, "Should find identity by email")

	if len(results) > 0 {
		assert.Equal(t, "user1", results[0].ID)
	}
}

// CONCURRENCY TESTS

func TestBaseProvider_SetIdentities_Concurrency(t *testing.T) {
	p := newTestProviderWithIdentities()

	var wg sync.WaitGroup
	goroutines := 100
	identitiesPerGoroutine := 10

	// Concurrently set identities from multiple goroutines
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			identities := make([]models.Identity, identitiesPerGoroutine)
			for j := 0; j < identitiesPerGoroutine; j++ {
				identities[j] = models.Identity{
					ID:    fmt.Sprintf("identity-%d-%d", id, j),
					Label: fmt.Sprintf("Identity %d-%d", id, j),
				}
			}
			p.SetIdentities(identities)
		}(i)
	}

	wg.Wait()
	time.Sleep(50 * time.Millisecond) // Let async operations settle

	// Verify no race conditions occurred and data is consistent
	results, err := p.ListIdentities(context.Background(), nil)
	require.NoError(t, err)
	assert.NotNil(t, results)

	// The last SetIdentities wins, should have identitiesPerGoroutine items
	assert.Equal(t, identitiesPerGoroutine, len(results))
}

func TestBaseProvider_AddIdentities_Concurrency(t *testing.T) {
	p := newTestProviderWithIdentities()

	// Set initial state
	p.SetIdentities([]models.Identity{
		{ID: "initial-1", Label: "Initial One"},
	})
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	goroutines := 50
	identitiesPerGoroutine := 2

	// Concurrently add identities from multiple goroutines
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			identities := make([]models.Identity, identitiesPerGoroutine)
			for j := 0; j < identitiesPerGoroutine; j++ {
				identities[j] = models.Identity{
					ID:    fmt.Sprintf("identity-%d-%d", id, j),
					Label: fmt.Sprintf("Identity %d-%d", id, j),
				}
			}
			p.AddIdentities(identities...)
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond) // Let async operations settle

	// Verify no race conditions occurred
	results, err := p.ListIdentities(context.Background(), nil)
	require.NoError(t, err)
	assert.NotNil(t, results)
	// Should have at least the initial identity
	assert.GreaterOrEqual(t, len(results), 1)
}

func TestBaseProvider_GetIdentity_Concurrency(t *testing.T) {
	p := newTestProviderWithIdentities()

	// Set up test data
	identities := make([]models.Identity, 100)
	for i := 0; i < 100; i++ {
		identities[i] = models.Identity{
			ID:    fmt.Sprintf("identity-%d", i),
			Label: fmt.Sprintf("Identity %d", i),
		}
	}
	p.SetIdentities(identities)
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	goroutines := 100
	ctx := context.Background()

	// Concurrently read identities from multiple goroutines
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Try to get identity multiple times
			for j := 0; j < 10; j++ {
				identityID := fmt.Sprintf("identity-%d", id%100)
				identity, err := p.GetIdentity(ctx, identityID)
				assert.NoError(t, err)
				if identity != nil {
					assert.Equal(t, identityID, identity.ID)
				}
			}
		}(i)
	}

	wg.Wait()
	// If no panic occurred, the test passes
}

func TestBaseProvider_MixedIdentityOperations_Concurrency(t *testing.T) {
	p := newTestProviderWithIdentities()

	// Set initial state
	initialIdentities := []models.Identity{
		{ID: "initial-1", Label: "Initial One"},
		{ID: "initial-2", Label: "Initial Two"},
	}
	p.SetIdentities(initialIdentities)
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	ctx := context.Background()
	iterations := 100

	// Writers: SetIdentities
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				identities := []models.Identity{
					{ID: fmt.Sprintf("set-%d-%d", id, j), Label: fmt.Sprintf("Set %d %d", id, j)},
				}
				p.SetIdentities(identities)
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// Writers: AddIdentities
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				identity := models.Identity{
					ID:    fmt.Sprintf("add-%d-%d", id, j),
					Label: fmt.Sprintf("Add %d %d", id, j),
				}
				p.AddIdentities(identity)
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// Readers: GetIdentity
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Try to get initial identities
				_, _ = p.GetIdentity(ctx, "initial-1")
				_, _ = p.GetIdentity(ctx, "initial-2")
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// Readers: ListIdentities
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, _ = p.ListIdentities(ctx, nil)
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond) // Let async operations settle

	// Verify data structures are still valid
	results, err := p.ListIdentities(context.Background(), nil)
	require.NoError(t, err)
	assert.NotNil(t, results)
}

func TestBaseProvider_IdentityRWLock_Behavior(t *testing.T) {
	p := newTestProviderWithIdentities()

	identities := []models.Identity{
		{ID: "identity-1", Label: "Identity One"},
		{ID: "identity-2", Label: "Identity Two"},
	}
	p.SetIdentities(identities)
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
				_, _ = p.GetIdentity(ctx, "identity-1")
				_, _ = p.ListIdentities(ctx, nil)
			}
		}()
	}

	// Start a writer in the middle
	time.Sleep(10 * time.Millisecond)
	wg.Add(1)
	go func() {
		defer wg.Done()
		newIdentities := []models.Identity{
			{ID: "identity-3", Label: "Identity Three"},
		}
		p.SetIdentities(newIdentities)
	}()

	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	// Verify the writer succeeded
	identity3, err := p.GetIdentity(context.Background(), "identity-3")
	assert.NoError(t, err, "Writer should have succeeded even with concurrent readers")
	assert.NotNil(t, identity3)
}

func TestBaseProvider_IdentityDataRaceDetection(t *testing.T) {
	// This test is designed to catch data races when run with -race flag
	p := newTestProviderWithIdentities()

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 1000; i++ {
			p.SetIdentities([]models.Identity{
				{ID: fmt.Sprintf("identity-%d", i), Label: fmt.Sprintf("Identity %d", i)},
			})
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		ctx := context.Background()
		for i := 0; i < 1000; i++ {
			_, _ = p.ListIdentities(ctx, nil)
			_, _ = p.GetIdentity(ctx, "identity-0")
		}
		done <- true
	}()

	// Add goroutine
	go func() {
		for i := 0; i < 1000; i++ {
			p.AddIdentities(models.Identity{
				ID:    fmt.Sprintf("add-identity-%d", i),
				Label: fmt.Sprintf("Add Identity %d", i),
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
	results, err := p.ListIdentities(context.Background(), nil)
	require.NoError(t, err)
	assert.NotNil(t, results)
}

func TestCreateKeysFromIdentity(t *testing.T) {
	// Test with user
	userIdentity := models.Identity{
		ID:    "user-123",
		Label: "John Doe",
		User: &models.User{
			ID:    "user-123",
			Name:  "John Doe",
			Email: "john@example.com",
		},
	}

	keys := models.CreateKeysFromIdentity(userIdentity)
	assert.Contains(t, keys, "user-123")
	assert.Contains(t, keys, "John Doe")
	assert.Contains(t, keys, "john@example.com")

	// Test with group
	groupIdentity := models.Identity{
		ID:    "group-456",
		Label: "Admin Group",
		Group: &models.Group{
			ID:    "group-456",
			Name:  "Admin Group",
			Email: "admins@example.com",
		},
	}

	keys = models.CreateKeysFromIdentity(groupIdentity)
	assert.Contains(t, keys, "group-456")
	assert.Contains(t, keys, "Admin Group")
	assert.Contains(t, keys, "admins@example.com")

	// Test with minimal identity
	minimalIdentity := models.Identity{
		ID:    "minimal",
		Label: "Minimal",
	}

	keys = models.CreateKeysFromIdentity(minimalIdentity)
	assert.Len(t, keys, 2)
	assert.Contains(t, keys, "minimal")
	assert.Contains(t, keys, "Minimal")
}
