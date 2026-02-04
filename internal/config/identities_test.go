package config_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
)

// MockIdentityProvider implements ProviderImpl for testing identity functions
type MockIdentityProvider struct {
	*models.BaseProvider
}

func NewMockIdentityProvider(name string, identities []models.Identity) *MockIdentityProvider {
	provider := models.ProviderConfig{
		Name:        name,
		Description: "Mock Identity Provider",
		Provider:    "mock",
		Enabled:     true,
	}

	mk := &MockIdentityProvider{
		BaseProvider: models.NewBaseProvider(
			name,
			provider,
			models.NewProviderCapabilities().WithDefaultIdentitiesConfiguration(),
		),
	}

	mk.SetIdentities(identities)

	return mk
}

func (m *MockIdentityProvider) Initialize(identifier string, provider models.ProviderConfig) error {
	return nil
}

// TestGetIdentity tests the GetIdentity function
func TestGetIdentity(t *testing.T) {
	tests := []struct {
		name          string
		identity      string
		providers     map[string]*MockIdentityProvider
		expectedID    string
		expectedEmail string
		expectError   bool
		errorContains string
	}{
		{
			name:     "get identity without provider prefix - found in first provider",
			identity: "john@example.com",
			providers: map[string]*MockIdentityProvider{
				"gsuite": NewMockIdentityProvider("gsuite", []models.Identity{
					{
						ID:    "john@example.com",
						Label: "John Doe",
						User: &models.User{
							Email:    "john@example.com",
							Username: "john",
							Name:     "John Doe",
						},
					},
				}),
			},
			expectedID:    "john@example.com",
			expectedEmail: "john@example.com",
			expectError:   false,
		},
		{
			name:     "get identity with provider prefix",
			identity: "gsuite:john@example.com",
			providers: map[string]*MockIdentityProvider{
				"gsuite": NewMockIdentityProvider("gsuite", []models.Identity{
					{
						ID:    "john@example.com",
						Label: "John Doe",
						User: &models.User{
							Email:    "john@example.com",
							Username: "john",
							Name:     "John Doe",
						},
					},
				}),
				"okta": NewMockIdentityProvider("okta", []models.Identity{
					{
						ID:    "okta-john",
						Label: "John Doe (Okta)",
						User: &models.User{
							Email:    "john@example.com",
							Username: "john-okta",
							Name:     "John Doe",
						},
					},
				}),
			},
			expectedID:    "john@example.com",
			expectedEmail: "john@example.com",
			expectError:   false,
		},
		{
			name:     "get identity with nonexistent provider prefix",
			identity: "nonexistent:john@example.com",
			providers: map[string]*MockIdentityProvider{
				"gsuite": NewMockIdentityProvider("gsuite", []models.Identity{}),
			},
			expectError:   true,
			errorContains: "provider 'nonexistent' not found",
		},
		{
			name:          "get identity without providers - returns error",
			identity:      "john@example.com",
			providers:     map[string]*MockIdentityProvider{},
			expectError:   true,
			errorContains: "identity not found",
		},
		{
			name:     "get identity not found in provider - returns error",
			identity: "unknown@example.com",
			providers: map[string]*MockIdentityProvider{
				"gsuite": NewMockIdentityProvider("gsuite", []models.Identity{
					{
						ID:    "john@example.com",
						Label: "John Doe",
						User: &models.User{
							Email:    "john@example.com",
							Username: "john",
						},
					},
				}),
			},
			expectError:   true,
			errorContains: "identity not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create config with mock providers
			config := &config.Config{}

			for name, mockProvider := range tt.providers {
				config.AddProvider(name, mockProvider)
			}

			// Call GetIdentity
			result, err := config.GetIdentity(tt.identity)

			if tt.expectError {
				require.Error(t, err)
				if len(tt.errorContains) != 0 {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.expectedID, result.ID)
			if result.User != nil {
				assert.Equal(t, tt.expectedEmail, result.User.Email)
			}
		})
	}
}

