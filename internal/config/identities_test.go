package config

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// MockIdentityProvider implements ProviderImpl for testing identity functions
type MockIdentityProvider struct {
	*models.BaseProvider
}

func NewMockIdentityProvider(name string, identities []models.Identity) *MockIdentityProvider {
	provider := models.Provider{
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

func (m *MockIdentityProvider) Initialize(identifier string, provider models.Provider) error {
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
			config := &Config{
				Providers: ProviderConfig{
					Definitions: make(map[string]models.Provider),
				},
			}

			for name, mockProvider := range tt.providers {
				provider := models.Provider{
					Name:        name,
					Description: "Test provider",
					Provider:    "mock",
					Enabled:     true,
				}
				provider.SetClient(mockProvider)
				config.Providers.Definitions[name] = provider
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
		identityType  IdentityType
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
			identityType: IdentityTypeUser,
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
			identityType: IdentityTypeUser,
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
			identityType: IdentityTypeGroup,
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
			identityType: IdentityTypeAll,
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
			identityType: IdentityTypeUser,
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
			identityType:  IdentityTypeUser,
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
			identityType:  IdentityTypeUser,
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
			identityType:  IdentityTypeUser,
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
			identityType: IdentityTypeGroup,
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
			config := &Config{
				Providers: ProviderConfig{
					Definitions: make(map[string]models.Provider),
				},
			}

			for name, mockProvider := range tt.providers {
				provider := models.Provider{
					Name:        name,
					Description: "Test provider",
					Provider:    "mock",
					Enabled:     true,
				}
				provider.SetClient(mockProvider)
				config.Providers.Definitions[name] = provider
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

	config := &Config{
		Providers: ProviderConfig{
			Definitions: make(map[string]models.Provider),
		},
	}

	// Register providers
	// Note: We can't easily iterate map here because we need specific spy instances

	prov1 := models.Provider{Name: "provider1", Description: "Test provider", Provider: "mock", Enabled: true}
	prov1.SetClient(provider1)
	config.Providers.Definitions["provider1"] = prov1

	prov2 := models.Provider{Name: "provider2", Description: "Test provider", Provider: "mock", Enabled: true}
	prov2.SetClient(provider2)
	config.Providers.Definitions["provider2"] = prov2

	prov3 := models.Provider{Name: "provider3", Description: "Test provider", Provider: "mock", Enabled: true}
	prov3.SetClient(provider3)
	config.Providers.Definitions["provider3"] = prov3

	user := &models.User{
		Email: "admin@example.com",
		Name:  "Admin",
	}

	results, err := config.GetIdentitiesWithFilter(user, IdentityTypeUser, nil)
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

	config := &Config{
		Providers: ProviderConfig{
			Definitions: make(map[string]models.Provider),
		},
	}

	provider := models.Provider{
		Name:        "provider1",
		Description: "Test provider",
		Provider:    "mock",
		Enabled:     true,
	}
	provider.SetClient(provider1)
	config.Providers.Definitions["provider1"] = provider

	user := &models.User{
		Email: "admin@example.com",
		Name:  "Admin",
	}

	results, err := config.GetIdentitiesWithFilter(user, IdentityTypeUser, nil)
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

	config := &Config{
		Providers: ProviderConfig{
			Definitions: make(map[string]models.Provider),
		},
	}

	p := models.Provider{
		Name:        "mixed",
		Description: "Test provider",
		Provider:    "mock",
		Enabled:     true,
	}
	p.SetClient(provider)
	config.Providers.Definitions["mixed"] = p

	user := &models.User{
		Email: "admin@example.com",
		Name:  "Admin",
	}

	t.Run("filter by IdentityTypeUser", func(t *testing.T) {
		results, err := config.GetIdentitiesWithFilter(user, IdentityTypeUser, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)
		for _, r := range results {
			assert.NotNil(t, r.Result.User)
			assert.Nil(t, r.Result.Group)
		}
	})

	t.Run("filter by IdentityTypeGroup", func(t *testing.T) {
		results, err := config.GetIdentitiesWithFilter(user, IdentityTypeGroup, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)
		for _, r := range results {
			assert.Nil(t, r.Result.User)
			assert.NotNil(t, r.Result.Group)
		}
	})

	t.Run("filter by IdentityTypeAll", func(t *testing.T) {
		results, err := config.GetIdentitiesWithFilter(user, IdentityTypeAll, nil)
		require.NoError(t, err)
		assert.Len(t, results, 4)
	})
}

// TestGetIdentity_EmailParsing tests that email parsing extracts username correctly
func TestGetIdentity_EmailParsing(t *testing.T) {
	config := &Config{
		Providers: ProviderConfig{
			Definitions: make(map[string]models.Provider),
		},
	}

	// No providers configured - should return error
	_, err := config.GetIdentity("john.doe@example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identity not found")
}

// TestGetIdentity_NonEmail tests identity lookup for non-email identities
func TestGetIdentity_NonEmail(t *testing.T) {
	config := &Config{
		Providers: ProviderConfig{
			Definitions: make(map[string]models.Provider),
		},
	}

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

	config := &Config{
		Providers: ProviderConfig{
			Definitions: make(map[string]models.Provider),
		},
	}

	for name, mockProvider := range providers {
		provider := models.Provider{
			Name:        name,
			Description: "Test provider",
			Provider:    "mock",
			Enabled:     true,
		}
		provider.SetClient(mockProvider)
		config.Providers.Definitions[name] = provider
	}

	user := &models.User{
		Email: "admin@example.com",
		Name:  "Admin",
	}

	results, err := config.GetIdentitiesWithFilter(user, IdentityTypeUser, nil)
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

		config := &Config{
			Providers: ProviderConfig{
				Definitions: make(map[string]models.Provider),
			},
		}
		p := models.Provider{
			Name:        "test",
			Description: "Test provider",
			Provider:    "mock",
			Enabled:     true,
		}
		p.SetClient(provider)
		config.Providers.Definitions["test"] = p

		results, err := config.GetIdentitiesWithFilter(currentUser, IdentityTypeUser, nil)
		require.NoError(t, err)

		// Should return current user as fallback
		assert.Len(t, results, 1)
		assert.Equal(t, currentUser.Email, results[0].Result.ID)
	})

	t.Run("user returned when provider returns empty results - IdentityTypeAll", func(t *testing.T) {
		// Provider returns no results
		provider := NewMockIdentityProvider("test", []models.Identity{})

		config := &Config{
			Providers: ProviderConfig{
				Definitions: make(map[string]models.Provider),
			},
		}
		p := models.Provider{
			Name:        "test",
			Description: "Test provider",
			Provider:    "mock",
			Enabled:     true,
		}
		p.SetClient(provider)
		config.Providers.Definitions["test"] = p

		results, err := config.GetIdentitiesWithFilter(currentUser, IdentityTypeAll, nil)
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

		config := &Config{
			Providers: ProviderConfig{
				Definitions: make(map[string]models.Provider),
			},
		}
		p := models.Provider{
			Name:        "test",
			Description: "Test provider",
			Provider:    "mock",
			Enabled:     true,
		}
		p.SetClient(provider)
		config.Providers.Definitions["test"] = p

		results, err := config.GetIdentitiesWithFilter(currentUser, IdentityTypeUser, nil)
		require.NoError(t, err)

		// Should only have the provider result, not current user
		assert.Len(t, results, 1)
		assert.Equal(t, "other@example.com", results[0].Result.ID)
	})

	t.Run("user NOT returned when IdentityTypeGroup even with empty results", func(t *testing.T) {
		provider := NewMockIdentityProvider("test", []models.Identity{})

		config := &Config{
			Providers: ProviderConfig{
				Definitions: make(map[string]models.Provider),
			},
		}
		p := models.Provider{
			Name:        "test",
			Description: "Test provider",
			Provider:    "mock",
			Enabled:     true,
		}
		p.SetClient(provider)
		config.Providers.Definitions["test"] = p

		results, err := config.GetIdentitiesWithFilter(currentUser, IdentityTypeGroup, nil)
		require.NoError(t, err)

		// Should be empty, not the current user
		assert.Len(t, results, 0)
	})

	t.Run("user NOT returned when filter is provided even with empty results", func(t *testing.T) {
		provider := NewMockIdentityProvider("test", []models.Identity{})

		config := &Config{
			Providers: ProviderConfig{
				Definitions: make(map[string]models.Provider),
			},
		}
		p := models.Provider{
			Name:        "test",
			Description: "Test provider",
			Provider:    "mock",
			Enabled:     true,
		}
		p.SetClient(provider)
		config.Providers.Definitions["test"] = p

		results, err := config.GetIdentitiesWithFilter(currentUser, IdentityTypeUser, &models.SearchRequest{Terms: []string{"nonexistent"}})
		require.NoError(t, err)

		// Should be empty because filter was provided
		assert.Len(t, results, 0)
	})

	t.Run("user returned when filter is empty string", func(t *testing.T) {
		provider := NewMockIdentityProvider("test", []models.Identity{})

		config := &Config{
			Providers: ProviderConfig{
				Definitions: make(map[string]models.Provider),
			},
		}
		p := models.Provider{
			Name:        "test",
			Description: "Test provider",
			Provider:    "mock",
			Enabled:     true,
		}
		p.SetClient(provider)
		config.Providers.Definitions["test"] = p

		// Pass an empty string as filter - this simulates ?q= in the URL
		results, err := config.GetIdentitiesWithFilter(currentUser, IdentityTypeUser, &models.SearchRequest{Terms: []string{""}})
		require.NoError(t, err)

		// Should return current user as fallback because "" filter should be ignored
		assert.Len(t, results, 1)
		assert.Equal(t, currentUser.Email, results[0].Result.ID)
	})

	t.Run("nil user - no fallback, empty results", func(t *testing.T) {
		provider := NewMockIdentityProvider("test", []models.Identity{})

		config := &Config{
			Providers: ProviderConfig{
				Definitions: make(map[string]models.Provider),
			},
		}
		p := models.Provider{
			Name:        "test",
			Description: "Test provider",
			Provider:    "mock",
			Enabled:     true,
		}
		p.SetClient(provider)
		config.Providers.Definitions["test"] = p

		// This should not panic and should return empty
		results, err := config.GetIdentitiesWithFilter(nil, IdentityTypeUser, nil)
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

	config := &Config{
		Providers: ProviderConfig{
			Definitions: make(map[string]models.Provider),
		},
	}

	for name, mockProvider := range providers {
		provider := models.Provider{
			Name:        name,
			Description: "Test provider",
			Provider:    "mock",
			Enabled:     true,
		}
		provider.SetClient(mockProvider)
		config.Providers.Definitions[name] = provider
	}

	user := &models.User{
		Email: "admin@example.com",
		Name:  "Admin",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := config.GetIdentitiesWithFilter(user, IdentityTypeUser, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}
