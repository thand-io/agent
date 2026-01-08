package models

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func NewMockProvider(name string, identities []Identity) Provider {
	provider := ProviderConfig{
		Name:        name,
		Description: "Mock Provider",
		Provider:    "mock",
		Enabled:     true,
	}

	base := NewBaseProvider(name, provider,
		NewProviderCapabilities().
			WithDefaultIdentitiesConfiguration())
	base.SetIdentities(identities)

	return base
}

func TestResolveIdentities(t *testing.T) {
	// Define identities
	id1 := Identity{
		ID:    "user1@example.com",
		Label: "User One",
		User: &User{
			Email: "user1@example.com",
			Name:  "User One",
		},
	}
	id2 := Identity{
		ID:    "user2",
		Label: "User Two",
		User: &User{
			Email: "user2@example.com",
			Name:  "User Two",
		},
	}
	id3 := Identity{
		ID:    "group1",
		Label: "Group One",
		Group: &Group{
			Name: "Group One",
		},
	}
	id4 := Identity{
		ID:    "user4-id",
		Label: "User Four",
		User: &User{
			Email: "user4@example.com",
			Name:  "User Four",
		},
	}

	// Create providers
	mp1 := NewMockProvider("p1", []Identity{id1, id2})
	mp2 := NewMockProvider("p2", []Identity{id3, id4})

	providers := map[string]Provider{
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
			req := ElevateRequest{
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