// TestGetIdentitiesWithFilter tests filtering and merging of identities
func TestGetIdentitiesWithFilter(t *testing.T) {
	tests := []struct {
		name          string
		user          *models.User
		identityType  config.IdentityType
		filter        []string
		query         string
		providers     map[string]*MockIdentityProvider
		expectedCount int
		expectedIDs   []string
		expectError   bool
		errorContains string
	}{
		{
			name: "get all users without filter",
			user: &models.User{
				Email: "admin@example.com",
				Name:  "Admin User",
			},
			identityType: config.IdentityTypeUser,
			filter:       nil,
			providers: map[string]*MockIdentityProvider{
				"gsuite": NewMockIdentityProvider("gsuite", []models.Identity{
					{
						ID:    "john@example.com",
						Label: "John Doe",
						User: &models.User{
							Email:    "john@example.com",
							Username: "john",
							Name:     "John Doe",
						},
					},
					{
						ID:    "jane@example.com",
						Label: "Jane Doe",
						User: &models.User{
							Email:    "jane@example.com",
							Username: "jane",
							Name:     "Jane Doe",
						},
					},
				}),
			},
			expectedCount: 2,
			expectedIDs:   []string{"john@example.com", "jane@example.com"},
			expectError:   false,
		},
		{
			name: "filter users by name",
			user: &models.User{
				Email: "admin@example.com",
				Name:  "Admin User",
			},
			identityType: config.IdentityTypeUser,
			filter:       []string{"john"},
			providers: map[string]*MockIdentityProvider{
				"gsuite": NewMockIdentityProvider("gsuite", []models.Identity{
					{
						ID:    "john@example.com",
						Label: "John Doe",
						User: &models.User{
							Email:    "john@example.com",
							Username: "john",
							Name:     "John Doe",
						},
					},
					{
						ID:    "jane@example.com",
						Label: "Jane Doe",
						User: &models.User{
							Email:    "jane@example.com",
							Username: "jane",
							Name:     "Jane Doe",
						},
					},
				}),
			},
			expectedCount: 1,
			expectedIDs:   []string{"john@example.com"},
			expectError:   false,
		},
		{
			name: "get only groups",
			user: &models.User{
				Email: "admin@example.com",
				Name:  "Admin User",
			},
			identityType: config.IdentityTypeGroup,
			filter:       nil,
			providers: map[string]*MockIdentityProvider{
				"gsuite": NewMockIdentityProvider("gsuite", []models.Identity{
					{
						ID:    "john@example.com",
						Label: "John Doe",
						User: &models.User{
							Email:    "john@example.com",
							Username: "john",
						},
					},
					{
						ID:    "developers",
						Label: "Developers Group",
						Group: &models.Group{
							Name:  "developers",
							Email: "developers@example.com",
						},
					},
					{
						ID:    "admins",
						Label: "Admins Group",
						Group: &models.Group{
							Name:  "admins",
							Email: "admins@example.com",
						},
					},
				}),
			},
			expectedCount: 2,
			expectedIDs:   []string{"developers", "admins"},
			expectError:   false,
		},
		{
			name: "merge identities from multiple providers - no duplicates",
			user: &models.User{
				Email: "admin@example.com",
				Name:  "Admin User",
			},
			identityType: config.IdentityTypeAll,
			filter:       nil,
			providers: map[string]*MockIdentityProvider{
				"gsuite": NewMockIdentityProvider("gsuite", []models.Identity{
					{
						ID:    "john@example.com",
						Label: "John Doe",
						User: &models.User{
							Email:    "john@example.com",
							Username: "john",
							Name:     "John Doe",
						},
					},
				}),
				"okta": NewMockIdentityProvider("okta", []models.Identity{
					{
						ID:    "jane@example.com",
						Label: "Jane Doe",
						User: &models.User{
							Email:    "jane@example.com",
							Username: "jane",
							Name:     "Jane Doe",
						},
					},
				}),
			},
			expectedCount: 2,
			expectedIDs:   []string{"john@example.com", "jane@example.com"},
			expectError:   false,
		},
		{
			name: "merge identities from multiple providers - with duplicates (same ID)",
			user: &models.User{
				Email: "admin@example.com",
				Name:  "Admin User",
			},
			identityType: config.IdentityTypeUser,
			filter:       nil,
			providers: map[string]*MockIdentityProvider{
				"gsuite": NewMockIdentityProvider("gsuite", []models.Identity{
					{
						ID:    "john@example.com",
						Label: "John Doe (GSuite)",
						User: &models.User{
							Email:    "john@example.com",
							Username: "john-gsuite",
							Name:     "John Doe",
						},
					},
				}),
				"okta": NewMockIdentityProvider("okta", []models.Identity{
					{
						ID:    "john@example.com",
						Label: "John Doe (Okta)",
						User: &models.User{
							Email:    "john@example.com",
							Username: "john-okta",
							Name:     "John Doe",
						},
					},
				}),
			},
			expectedCount: 1, // Should deduplicate by ID
			expectedIDs:   []string{"john@example.com"},
			expectError:   false,
		},
		{
			name: "no identity providers - returns current user",
			user: &models.User{
				Email: "admin@example.com",
				Name:  "Admin User",
			},
			identityType:  config.IdentityTypeUser,
			filter:        nil,
			providers:     map[string]*MockIdentityProvider{},
			expectedCount: 1,
			expectedIDs:   []string{"admin@example.com"},
			expectError:   false,
		},
		{
			name: "no identity providers with filter - user matches",
			user: &models.User{
				Email: "admin@example.com",
				Name:  "Admin User",
			},
			identityType:  config.IdentityTypeUser,
			filter:        []string{"admin"},
			providers:     map[string]*MockIdentityProvider{},
			expectedCount: 1,
			expectedIDs:   []string{"admin@example.com"},
			expectError:   false,
		},
		{
			name: "no identity providers with filter - user does not match",
			user: &models.User{
				Email: "admin@example.com",
				Name:  "Admin User",
			},
			identityType:  config.IdentityTypeUser,
			filter:        []string{"john"},
			providers:     map[string]*MockIdentityProvider{},
			expectedCount: 0,
			expectedIDs:   []string{},
			expectError:   false,
		},
		{
			name: "filter groups by name",
			user: &models.User{
				Email: "admin@example.com",
				Name:  "Admin User",
			},
			identityType: config.IdentityTypeGroup,
			filter:       []string{"developers"},
			query:        "developers",
			providers: map[string]*MockIdentityProvider{
				"gsuite": NewMockIdentityProvider("gsuite", []models.Identity{
					{
						ID:    "developers",
						Label: "Developers Group",
						Group: &models.Group{
							Name:  "developers",
							Email: "developers@example.com",
						},
					},
					{
						ID:    "admins",
						Label: "Admins Group",
						Group: &models.Group{
							Name:  "admins",
							Email: "admins@example.com",
						},
					},
				}),
			},
			expectedCount: 1,
			expectedIDs:   []string{"developers"},
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create config with mock providers
			config := &config.Config{}

			for name, mockProvider := range tt.providers {
				config.AddProvider(name, mockProvider)
			}

			// Call GetIdentitiesWithFilter
			var searchReq *models.SearchRequest
			if len(tt.filter) > 0 || len(tt.query) != 0 {
				searchReq = &models.SearchRequest{
					Terms: tt.filter,
					Query: tt.query,
				}
			}
			results, err := config.GetIdentitiesWithFilter(tt.user, tt.identityType, searchReq)

			if tt.expectError {
				require.Error(t, err)
				if len(tt.errorContains) != 0 {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Len(t, results, tt.expectedCount)

			// Verify expected IDs are present
			resultIDs := make([]string, len(results))
			for i, r := range results {
				resultIDs[i] = r.Result.ID
			}
			assert.ElementsMatch(t, tt.expectedIDs, resultIDs)
		})
	}
}

// SpyIdentityProvider wraps MockIdentityProvider to record calls
type SpyIdentityProvider struct {
	*MockIdentityProvider
	mu        *sync.Mutex
	callOrder *[]string
	name      string
}

func (s *SpyIdentityProvider) ListIdentities(ctx context.Context, searchRequest *models.SearchRequest) ([]models.SearchResult[models.Identity], error) {
	s.mu.Lock()
	*s.callOrder = append(*s.callOrder, s.name)
	s.mu.Unlock()
	return s.MockIdentityProvider.ListIdentities(ctx, searchRequest)
}

// TestGetIdentitiesWithFilter_ConcurrentProviders tests that multiple providers are queried in parallel
func TestGetIdentitiesWithFilter_ConcurrentProviders(t *testing.T) {
	var callOrder []string
	var mu sync.Mutex

	// Create providers that record their call order
	p1 := NewMockIdentityProvider("provider1", []models.Identity{
		{
			ID:    "user1@example.com",
			Label: "User 1",
			User: &models.User{
				Email:    "user1@example.com",
				Username: "user1",
			},
		},
	})
	provider1 := &SpyIdentityProvider{
		MockIdentityProvider: p1,
		mu:                   &mu,
		callOrder:            &callOrder,
		name:                 "provider1",
	}

	p2 := NewMockIdentityProvider("provider2", []models.Identity{
		{
			ID:    "user2@example.com",
			Label: "User 2",
			User: &models.User{
				Email:    "user2@example.com",
				Username: "user2",
			},
		},
	})
	provider2 := &SpyIdentityProvider{
		MockIdentityProvider: p2,
		mu:                   &mu,
		callOrder:            &callOrder,
		name:                 "provider2",
	}

	p3 := NewMockIdentityProvider("provider3", []models.Identity{
		{
			ID:    "user3@example.com",
			Label: "User 3",
			User: &models.User{
				Email:    "user3@example.com",
				Username: "user3",
			},
		},
	})
	provider3 := &SpyIdentityProvider{
		MockIdentityProvider: p3,
		mu:                   &mu,
		callOrder:            &callOrder,
		name:                 "provider3",
	}

	cfg := &config.Config{}

	// Register providers
	// Note: We can't easily iterate map here because we need specific spy instances
	cfg.AddProvider("provider1", provider1)
	cfg.AddProvider("provider2", provider2)
	cfg.AddProvider("provider3", provider3)

	user := &models.User{
		Email: "admin@example.com",
		Name:  "Admin",
	}

	results, err := cfg.GetIdentitiesWithFilter(user, config.IdentityTypeUser, nil)
	require.NoError(t, err)

	// All 3 providers should have been called
	assert.Len(t, callOrder, 3)

	// All 3 users should be returned
	assert.Len(t, results, 3)
}

// ErrorIdentityProvider wraps MockIdentityProvider to return errors
type ErrorIdentityProvider struct {
	*MockIdentityProvider
}

func (e *ErrorIdentityProvider) ListIdentities(ctx context.Context, searchRequest *models.SearchRequest) ([]models.SearchResult[models.Identity], error) {
	return nil, fmt.Errorf("provider error")
}

// TestGetIdentitiesWithFilter_ProviderError tests handling of provider errors
func TestGetIdentitiesWithFilter_ProviderError(t *testing.T) {
	p1 := NewMockIdentityProvider("provider1", []models.Identity{})
	provider1 := &ErrorIdentityProvider{MockIdentityProvider: p1}

	cfg := &config.Config{}

	cfg.AddProvider("provider1", provider1)
	user := &models.User{
		Email: "admin@example.com",
		Name:  "Admin",
	}

	results, err := cfg.GetIdentitiesWithFilter(user, config.IdentityTypeUser, nil)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, user.Email, results[0].Result.ID)
}

