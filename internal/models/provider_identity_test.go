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

// Helper function to create a BaseProvider with identity support
func newTestProviderWithIdentities() *BaseProvider {
	provider := Provider{
		Name:        "test-provider",
		Description: "Test Provider",
		Provider:    "test",
	}

	return NewBaseProvider("test-provider-id", provider,
		NewProviderCapabilities().WithDefaultIdentitiesConfiguration())
}

func TestBaseProvider_SetIdentities(t *testing.T) {
	p := newTestProviderWithIdentities()

	identities := []Identity{
		{
			ID:    "user1",
			Label: "User One",
			User: &User{
				ID:    "user1",
				Name:  "User One",
				Email: "user1@example.com",
			},
		},
		{
			ID:    "user2",
			Label: "User Two",
			User: &User{
				ID:    "user2",
				Name:  "User Two",
				Email: "user2@example.com",
			},
		},
		{
			ID:    "group1",
			Label: "Group One",
			Group: &Group{
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
	p.identity.mu.RLock()
	assert.Len(t, p.identity.identities, 3)
	// Each identity has multiple keys (ID, Label, Email, Name)
	assert.NotEmpty(t, p.identity.identitiesMap)
	p.identity.mu.RUnlock()

	// Verify map contains expected keys (lowercase)
	p.identity.mu.RLock()
	assert.Contains(t, p.identity.identitiesMap, "user1")
	assert.Contains(t, p.identity.identitiesMap, "user one")
	assert.Contains(t, p.identity.identitiesMap, "user1@example.com")
	assert.Contains(t, p.identity.identitiesMap, "group1")
	assert.Contains(t, p.identity.identitiesMap, "group one")
	assert.Contains(t, p.identity.identitiesMap, "group1@example.com")
	p.identity.mu.RUnlock()

	// Verify map points to correct identities
	p.identity.mu.RLock()
	assert.Equal(t, "user1", p.identity.identitiesMap["user1"].ID)
	assert.Equal(t, "User One", p.identity.identitiesMap["user one"].Label)
	assert.Equal(t, "user1@example.com", p.identity.identitiesMap["user1@example.com"].User.Email)
	p.identity.mu.RUnlock()
}

func TestBaseProvider_SetIdentities_ReplacesExisting(t *testing.T) {
	p := newTestProviderWithIdentities()

	// Set initial identities
	initialIdentities := []Identity{
		{ID: "user1", Label: "User One"},
		{ID: "user2", Label: "User Two"},
	}
	p.SetIdentities(initialIdentities)

	// Replace with new identities
	newIdentities := []Identity{
		{ID: "user3", Label: "User Three"},
	}
	p.SetIdentities(newIdentities)

	// Verify old identities are replaced
	p.identity.mu.RLock()
	assert.Len(t, p.identity.identities, 1)
	assert.Equal(t, "user3", p.identity.identities[0].ID)
	assert.NotContains(t, p.identity.identitiesMap, "user1")
	assert.NotContains(t, p.identity.identitiesMap, "user2")
	assert.Contains(t, p.identity.identitiesMap, "user3")
	p.identity.mu.RUnlock()
}

func TestBaseProvider_AddIdentities(t *testing.T) {
	p := newTestProviderWithIdentities()

	// Set initial identities
	initialIdentities := []Identity{
		{ID: "user1", Label: "User One"},
	}
	p.SetIdentities(initialIdentities)
	time.Sleep(10 * time.Millisecond)

	// Add new identities
	p.AddIdentities(
		Identity{ID: "user2", Label: "User Two"},
		Identity{ID: "group1", Label: "Group One"},
	)
	time.Sleep(10 * time.Millisecond)

	// Verify all identities are present
	p.identity.mu.RLock()
	assert.Len(t, p.identity.identities, 3)
	assert.Contains(t, p.identity.identitiesMap, "user1")
	assert.Contains(t, p.identity.identitiesMap, "user2")
	assert.Contains(t, p.identity.identitiesMap, "group1")
	p.identity.mu.RUnlock()
}

func TestBaseProvider_AddIdentities_FiltersDuplicates(t *testing.T) {
	p := newTestProviderWithIdentities()

	// Set initial identities
	initialIdentities := []Identity{
		{ID: "user1", Label: "User One"},
		{ID: "user2", Label: "User Two"},
	}
	p.SetIdentities(initialIdentities)
	time.Sleep(10 * time.Millisecond)

	// Try to add duplicate and new identity
	p.AddIdentities(
		Identity{ID: "user1", Label: "User One"},  // duplicate by ID
		Identity{ID: "user3", Label: "User Two"},  // duplicate by Label
		Identity{ID: "user4", Label: "User Four"}, // new
	)
	time.Sleep(10 * time.Millisecond)

	// Verify duplicates are filtered
	p.identity.mu.RLock()
	assert.Len(t, p.identity.identities, 3, "Should have 3 identities (2 initial + 1 new)")
	assert.Contains(t, p.identity.identitiesMap, "user1")
	assert.Contains(t, p.identity.identitiesMap, "user2")
	assert.Contains(t, p.identity.identitiesMap, "user4")
	p.identity.mu.RUnlock()
}

func TestBaseProvider_GetIdentity(t *testing.T) {
	p := newTestProviderWithIdentities()

	identities := []Identity{
		{
			ID:    "user-1",
			Label: "John Doe",
			User: &User{
				ID:    "user-1",
				Name:  "John Doe",
				Email: "john@example.com",
			},
		},
		{
			ID:    "group-1",
			Label: "Admin Group",
			Group: &Group{
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
	p := NewBaseProvider("test", Provider{Name: "test"}, NewProviderCapabilities())

	ctx := context.Background()
	identity, err := p.GetIdentity(ctx, "user1")
	assert.Error(t, err)
	assert.Nil(t, identity)
	assert.Contains(t, err.Error(), "no identities")
}

func TestBaseProvider_ListIdentities(t *testing.T) {
	p := newTestProviderWithIdentities()

	identities := []Identity{
		{
			ID:    "user1",
			Label: "Production Admin",
			User: &User{
				ID:    "user1",
				Name:  "Production Admin",
				Email: "admin@prod.com",
			},
		},
		{
			ID:    "user2",
			Label: "Development User",
			User: &User{
				ID:    "user2",
				Name:  "Development User",
				Email: "dev@test.com",
			},
		},
		{
			ID:    "group1",
			Label: "Staging Group",
			Group: &Group{
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
	results, err = p.ListIdentities(ctx, &SearchRequest{})
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Test listing with filter by label
	results, err = p.ListIdentities(ctx, &SearchRequest{
		Terms: []string{"production"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "user1", results[0].Result.ID)

	// Test listing with case-insensitive filter
	results, err = p.ListIdentities(ctx, &SearchRequest{
		Terms: []string{"DEVELOPMENT"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "user2", results[0].Result.ID)

	// Test listing with email filter
	results, err = p.ListIdentities(ctx, &SearchRequest{
		Terms: []string{"staging@example.com"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "group1", results[0].Result.ID)

	// Test partial match on name
	results, err = p.ListIdentities(ctx, &SearchRequest{
		Terms: []string{"admin"},
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 1)
}

func TestBaseProvider_ListIdentities_WithoutCapability(t *testing.T) {
	p := NewBaseProvider("test", Provider{Name: "test"}, NewProviderCapabilities())

	ctx := context.Background()
	results, err := p.ListIdentities(ctx, nil)
	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "no identities")
}

func TestBaseProvider_ListIdentities_Search(t *testing.T) {

	p := NewBaseProvider("test", Provider{
		Name: "Test Provider",
	}, NewProviderCapabilities().WithDefaultIdentitiesConfiguration())

	userEmail := "hugh@thand.io"
	identity := Identity{
		ID:    "user1",
		Label: "Hugh",
		User: &User{
			Email: userEmail,
			Name:  "Hugh",
		},
	}

	p.SetIdentities([]Identity{identity})

	// Wait for index to be built
	time.Sleep(500 * time.Millisecond)

	ctx := context.Background()
	// Simulate what identities.go does: append *
	searchReq := &SearchRequest{
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

			identities := make([]Identity, identitiesPerGoroutine)
			for j := 0; j < identitiesPerGoroutine; j++ {
				identities[j] = Identity{
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
	p.identity.mu.RLock()
	assert.NotNil(t, p.identity.identities)
	assert.NotNil(t, p.identity.identitiesMap)
	identityCount := len(p.identity.identities)
	mapCount := len(p.identity.identitiesMap)
	p.identity.mu.RUnlock()

	// The last SetIdentities wins, should have identitiesPerGoroutine items
	assert.Equal(t, identitiesPerGoroutine, identityCount)
	// Each identity has at least 2 keys (ID and Label)
	assert.GreaterOrEqual(t, mapCount, identitiesPerGoroutine*2)
}

func TestBaseProvider_AddIdentities_Concurrency(t *testing.T) {
	p := newTestProviderWithIdentities()

	// Set initial state
	p.SetIdentities([]Identity{
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

			identities := make([]Identity, identitiesPerGoroutine)
			for j := 0; j < identitiesPerGoroutine; j++ {
				identities[j] = Identity{
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
	p.identity.mu.RLock()
	assert.NotNil(t, p.identity.identities)
	assert.NotNil(t, p.identity.identitiesMap)
	// Should have at least the initial identity
	assert.GreaterOrEqual(t, len(p.identity.identities), 1)
	p.identity.mu.RUnlock()
}

func TestBaseProvider_GetIdentity_Concurrency(t *testing.T) {
	p := newTestProviderWithIdentities()

	// Set up test data
	identities := make([]Identity, 100)
	for i := 0; i < 100; i++ {
		identities[i] = Identity{
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
	initialIdentities := []Identity{
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
				identities := []Identity{
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
				identity := Identity{
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
	p.identity.mu.RLock()
	assert.NotNil(t, p.identity.identities)
	assert.NotNil(t, p.identity.identitiesMap)
	p.identity.mu.RUnlock()
}

func TestBaseProvider_IdentityRWLock_Behavior(t *testing.T) {
	p := newTestProviderWithIdentities()

	identities := []Identity{
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
		newIdentities := []Identity{
			{ID: "identity-3", Label: "Identity Three"},
		}
		p.SetIdentities(newIdentities)
	}()

	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	// Verify the writer succeeded
	p.identity.mu.RLock()
	found := false
	for _, i := range p.identity.identities {
		if i.ID == "identity-3" {
			found = true
			break
		}
	}
	p.identity.mu.RUnlock()
	assert.True(t, found, "Writer should have succeeded even with concurrent readers")
}

func TestBaseProvider_IdentityDataRaceDetection(t *testing.T) {
	// This test is designed to catch data races when run with -race flag
	p := newTestProviderWithIdentities()

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 1000; i++ {
			p.SetIdentities([]Identity{
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
			p.AddIdentities(Identity{
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
	p.identity.mu.RLock()
	assert.NotNil(t, p.identity.identities)
	assert.NotNil(t, p.identity.identitiesMap)
	p.identity.mu.RUnlock()
}

func TestCreateKeysFromIdentity(t *testing.T) {
	// Test with user
	userIdentity := Identity{
		ID:    "user-123",
		Label: "John Doe",
		User: &User{
			ID:    "user-123",
			Name:  "John Doe",
			Email: "john@example.com",
		},
	}

	keys := CreateKeysFromIdentity(userIdentity)
	assert.Contains(t, keys, "user-123")
	assert.Contains(t, keys, "John Doe")
	assert.Contains(t, keys, "john@example.com")

	// Test with group
	groupIdentity := Identity{
		ID:    "group-456",
		Label: "Admin Group",
		Group: &Group{
			ID:    "group-456",
			Name:  "Admin Group",
			Email: "admins@example.com",
		},
	}

	keys = CreateKeysFromIdentity(groupIdentity)
	assert.Contains(t, keys, "group-456")
	assert.Contains(t, keys, "Admin Group")
	assert.Contains(t, keys, "admins@example.com")

	// Test with minimal identity
	minimalIdentity := Identity{
		ID:    "minimal",
		Label: "Minimal",
	}

	keys = CreateKeysFromIdentity(minimalIdentity)
	assert.Len(t, keys, 2)
	assert.Contains(t, keys, "minimal")
	assert.Contains(t, keys, "Minimal")
}
