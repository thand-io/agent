package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thand-io/agent/internal/providers"
)

func TestIdentity_GetId(t *testing.T) {
	identity := Identity{ID: "test-id"}
	assert.Equal(t, "test-id", identity.GetId())
}

func TestIdentity_String(t *testing.T) {
	tests := []struct {
		name     string
		identity Identity
		expected string
	}{
		{
			name: "User Identity",
			identity: Identity{
				User: &User{Name: "John", Email: "john@example.com"},
			},
			expected: "John (john@example.com)",
		},
		{
			name: "Group Identity",
			identity: Identity{
				Group: &Group{Name: "Admins", Email: "admins@example.com"},
			},
			expected: "Admins (admins@example.com)",
		},
		{
			name:     "Empty Identity",
			identity: Identity{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.identity.String())
		})
	}
}

func TestIdentity_GetEmail(t *testing.T) {
	tests := []struct {
		name     string
		identity Identity
		expected string
	}{
		{
			name: "User Email",
			identity: Identity{
				User: &User{Email: "john@example.com"},
			},
			expected: "john@example.com",
		},
		{
			name: "Group Email",
			identity: Identity{
				Group: &Group{Email: "admins@example.com"},
			},
			expected: "admins@example.com",
		},
		{
			name:     "No Email",
			identity: Identity{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.identity.GetEmail())
		})
	}
}

func TestIdentity_Equals(t *testing.T) {
	user1 := &User{ID: "u1", Email: "u1@example.com"}
	user2 := &User{ID: "u2", Email: "u2@example.com"}
	group1 := &Group{ID: "g1", Name: "g1"}
	group2 := &Group{ID: "g2", Name: "g2"}

	tests := []struct {
		name     string
		id1      Identity
		id2      *Identity
		expected bool
	}{
		{
			name:     "Same User",
			id1:      Identity{User: user1},
			id2:      &Identity{User: user1},
			expected: true,
		},
		{
			name:     "Different User",
			id1:      Identity{User: user1},
			id2:      &Identity{User: user2},
			expected: false,
		},
		{
			name:     "Same Group",
			id1:      Identity{Group: group1},
			id2:      &Identity{Group: group1},
			expected: true,
		},
		{
			name:     "Different Group",
			id1:      Identity{Group: group1},
			id2:      &Identity{Group: group2},
			expected: false,
		},
		{
			name:     "User vs Group",
			id1:      Identity{User: user1},
			id2:      &Identity{Group: group1},
			expected: false,
		},
		{
			name:     "Nil Other",
			id1:      Identity{User: user1},
			id2:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.id1.Equals(tt.id2))
		})
	}
}

func TestIdentity_GetMappableIdentifier(t *testing.T) {
	tests := []struct {
		name     string
		identity Identity
		expected string
	}{
		{
			name: "User Email",
			identity: Identity{
				User: &User{Email: "John@Example.com"},
			},
			expected: "john@example.com",
		},
		{
			name: "User Username",
			identity: Identity{
				User: &User{Username: "JohnDoe"},
			},
			expected: "johndoe",
		},
		{
			name: "User ID",
			identity: Identity{
				ID:   "user-id",
				User: &User{},
			},
			expected: "user-id",
		},
		{
			name: "Group Email",
			identity: Identity{
				Group: &Group{Email: "Admins@Example.com"},
			},
			expected: "admins@example.com",
		},
		{
			name: "Group Name",
			identity: Identity{
				Group: &Group{Name: "Admins"},
			},
			expected: "admins",
		},
		{
			name: "Group ID",
			identity: Identity{
				ID:    "group-id",
				Group: &Group{},
			},
			expected: "group-id",
		},
		{
			name: "Identity ID only",
			identity: Identity{
				ID: "some-id",
			},
			expected: "some-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.identity.GetMappableIdentifier())
		})
	}
}

func TestIdentity_GetLabel(t *testing.T) {
	identity := Identity{Label: "Test Label"}
	assert.Equal(t, "Test Label", identity.GetLabel())
}

func TestIdentity_TypeChecks(t *testing.T) {
	userIdentity := Identity{User: &User{}}
	groupIdentity := Identity{Group: &Group{}}
	emptyIdentity := Identity{}

	assert.True(t, userIdentity.IsUser())
	assert.False(t, userIdentity.IsGroup())
	assert.NotNil(t, userIdentity.GetUser())
	assert.Nil(t, userIdentity.GetGroup())

	assert.False(t, groupIdentity.IsUser())
	assert.True(t, groupIdentity.IsGroup())
	assert.Nil(t, groupIdentity.GetUser())
	assert.NotNil(t, groupIdentity.GetGroup())

	assert.False(t, emptyIdentity.IsUser())
	assert.False(t, emptyIdentity.IsGroup())
}

func TestIdentity_Providers(t *testing.T) {
	identity := Identity{}
	assert.Nil(t, identity.GetProviders())

	provider := providers.NewMockProvider(ProviderConfig{
		Name:     "aws-prod",
		Provider: "aws",
	})

	identity.AddProvider(provider)
	providers := identity.GetProviders()
	assert.NotNil(t, providers)
	assert.Equal(t, "aws", providers["aws-prod"])

	// Add duplicate, should not change anything
	identity.AddProvider(provider)
	assert.Equal(t, 1, len(identity.GetProviders()))

	// Add nil provider
	identity.AddProvider(nil)
	assert.Equal(t, 1, len(identity.GetProviders()))
}