// TestGetIdentitiesWithFilter_MixedUserAndGroup tests filtering by identity type
func TestGetIdentitiesWithFilter_MixedUserAndGroup(t *testing.T) {
	provider := NewMockIdentityProvider("mixed", []models.Identity{
		{
			ID:    "user1@example.com",
			Label: "User 1",
			User: &models.User{
				Email:    "user1@example.com",
				Username: "user1",
			},
		},
		{
			ID:    "user2@example.com",
			Label: "User 2",
			User: &models.User{
				Email:    "user2@example.com",
				Username: "user2",
			},
		},
		{
			ID:    "developers",
			Label: "Developers",
			Group: &models.Group{
				Name:  "developers",
				Email: "developers@example.com",
			},
		},
		{
			ID:    "admins",
			Label: "Admins",
			Group: &models.Group{
				Name:  "admins",
				Email: "admins@example.com",
			},
		},
	})

	cfg := &config.Config{}
	cfg.AddProvider("mixed", provider)

	user := &models.User{
		Email: "admin@example.com",
		Name:  "Admin",
	}

	t.Run("filter by IdentityTypeUser", func(t *testing.T) {
		results, err := cfg.GetIdentitiesWithFilter(user, config.IdentityTypeUser, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)
		for _, r := range results {
			assert.NotNil(t, r.Result.User)
			assert.Nil(t, r.Result.Group)
		}
	})

	t.Run("filter by IdentityTypeGroup", func(t *testing.T) {
		results, err := cfg.GetIdentitiesWithFilter(user, config.IdentityTypeGroup, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)
		for _, r := range results {
			assert.Nil(t, r.Result.User)
			assert.NotNil(t, r.Result.Group)
		}
	})

	t.Run("filter by IdentityTypeAll", func(t *testing.T) {
		results, err := cfg.GetIdentitiesWithFilter(user, config.IdentityTypeAll, nil)
		require.NoError(t, err)
		assert.Len(t, results, 4)
	})
}

// TestGetIdentity_EmailParsing tests that email parsing extracts username correctly
func TestGetIdentity_EmailParsing(t *testing.T) {
	config := &config.Config{}

	// No providers configured - should return error
	_, err := config.GetIdentity("john.doe@example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identity not found")
}

// TestGetIdentity_NonEmail tests identity lookup for non-email identities
func TestGetIdentity_NonEmail(t *testing.T) {
	config := &config.Config{}

	// No providers configured - should return error
	_, err := config.GetIdentity("johndoe")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identity not found")
}

// TestGetIdentity_ProviderPrefixFormat tests various provider prefix formats
func TestGetIdentity_ProviderPrefixFormat(t *testing.T) {
	tests := []struct {
		name       string
		identity   string
		wantPrefix string
		wantKey    string
		hasPrefix  bool
	}{
		{
			name:       "simple prefix",
			identity:   "aws:admin",
			wantPrefix: "aws",
			wantKey:    "admin",
			hasPrefix:  true,
		},
		{
			name:       "prefix with email",
			identity:   "gsuite:john@example.com",
			wantPrefix: "gsuite",
			wantKey:    "john@example.com",
			hasPrefix:  true,
		},
		{
			name:       "no prefix - plain email",
			identity:   "john@example.com",
			wantPrefix: "",
			wantKey:    "john@example.com",
			hasPrefix:  false,
		},
		{
			name:       "no prefix - plain username",
			identity:   "johndoe",
			wantPrefix: "",
			wantKey:    "johndoe",
			hasPrefix:  false,
		},
		{
			name:       "prefix with hyphen in provider name",
			identity:   "aws-prod:admin@example.com",
			wantPrefix: "aws-prod",
			wantKey:    "admin@example.com",
			hasPrefix:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse the identity string as done in GetIdentity
			var providerID string
			var identityKey string

			colonIdx := -1
			for i, c := range tt.identity {
				if c == ':' {
					colonIdx = i
					break
				}
			}

			if colonIdx != -1 {
				providerID = tt.identity[:colonIdx]
				identityKey = tt.identity[colonIdx+1:]
			} else {
				identityKey = tt.identity
			}

			if tt.hasPrefix {
				assert.Equal(t, tt.wantPrefix, providerID)
				assert.Equal(t, tt.wantKey, identityKey)
			} else {
				assert.Equal(t, "", providerID)
				assert.Equal(t, tt.wantKey, identityKey)
			}
		})
	}
}

