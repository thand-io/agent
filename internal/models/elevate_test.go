package models_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thand-io/agent/internal/models"
)

func NewMockIdentityProvider(name string, identities []models.Identity) models.Provider {
	provider := models.ProviderConfig{
		Name:        name,
		Description: "Mock Provider",
		Provider:    "mock",
		Enabled:     true,
	}

	base := models.NewBaseProvider(name, provider,
		models.NewProviderCapabilities().
			WithDefaultIdentitiesConfiguration())
	base.SetIdentities(identities)

	return base
}

func TestResolveIdentities(t *testing.T) {
	// Define identities
	id1 := models.Identity{
		ID:    "user1@example.com",
		Label: "User One",
		User: &models.User{
			Email: "user1@example.com",
			Name:  "User One",
		},
	}
	id2 := models.Identity{
		ID:    "user2",
		Label: "User Two",
		User: &models.User{
			Email: "user2@example.com",
			Name:  "User Two",
		},
	}
	id3 := models.Identity{
		ID:    "group1",
		Label: "Group One",
		Group: &models.Group{
			Name: "Group One",
		},
	}
	id4 := models.Identity{
		ID:    "user4-id",
		Label: "User Four",
		User: &models.User{
			Email: "user4@example.com",
			Name:  "User Four",
		},
	}

	// Create providers
	mp1 := NewMockIdentityProvider("p1", []models.Identity{id1, id2})
	mp2 := NewMockIdentityProvider("p2", []models.Identity{id3, id4})

	providers := map[string]models.Provider{
		"p1": mp1,
		"p2": mp2,
	}

	tests := []struct {
		name          string
		identities    []string
		expectedCount int
		expectedIDs   []string
	}{
		{
			name:          "Resolve by ID",
			identities:    []string{"user1@example.com"},
			expectedCount: 1,
			expectedIDs:   []string{"user1@example.com"},
		},
		{
			name:          "Resolve by Name (ID)",
			identities:    []string{"user2"},
			expectedCount: 1,
			expectedIDs:   []string{"user2"},
		},
		{
			name:          "Resolve by Label",
			identities:    []string{"User Two"},
			expectedCount: 1,
			expectedIDs:   []string{"user2"},
		},
		{
			name:          "Resolve with Provider Prefix",
			identities:    []string{"p1:user1@example.com"},
			expectedCount: 1,
			expectedIDs:   []string{"user1@example.com"},
		},
		{
			name:          "Resolve with Wrong Provider Prefix",
			identities:    []string{"p2:user1@example.com"},
			expectedCount: 0,
			expectedIDs:   []string{},
		},
		{
			name:          "Resolve Group",
			identities:    []string{"group1"},
			expectedCount: 1,
			expectedIDs:   []string{"group1"},
		},
		{
			name:          "Resolve by Email (diff from ID)",
			identities:    []string{"user4@example.com"},
			expectedCount: 1,
			expectedIDs:   []string{"user4-id"},
		},
		{
			name:          "Resolve Multiple",
			identities:    []string{"user1@example.com", "group1"},
			expectedCount: 2,
			expectedIDs:   []string{"user1@example.com", "group1"},
		},
		{
			name:          "Resolve Non-Existent",
			identities:    []string{"nonexistent"},
			expectedCount: 0,
			expectedIDs:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := models.ElevateRequest{
				Identities: tt.identities,
			}
			resolved := req.ResolveIdentities(context.Background(), providers)
			assert.Len(t, resolved, tt.expectedCount)

			var ids []string
			for _, id := range resolved {
				ids = append(ids, id.ID)
			}
			assert.ElementsMatch(t, tt.expectedIDs, ids)
		})
	}
}
