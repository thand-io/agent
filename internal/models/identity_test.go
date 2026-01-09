package models_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thand-io/agent/internal/models"
)

func TestIdentity_GetId(t *testing.T) {
	identity := models.Identity{ID: "test-id"}
	assert.Equal(t, "test-id", identity.GetId())
}

func TestIdentity_String(t *testing.T) {
	tests := []struct {
		name     string
		identity models.Identity
		expected string
	}{
		{
			name: "User Identity",
			identity: models.Identity{
				User: &models.User{Name: "John", Email: "john@example.com"},
			},
			expected: "John (john@example.com)",
		},
		{
			name: "Group Identity",
			identity: models.Identity{
				Group: &models.Group{Name: "Admins", Email: "admins@example.com"},
			},
			expected: "Admins (admins@example.com)",
		},
		{
			name:     "Empty Identity",
			identity: models.Identity{},
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
		identity models.Identity
		expected string
	}{
		{
			name: "User Email",
			identity: models.Identity{
				User: &models.User{Email: "john@example.com"},
			},
			expected: "john@example.com",
		},
		{
			name: "Group Email",
			identity: models.Identity{
				Group: &models.Group{Email: "admins@example.com"},
			},
			expected: "admins@example.com",
		},
		{
			name:     "No Email",
			identity: models.Identity{},
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
	user1 := &models.User{ID: "u1", Email: "u1@example.com"}
	user2 := &models.User{ID: "u2", Email: "u2@example.com"}
	group1 := &models.Group{ID: "g1", Name: "g1"}
	group2 := &models.Group{ID: "g2", Name: "g2"}

	tests := []struct {
		name     string
		id1      models.Identity
		id2      *models.Identity
		expected bool
	}{
		{
			name:     "Same User",
			id1:      models.Identity{User: user1},
			id2:      &models.Identity{User: user1},
			expected: true,
		},
		{
			name:     "Different User",
			id1:      models.Identity{User: user1},
			id2:      &models.Identity{User: user2},
			expected: false,
		},
		{
			name:     "Same Group",
			id1:      models.Identity{Group: group1},
			id2:      &models.Identity{Group: group1},
			expected: true,
		},
		{
			name:     "Different Group",
			id1:      models.Identity{Group: group1},
			id2:      &models.Identity{Group: group2},
			expected: false,
		},
		{
			name:     "User vs Group",
			id1:      models.Identity{User: user1},
			id2:      &models.Identity{Group: group1},
			expected: false,
		},
		{
			name:     "Nil Other",
			id1:      models.Identity{User: user1},
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
		identity models.Identity
		expected string
	}{
		{
			name: "User Email",
			identity: models.Identity{
				User: &models.User{Email: "John@Example.com"},
			},
			expected: "john@example.com",
		},
		{
			name: "User Username",
			identity: models.Identity{
				User: &models.User{Username: "JohnDoe"},
			},
			expected: "johndoe",
		},
		{
			name: "User ID",
			identity: models.Identity{
				ID:   "user-id",
				User: &models.User{},
			},
			expected: "user-id",
		},
		{
			name: "Group Email",
			identity: models.Identity{
				Group: &models.Group{Email: "Admins@Example.com"},
			},
			expected: "admins@example.com",
		},
		{
			name: "Group Name",
			identity: models.Identity{
				Group: &models.Group{Name: "Admins"},
			},
			expected: "admins",
		},
		{
			name: "Group ID",
			identity: models.Identity{
				ID:    "group-id",
				Group: &models.Group{},
			},
			expected: "group-id",
		},
		{
			name: "Identity ID only",
			identity: models.Identity{
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
	identity := models.Identity{Label: "Test Label"}
	assert.Equal(t, "Test Label", identity.GetLabel())
}

func TestIdentity_TypeChecks(t *testing.T) {
	userIdentity := models.Identity{User: &models.User{}}
	groupIdentity := models.Identity{Group: &models.Group{}}
	emptyIdentity := models.Identity{}

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
	identity := models.Identity{}
	assert.Nil(t, identity.GetProviders())

	provider := models.NewBaseProvider("aws-prod", models.ProviderConfig{
		Name:     "aws-prod",
		Provider: "aws",
	}, models.NewProviderCapabilities())

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