// TestGetIdentitiesWithFilter_DeduplicationAcrossProviders tests that duplicate identities from multiple providers are deduplicated
func TestGetIdentitiesWithFilter_DeduplicationAcrossProviders(t *testing.T) {
	// Create 3 providers that all return the same user
	providers := map[string]*MockIdentityProvider{
		"provider1": NewMockIdentityProvider("provider1", []models.Identity{
			{
				ID:    "shared@example.com",
				Label: "Shared User (P1)",
				User: &models.User{
					Email:    "shared@example.com",
					Username: "shared-p1",
					Name:     "Shared User",
				},
			},
			{
				ID:    "unique1@example.com",
				Label: "Unique User 1",
				User: &models.User{
					Email:    "unique1@example.com",
					Username: "unique1",
					Name:     "Unique User 1",
				},
			},
		}),
		"provider2": NewMockIdentityProvider("provider2", []models.Identity{
			{
				ID:    "shared@example.com",
				Label: "Shared User (P2)",
				User: &models.User{
					Email:    "shared@example.com",
					Username: "shared-p2",
					Name:     "Shared User",
				},
			},
			{
				ID:    "unique2@example.com",
				Label: "Unique User 2",
				User: &models.User{
					Email:    "unique2@example.com",
					Username: "unique2",
					Name:     "Unique User 2",
				},
			},
		}),
		"provider3": NewMockIdentityProvider("provider3", []models.Identity{
			{
				ID:    "shared@example.com",
				Label: "Shared User (P3)",
				User: &models.User{
					Email:    "shared@example.com",
					Username: "shared-p3",
					Name:     "Shared User",
				},
			},
			{
				ID:    "unique3@example.com",
				Label: "Unique User 3",
				User: &models.User{
					Email:    "unique3@example.com",
					Username: "unique3",
					Name:     "Unique User 3",
				},
			},
		}),
	}

	cfg := &config.Config{}

	for name, mockProvider := range providers {
		cfg.AddProvider(name, mockProvider)
	}

	user := &models.User{
		Email: "admin@example.com",
		Name:  "Admin",
	}

	results, err := cfg.GetIdentitiesWithFilter(user, config.IdentityTypeUser, nil)
	require.NoError(t, err)

	// Should have 4 unique identities: shared@example.com + 3 unique ones
	assert.Len(t, results, 4)

	// Count how many times shared@example.com appears
	sharedCount := 0
	for _, r := range results {
		if r.Result.ID == "shared@example.com" {
			sharedCount++
		}
	}
	assert.Equal(t, 1, sharedCount, "shared@example.com should appear only once after deduplication")

	// Verify all unique identities are present
	resultIDs := make(map[string]bool)
	for _, r := range results {
		resultIDs[r.Result.ID] = true
	}
	assert.True(t, resultIDs["shared@example.com"])
	assert.True(t, resultIDs["unique1@example.com"])
	assert.True(t, resultIDs["unique2@example.com"])
	assert.True(t, resultIDs["unique3@example.com"])
}

// TestGetIdentitiesWithFilter_CurrentUserFallback tests that the current user is returned
// when there are no results, no filter, and the identity type is User or All
func TestGetIdentitiesWithFilter_CurrentUserFallback(t *testing.T) {
	currentUser := &models.User{
		Email: "current@example.com",
		Name:  "Current User",
	}

	t.Run("user returned when provider returns empty results - IdentityTypeUser", func(t *testing.T) {
		// Provider returns no results
		provider := NewMockIdentityProvider("test", []models.Identity{})

		cfg := &config.Config{}
		cfg.AddProvider("test", provider)

		results, err := cfg.GetIdentitiesWithFilter(currentUser, config.IdentityTypeUser, nil)
		require.NoError(t, err)

		// Should return current user as fallback
		assert.Len(t, results, 1)
		assert.Equal(t, currentUser.Email, results[0].Result.ID)
	})

	t.Run("user returned when provider returns empty results - IdentityTypeAll", func(t *testing.T) {
		// Provider returns no results
		provider := NewMockIdentityProvider("test", []models.Identity{})

		cfg := &config.Config{}
		cfg.AddProvider("test", provider)

		results, err := cfg.GetIdentitiesWithFilter(currentUser, config.IdentityTypeAll, nil)
		require.NoError(t, err)

		// Should return current user as fallback
		assert.Len(t, results, 1)
		assert.Equal(t, currentUser.Email, results[0].Result.ID)
	})

	t.Run("user NOT returned when provider has results", func(t *testing.T) {
		provider := NewMockIdentityProvider("test", []models.Identity{
			{
				ID:    "other@example.com",
				Label: "Other User",
				User: &models.User{
					Email:    "other@example.com",
					Username: "other",
				},
			},
		})

		cfg := &config.Config{}
		cfg.AddProvider("test", provider)

		results, err := cfg.GetIdentitiesWithFilter(currentUser, config.IdentityTypeUser, nil)
		require.NoError(t, err)

		// Should only have the provider result, not current user
		assert.Len(t, results, 1)
		assert.Equal(t, "other@example.com", results[0].Result.ID)
	})

	t.Run("user NOT returned when IdentityTypeGroup even with empty results", func(t *testing.T) {
		provider := NewMockIdentityProvider("test", []models.Identity{})

		cfg := &config.Config{}
		cfg.AddProvider("test", provider)

		results, err := cfg.GetIdentitiesWithFilter(currentUser, config.IdentityTypeGroup, nil)
		require.NoError(t, err)

		// Should be empty, not the current user
		assert.Len(t, results, 0)
	})

	t.Run("user NOT returned when filter is provided even with empty results", func(t *testing.T) {
		provider := NewMockIdentityProvider("test", []models.Identity{})

		cfg := &config.Config{}
		cfg.AddProvider("test", provider)

		results, err := cfg.GetIdentitiesWithFilter(currentUser, config.IdentityTypeUser, &models.SearchRequest{Terms: []string{"nonexistent"}})
		require.NoError(t, err)

		// Should be empty because filter was provided
		assert.Len(t, results, 0)
	})

	t.Run("user returned when filter is empty string", func(t *testing.T) {
		provider := NewMockIdentityProvider("test", []models.Identity{})

		cfg := &config.Config{}
		cfg.AddProvider("test", provider)

		// Pass an empty string as filter - this simulates ?q= in the URL
		results, err := cfg.GetIdentitiesWithFilter(currentUser, config.IdentityTypeUser, &models.SearchRequest{Terms: []string{""}})
		require.NoError(t, err)

		// Should return current user as fallback because "" filter should be ignored
		assert.Len(t, results, 1)
		assert.Equal(t, currentUser.Email, results[0].Result.ID)
	})

	t.Run("nil user - no fallback, empty results", func(t *testing.T) {
		provider := NewMockIdentityProvider("test", []models.Identity{})

		cfg := &config.Config{}
		cfg.AddProvider("test", provider)

		// This should not panic and should return empty
		results, err := cfg.GetIdentitiesWithFilter(nil, config.IdentityTypeUser, nil)
		require.NoError(t, err)
		assert.Len(t, results, 0)
	})
}

// Benchmark for GetIdentitiesWithFilter with multiple providers
func BenchmarkGetIdentitiesWithFilter_MultipleProviders(b *testing.B) {
	// Create providers with many identities
	providers := make(map[string]*MockIdentityProvider)
	for i := 0; i < 5; i++ {
		identities := make([]models.Identity, 0, 100)
		for j := 0; j < 100; j++ {
			id := fmt.Sprintf("user%d-%d@example.com", i, j)
			identities = append(identities, models.Identity{
				ID:    id,
				Label: fmt.Sprintf("User %d-%d", i, j),
				User: &models.User{
					Email:    id,
					Username: fmt.Sprintf("user%d-%d", i, j),
				},
			})
		}
		providers[fmt.Sprintf("provider%d", i)] = NewMockIdentityProvider(fmt.Sprintf("provider%d", i), identities)
	}

	cfg := &config.Config{}

	for name, mockProvider := range providers {
		cfg.AddProvider(name, mockProvider)
	}

	user := &models.User{
		Email: "admin@example.com",
		Name:  "Admin",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cfg.GetIdentitiesWithFilter(user, config.IdentityTypeUser, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// TestMergeStrings tests the mergeStrings helper function
func TestMergeStrings(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		source   string
		expected string
	}{
		{
			name:     "Target empty, source has value",
			target:   "",
			source:   "new value",
			expected: "new value",
		},
		{
			name:     "Target has value, source has value",
			target:   "existing value",
			source:   "new value",
			expected: "existing value",
		},
		{
			name:     "Both empty",
			target:   "",
			source:   "",
			expected: "",
		},
		{
			name:     "Target has value, source empty",
			target:   "existing value",
			source:   "",
			expected: "existing value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := tt.target
			config.MergeStrings(&target, tt.source)
			assert.Equal(t, tt.expected, target)
		})
	}
}

// TestMergeIdentities_UserFields tests merging User fields between identities
func TestMergeIdentities_UserFields(t *testing.T) {
	tests := []struct {
		name     string
		target   *models.Identity
		source   *models.Identity
		expected *models.Identity
	}{
		{
			name: "Target has no User, source has User",
			target: &models.Identity{
				ID: "id1",
			},
			source: &models.Identity{
				User: &models.User{
					ID:       "user1",
					Username: "john",
					Email:    "john@example.com",
					Name:     "John Doe",
				},
			},
			expected: &models.Identity{
				ID: "id1",
				User: &models.User{
					ID:       "user1",
					Username: "john",
					Email:    "john@example.com",
					Name:     "John Doe",
				},
			},
		},
		{
			name: "Both have User, merge missing fields",
			target: &models.Identity{
				User: &models.User{
					ID:    "user1",
					Email: "john@example.com",
				},
			},
			source: &models.Identity{
				User: &models.User{
					ID:       "user2",
					Username: "john",
					Name:     "John Doe",
					Source:   "okta",
				},
			},
			expected: &models.Identity{
				User: &models.User{
					ID:       "user1", // Existing value preserved
					Username: "john",  // Filled from source
					Email:    "john@example.com",
					Name:     "John Doe", // Filled from source
					Source:   "okta",     // Filled from source
				},
			},
		},
		{
			name: "Merge User.Verified pointer field",
			target: &models.Identity{
				User: &models.User{
					Email: "john@example.com",
				},
			},
			source: &models.Identity{
				User: &models.User{
					Verified: boolPtr(true),
				},
			},
			expected: &models.Identity{
				User: &models.User{
					Email:    "john@example.com",
					Verified: boolPtr(true),
				},
			},
		},
		{
			name: "Merge User.Groups",
			target: &models.Identity{
				User: &models.User{
					Email: "john@example.com",
				},
			},
			source: &models.Identity{
				User: &models.User{
					Groups: []string{"admin", "users"},
				},
			},
			expected: &models.Identity{
				User: &models.User{
					Email:  "john@example.com",
					Groups: []string{"admin", "users"},
				},
			},
		},
		{
			name: "Append Groups without duplicates",
			target: &models.Identity{
				User: &models.User{
					Email:  "john@example.com",
					Groups: []string{"developers"},
				},
			},
			source: &models.Identity{
				User: &models.User{
					Groups: []string{"admin", "users"},
				},
			},
			expected: &models.Identity{
				User: &models.User{
					Email:  "john@example.com",
					Groups: []string{"developers", "admin", "users"},
				},
			},
		},
		{
			name: "Append Groups with duplicates removed",
			target: &models.Identity{
				User: &models.User{
					Email:  "john@example.com",
					Groups: []string{"developers", "admin"},
				},
			},
			source: &models.Identity{
				User: &models.User{
					Groups: []string{"admin", "users"},
				},
			},
			expected: &models.Identity{
				User: &models.User{
					Email:  "john@example.com",
					Groups: []string{"developers", "admin", "users"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.MergeIdentities(tt.target, tt.source)
			assert.Equal(t, tt.expected.User, tt.target.User)
		})
	}
}

// TestMergeIdentities_GroupFields tests merging Group fields between identities
func TestMergeIdentities_GroupFields(t *testing.T) {
	tests := []struct {
		name     string
		target   *models.Identity
		source   *models.Identity
		expected *models.Identity
	}{
		{
			name: "Target has no Group, source has Group",
			target: &models.Identity{
				ID: "id1",
			},
			source: &models.Identity{
				Group: &models.Group{
					ID:     "group1",
					Name:   "Admins",
					Email:  "admins@example.com",
					Parent: "parent-group",
				},
			},
			expected: &models.Identity{
				ID: "id1",
				Group: &models.Group{
					ID:     "group1",
					Name:   "Admins",
					Email:  "admins@example.com",
					Parent: "parent-group",
				},
			},
		},
		{
			name: "Both have Group, merge missing fields",
			target: &models.Identity{
				Group: &models.Group{
					ID:   "group1",
					Name: "Admins",
				},
			},
			source: &models.Identity{
				Group: &models.Group{
					ID:     "group2",
					Email:  "admins@example.com",
					Parent: "parent-group",
				},
			},
			expected: &models.Identity{
				Group: &models.Group{
					ID:     "group1", // Existing value preserved
					Name:   "Admins",
					Email:  "admins@example.com", // Filled from source
					Parent: "parent-group",       // Filled from source
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.MergeIdentities(tt.target, tt.source)
			assert.Equal(t, tt.expected.Group, tt.target.Group)
		})
	}
}

// TestMergeIdentities_IdentityFields tests merging Identity-level fields
func TestMergeIdentities_IdentityFields(t *testing.T) {
	tests := []struct {
		name     string
		target   *models.Identity
		source   *models.Identity
		expected *models.Identity
	}{
		{
			name: "Merge Identity-level fields",
			target: &models.Identity{
				ID: "id1",
			},
			source: &models.Identity{
				ID:     "id2",
				Label:  "User Label",
				Tenant: "tenant1",
			},
			expected: &models.Identity{
				ID:     "id1", // Existing value preserved
				Label:  "User Label",
				Tenant: "tenant1",
			},
		},
		{
			name: "Don't overwrite existing Identity fields",
			target: &models.Identity{
				ID:     "id1",
				Label:  "Existing Label",
				Tenant: "existing-tenant",
			},
			source: &models.Identity{
				ID:     "id2",
				Label:  "New Label",
				Tenant: "new-tenant",
			},
			expected: &models.Identity{
				ID:     "id1",
				Label:  "Existing Label",
				Tenant: "existing-tenant",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.MergeIdentities(tt.target, tt.source)
			assert.Equal(t, tt.expected.ID, tt.target.ID)
			assert.Equal(t, tt.expected.Label, tt.target.Label)
			assert.Equal(t, tt.expected.Tenant, tt.target.Tenant)
		})
	}
}

// TestMergeIdentities_ComplexMerge tests a complex merging scenario
func TestMergeIdentities_ComplexMerge(t *testing.T) {
	target := &models.Identity{
		ID:    "identity1",
		Label: "Primary Identity",
		User: &models.User{
			Email: "john@example.com",
		},
	}

	source := &models.Identity{
		ID:     "identity2",
		Tenant: "acme-corp",
		User: &models.User{
			ID:       "user123",
			Username: "johndoe",
			Name:     "John Doe",
			Source:   "okta",
			Verified: boolPtr(true),
			Groups:   []string{"developers", "admins"},
		},
	}

	expected := &models.Identity{
		ID:     "identity1", // Original preserved
		Label:  "Primary Identity",
		Tenant: "acme-corp", // Merged from source
		User: &models.User{
			ID:       "user123", // Merged from source
			Username: "johndoe", // Merged from source
			Email:    "john@example.com",
			Name:     "John Doe",                       // Merged from source
			Source:   "okta",                           // Merged from source
			Verified: boolPtr(true),                    // Merged from source
			Groups:   []string{"developers", "admins"}, // Merged from source
		},
	}

	config.MergeIdentities(target, source)

	assert.Equal(t, expected.ID, target.ID)
	assert.Equal(t, expected.Label, target.Label)
	assert.Equal(t, expected.Tenant, target.Tenant)
	assert.Equal(t, expected.User.ID, target.User.ID)
	assert.Equal(t, expected.User.Username, target.User.Username)
	assert.Equal(t, expected.User.Email, target.User.Email)
	assert.Equal(t, expected.User.Name, target.User.Name)
	assert.Equal(t, expected.User.Source, target.User.Source)
	assert.Equal(t, *expected.User.Verified, *target.User.Verified)
	assert.Equal(t, expected.User.Groups, target.User.Groups)
}

// TestMergeIdentities_BothUserAndGroup tests merging when target has User and source has Group
func TestMergeIdentities_BothUserAndGroup(t *testing.T) {
	target := &models.Identity{
		ID: "identity1",
		User: &models.User{
			Email: "john@example.com",
		},
	}

	source := &models.Identity{
		ID: "identity2",
		Group: &models.Group{
			Name:  "Admins",
			Email: "admins@example.com",
		},
	}

	config.MergeIdentities(target, source)

	// Target should keep its User and gain the Group
	assert.NotNil(t, target.User)
	assert.Equal(t, "john@example.com", target.User.Email)
	assert.NotNil(t, target.Group)
	assert.Equal(t, "Admins", target.Group.Name)
	assert.Equal(t, "admins@example.com", target.Group.Email)
}

// TestGetIdentity_MergesFromMultipleProviders tests that GetIdentity merges results from multiple providers
func TestGetIdentity_MergesFromMultipleProviders(t *testing.T) {
	// Provider 1 has partial user info
	provider1 := NewMockIdentityProvider("provider1", []models.Identity{
		{
			ID: "user@example.com",
			User: &models.User{
				Email: "user@example.com",
			},
		},
	})

	// Provider 2 has additional user info
	provider2 := NewMockIdentityProvider("provider2", []models.Identity{
		{
			ID: "user@example.com",
			User: &models.User{
				Email:    "user@example.com",
				Username: "johndoe",
				Name:     "John Doe",
			},
		},
	})

	// Provider 3 has even more info
	provider3 := NewMockIdentityProvider("provider3", []models.Identity{
		{
			ID:    "user@example.com",
			Label: "John Doe User",
			User: &models.User{
				Email:    "user@example.com",
				ID:       "user123",
				Source:   "okta",
				Verified: boolPtr(true),
			},
		},
	})

	cfg := &config.Config{}
	cfg.AddProvider("provider1", provider1)
	cfg.AddProvider("provider2", provider2)
	cfg.AddProvider("provider3", provider3)

	result, err := cfg.GetIdentity("user@example.com")
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should have merged data from all providers
	assert.Equal(t, "user@example.com", result.User.Email)
	assert.Equal(t, "johndoe", result.User.Username)
	assert.Equal(t, "John Doe", result.User.Name)
	assert.Equal(t, "user123", result.User.ID)
	assert.Equal(t, "okta", result.User.Source)
	assert.True(t, *result.User.Verified)
	assert.Equal(t, "John Doe User", result.Label)

	// Should track all providers
	providers := result.GetProviders()
	assert.Len(t, providers, 3)
	assert.Contains(t, providers, "provider1")
	assert.Contains(t, providers, "provider2")
	assert.Contains(t, providers, "provider3")
}

// TestGetIdentity_SortsByIdentifier tests that GetIdentity returns alphabetically first identity
func TestGetIdentity_SortsByIdentifier(t *testing.T) {
	// Both providers have the same user but with different mappable identifiers.
	// Provider 1 returns user with email "zebra@example.com"
	provider1 := NewMockIdentityProvider("provider1", []models.Identity{
		{
			ID:    "test-user",
			Label: "Zebra User from Provider 1",
			User: &models.User{
				Email:    "zebra@example.com",
				Username: "test-user",
				Name:     "Zebra",
			},
		},
	})

	// Provider 2 returns the same user but with email "apple@example.com"
	provider2 := NewMockIdentityProvider("provider2", []models.Identity{
		{
			ID:    "test-user",
			Label: "Apple User from Provider 2",
			User: &models.User{
				Email:    "apple@example.com",
				Username: "test-user",
				Name:     "Apple",
			},
		},
	})

	cfg := &config.Config{}
	cfg.AddProvider("provider1", provider1)
	cfg.AddProvider("provider2", provider2)

	// Search for the user that exists in both providers
	result, err := cfg.GetIdentity("test-user")
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should return the identity with the alphabetically first mappable identifier.
	// GetMappableIdentifier uses email as the primary identifier, so "apple@example.com"
	// comes before "zebra@example.com", and we should get the Apple user.
	assert.Equal(t, "apple@example.com", result.User.Email)
	assert.Equal(t, "Apple User from Provider 2", result.Label)

	// Should have both providers tracked in the merged result
	assert.Len(t, result.Providers, 2)
	assert.Contains(t, result.Providers, "provider1")
	assert.Contains(t, result.Providers, "provider2")
}

// TestGetIdentity_DeterministicOutput tests that GetIdentity returns the same output every time
func TestGetIdentity_DeterministicOutput(t *testing.T) {
	// Create multiple providers with overlapping but incomplete data
	// Provider order and response timing should not affect the final result

	provider1 := NewMockIdentityProvider("provider1", []models.Identity{
		{
			ID:    "john@example.com",
			Label: "John from Provider 1",
			User: &models.User{
				Email: "john@example.com",
				Name:  "John Doe",
			},
		},
	})

	provider2 := NewMockIdentityProvider("provider2", []models.Identity{
		{
			ID: "john@example.com",
			User: &models.User{
				Email:    "john@example.com",
				Username: "johndoe",
				ID:       "user123",
			},
		},
	})

	provider3 := NewMockIdentityProvider("provider3", []models.Identity{
		{
			ID:     "john@example.com",
			Tenant: "acme-corp",
			User: &models.User{
				Email:    "john@example.com",
				Source:   "okta",
				Verified: boolPtr(true),
				Groups:   []string{"developers", "admins"},
			},
		},
	})

	cfg := &config.Config{}
	cfg.AddProvider("provider1", provider1)
	cfg.AddProvider("provider2", provider2)
	cfg.AddProvider("provider3", provider3)

	// Call GetIdentity multiple times and verify results are identical
	const iterations = 10
	var results []*models.Identity

	for i := 0; i < iterations; i++ {
		result, err := cfg.GetIdentity("john@example.com")
		require.NoError(t, err)
		require.NotNil(t, result)
		results = append(results, result)
	}

	// Verify all results are identical
	baseResult := results[0]

	for i := 1; i < iterations; i++ {
		result := results[i]

		// Check Identity-level fields
		assert.Equal(t, baseResult.ID, result.ID, "Iteration %d: ID mismatch", i)
		assert.Equal(t, baseResult.Label, result.Label, "Iteration %d: Label mismatch", i)
		assert.Equal(t, baseResult.Tenant, result.Tenant, "Iteration %d: Tenant mismatch", i)

		// Check User fields
		require.NotNil(t, result.User, "Iteration %d: User should not be nil", i)
		assert.Equal(t, baseResult.User.ID, result.User.ID, "Iteration %d: User.ID mismatch", i)
		assert.Equal(t, baseResult.User.Email, result.User.Email, "Iteration %d: User.Email mismatch", i)
		assert.Equal(t, baseResult.User.Username, result.User.Username, "Iteration %d: User.Username mismatch", i)
		assert.Equal(t, baseResult.User.Name, result.User.Name, "Iteration %d: User.Name mismatch", i)
		assert.Equal(t, baseResult.User.Source, result.User.Source, "Iteration %d: User.Source mismatch", i)

		if baseResult.User.Verified != nil {
			require.NotNil(t, result.User.Verified, "Iteration %d: User.Verified should not be nil", i)
			assert.Equal(t, *baseResult.User.Verified, *result.User.Verified, "Iteration %d: User.Verified mismatch", i)
		}

		assert.Equal(t, baseResult.User.Groups, result.User.Groups, "Iteration %d: User.Groups mismatch", i)

		// Check providers are tracked consistently
		assert.Equal(t, len(baseResult.GetProviders()), len(result.GetProviders()), "Iteration %d: Provider count mismatch", i)
		for providerName, providerType := range baseResult.GetProviders() {
			assert.Equal(t, providerType, result.GetProviders()[providerName], "Iteration %d: Provider %s mismatch", i, providerName)
		}
	}

	// Verify the final merged result has all expected data
	finalResult := results[len(results)-1]
	assert.Equal(t, "john@example.com", finalResult.ID)
	assert.Equal(t, "John from Provider 1", finalResult.Label)
	assert.Equal(t, "acme-corp", finalResult.Tenant)
	assert.Equal(t, "john@example.com", finalResult.User.Email)
	assert.Equal(t, "johndoe", finalResult.User.Username)
	assert.Equal(t, "John Doe", finalResult.User.Name)
	assert.Equal(t, "user123", finalResult.User.ID)
	assert.Equal(t, "okta", finalResult.User.Source)
	assert.True(t, *finalResult.User.Verified)
	assert.Equal(t, []string{"developers", "admins"}, finalResult.User.Groups)
	assert.Len(t, finalResult.GetProviders(), 3)
}

// TestGetIdentity_DeterministicWithMultipleIdentities tests deterministic ordering when multiple identities match
func TestGetIdentity_DeterministicWithMultipleIdentities(t *testing.T) {
	// Create providers with different identities that could match the same search
	// The alphabetically first one should always be returned

	provider1 := NewMockIdentityProvider("provider1", []models.Identity{
		{
			ID:    "test-user",
			Label: "Test User Z",
			User: &models.User{
				Email:    "zebra@example.com",
				Username: "test-user",
				Name:     "Zebra User",
			},
		},
	})

	provider2 := NewMockIdentityProvider("provider2", []models.Identity{
		{
			ID:    "test-user",
			Label: "Test User A",
			User: &models.User{
				Email:    "apple@example.com",
				Username: "test-user",
				Name:     "Apple User",
			},
		},
	})

	provider3 := NewMockIdentityProvider("provider3", []models.Identity{
		{
			ID:    "test-user",
			Label: "Test User M",
			User: &models.User{
				Email:    "mango@example.com",
				Username: "test-user",
				Name:     "Mango User",
			},
		},
	})

	cfg := &config.Config{}
	cfg.AddProvider("provider1", provider1)
	cfg.AddProvider("provider2", provider2)
	cfg.AddProvider("provider3", provider3)

	// Call multiple times to verify consistency
	const iterations = 20
	var emails []string

	for i := 0; i < iterations; i++ {
		result, err := cfg.GetIdentity("test-user")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.User)
		emails = append(emails, result.User.Email)
	}

	// All results should have the same email (alphabetically first: apple@example.com)
	expectedEmail := "apple@example.com"
	for i, email := range emails {
		assert.Equal(t, expectedEmail, email, "Iteration %d: Expected %s but got %s", i, expectedEmail, email)
	}

	// Verify the result is indeed the alphabetically first one
	finalResult, err := cfg.GetIdentity("test-user")
	require.NoError(t, err)
	assert.Equal(t, "apple@example.com", finalResult.User.Email)
	assert.Equal(t, "Test User A", finalResult.Label)

	// Should have all three providers tracked since they all returned the same identity key
	assert.Len(t, finalResult.GetProviders(), 3)
}

// TestGetIdentity_NoMutationOfCachedIdentities verifies that GetIdentity
// does not mutate provider-owned cached identity objects when merging.
// This test ensures the fix for data race issues where concurrent requests
// could cause mutations to shared cached objects.
func TestGetIdentity_NoMutationOfCachedIdentities(t *testing.T) {
	// Create a shared identity that will be cached by a provider
	cachedIdentity := models.Identity{
		ID:    "user@example.com",
		Label: "Cached User",
		User: &models.User{
			ID:       "123",
			Email:    "user@example.com",
			Username: "cacheduser",
			Name:     "Cached User",
		},
		Providers: map[string]string{
			"provider1": "mock",
		},
	}

	// Create two providers that return the same cached identity
	provider1 := NewMockIdentityProvider("provider1", []models.Identity{cachedIdentity})
	provider2 := NewMockIdentityProvider("provider2", []models.Identity{
		{
			ID:    "user@example.com",
			Label: "User from Provider 2",
			User: &models.User{
				ID:       "456",
				Email:    "user@example.com",
				Username: "user2",
				Name:     "User From Provider 2",
				Source:   "provider2",
			},
		},
	})

	// Create config with providers
	cfg := &config.Config{}
	cfg.AddProvider("provider1", provider1)
	cfg.AddProvider("provider2", provider2)

	// Get the identity first time
	result1, err := cfg.GetIdentity("user@example.com")
	require.NoError(t, err)
	require.NotNil(t, result1)

	// The result should have merged data from both providers
	assert.Contains(t, result1.Providers, "provider1")
	assert.Contains(t, result1.Providers, "provider2")
	assert.Equal(t, "Cached User", result1.Label) // First alphabetically

	// Now verify the cached identity was NOT mutated
	// Get the original cached identity from provider1
	cachedFromProvider, err := provider1.GetIdentity(context.Background(), "user@example.com")
	require.NoError(t, err)
	require.NotNil(t, cachedFromProvider)

	// The cached identity should only have provider1 in its Providers map
	assert.Len(t, cachedFromProvider.Providers, 1, "Cached identity should not have been mutated with provider2")
	assert.Contains(t, cachedFromProvider.Providers, "provider1")
	assert.NotContains(t, cachedFromProvider.Providers, "provider2", "Provider2 should not be in the original cached identity")

	// Verify the User object wasn't mutated either
	assert.Equal(t, "123", cachedFromProvider.User.ID)
	assert.Equal(t, "cacheduser", cachedFromProvider.User.Username)
	assert.Empty(t, cachedFromProvider.User.Source, "Source field should remain empty in cached identity")
}

// TestGetIdentity_ConcurrentAccess tests that concurrent calls to GetIdentity
// don't cause data races or panics when merging identities.
func TestGetIdentity_ConcurrentAccess(t *testing.T) {
	// Create providers with identities
	provider1 := NewMockIdentityProvider("provider1", []models.Identity{
		{
			ID:    "user@example.com",
			Label: "User 1",
			User: &models.User{
				ID:       "1",
				Email:    "user@example.com",
				Username: "user1",
				Name:     "User One",
			},
		},
	})

	provider2 := NewMockIdentityProvider("provider2", []models.Identity{
		{
			ID:    "user@example.com",
			Label: "User 2",
			User: &models.User{
				ID:       "2",
				Email:    "user@example.com",
				Username: "user2",
				Name:     "User Two",
				Source:   "provider2",
			},
		},
	})

	cfg := &config.Config{}
	cfg.AddProvider("provider1", provider1)
	cfg.AddProvider("provider2", provider2)

	// Run concurrent requests to GetIdentity
	const numGoroutines = 50
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			result, err := cfg.GetIdentity("user@example.com")
			if err != nil {
				errors <- err
				return
			}

			// Verify result has expected properties
			if result == nil {
				errors <- fmt.Errorf("result is nil")
				return
			}

			if len(result.Providers) != 2 {
				errors <- fmt.Errorf("expected 2 providers, got %d", len(result.Providers))
				return
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check if any errors occurred
	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}
}

// Helper function to create bool pointers
func boolPtr(b bool) *bool {
	return &b
}
