package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUser_String(t *testing.T) {
	tests := []struct {
		name     string
		user     User
		expected string
	}{
		{
			name: "Name and Email",
			user: User{
				Name:  "John Doe",
				Email: "john@example.com",
			},
			expected: "John Doe (john@example.com)",
		},
		{
			name: "Name and Username",
			user: User{
				Name:     "John Doe",
				Username: "johndoe",
			},
			expected: "John Doe (johndoe)",
		},
		{
			name: "Name only",
			user: User{
				Name: "John Doe",
			},
			expected: "John Doe",
		},
		{
			name: "Email only",
			user: User{
				Email: "john@example.com",
			},
			expected: "john@example.com",
		},
		{
			name: "Username only",
			user: User{
				Username: "johndoe",
			},
			expected: "johndoe",
		},
		{
			name:     "Empty",
			user:     User{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.user.String())
		})
	}
}

func TestUser_Equals(t *testing.T) {
	tests := []struct {
		name     string
		user1    User
		user2    *User
		expected bool
	}{
		{
			name: "Same ID",
			user1: User{
				ID: "123",
			},
			user2: &User{
				ID: "123",
			},
			expected: true,
		},
		{
			name: "Different ID",
			user1: User{
				ID: "123",
			},
			user2: &User{
				ID: "456",
			},
			expected: false,
		},
		{
			name: "Same Email",
			user1: User{
				Email: "john@example.com",
			},
			user2: &User{
				Email: "john@example.com",
			},
			expected: true,
		},
		{
			name: "Same Email Case Insensitive",
			user1: User{
				Email: "john@example.com",
			},
			user2: &User{
				Email: "JOHN@EXAMPLE.COM",
			},
			expected: true,
		},
		{
			name: "Different Email",
			user1: User{
				Email: "john@example.com",
			},
			user2: &User{
				Email: "jane@example.com",
			},
			expected: false,
		},
		{
			name: "Same Username",
			user1: User{
				Username: "johndoe",
			},
			user2: &User{
				Username: "johndoe",
			},
			expected: true,
		},
		{
			name: "Same Username Case Insensitive",
			user1: User{
				Username: "johndoe",
			},
			user2: &User{
				Username: "JOHNDOE",
			},
			expected: true,
		},
		{
			name: "Different Username",
			user1: User{
				Username: "johndoe",
			},
			user2: &User{
				Username: "janedoe",
			},
			expected: false,
		},
		{
			name: "ID and Email vs Email only",
			user1: User{
				ID:    "123",
				Email: "john@example.com",
			},
			user2: &User{
				Email: "john@example.com",
			},
			expected: true,
		},
		{
			name: "Different IDs but same Email",
			user1: User{
				ID:    "123",
				Email: "john@example.com",
			},
			user2: &User{
				ID:    "456",
				Email: "john@example.com",
			},
			expected: true,
		},
		{
			name: "Different IDs but same Username",
			user1: User{
				ID:       "123",
				Username: "johndoe",
			},
			user2: &User{
				ID:       "456",
				Username: "johndoe",
			},
			expected: true,
		},
		{
			name: "Different IDs and Emails but same Username",
			user1: User{
				ID:       "123",
				Email:    "john@example.com",
				Username: "johndoe",
			},
			user2: &User{
				ID:       "456",
				Email:    "jane@example.com",
				Username: "johndoe",
			},
			expected: true,
		},
		{
			name: "All different fields",
			user1: User{
				ID:       "123",
				Email:    "john@example.com",
				Username: "johndoe",
			},
			user2: &User{
				ID:       "456",
				Email:    "jane@example.com",
				Username: "janedoe",
			},
			expected: false,
		},
		{
			name:     "Nil Other",
			user1:    User{},
			user2:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.user1.Equals(tt.user2))
		})
	}
}

func TestUser_GetName(t *testing.T) {
	tests := []struct {
		name     string
		user     User
		expected string
	}{
		{
			name: "Name present",
			user: User{
				Name: "John Doe",
			},
			expected: "John Doe",
		},
		{
			name: "Username present",
			user: User{
				Username: "johndoe",
			},
			expected: "johndoe",
		},
		{
			name: "Email present",
			user: User{
				Email: "john@example.com",
			},
			expected: "john@example.com",
		},
		{
			name:     "Empty",
			user:     User{},
			expected: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.user.GetName())
		})
	}
}

func TestUser_GetUsername(t *testing.T) {
	tests := []struct {
		name     string
		user     User
		expected string
	}{
		{
			name: "Username present",
			user: User{
				Username: "johndoe",
			},
			expected: "johndoe",
		},
		{
			name: "Name present",
			user: User{
				Name: "John Doe",
			},
			expected: "john_doe",
		},
		{
			name: "Email present",
			user: User{
				Email: "john@example.com",
			},
			expected: "john",
		},
		{
			name: "ID present",
			user: User{
				ID: "123",
			},
			expected: "123",
		},
		{
			name:     "Empty",
			user:     User{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.user.GetUsername())
		})
	}
}

func TestUser_GetIdentity(t *testing.T) {
	tests := []struct {
		name     string
		user     User
		expected string
	}{
		{
			name: "Email present",
			user: User{
				Email: "john@example.com",
			},
			expected: "john@example.com",
		},
		{
			name: "Username present",
			user: User{
				Username: "johndoe",
			},
			expected: "johndoe",
		},
		{
			name: "ID present",
			user: User{
				ID: "123",
			},
			expected: "123",
		},
		{
			name: "Name present",
			user: User{
				Name: "John Doe",
			},
			expected: "john_doe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.user.GetMappableIdentifier())
		})
	}
}

func TestUser_GetFirstName(t *testing.T) {
	tests := []struct {
		name     string
		user     User
		expected string
	}{
		{
			name: "Full Name",
			user: User{
				Name: "John Doe",
			},
			expected: "John",
		},
		{
			name: "Single Name",
			user: User{
				Name: "John",
			},
			expected: "John",
		},
		{
			name:     "Empty",
			user:     User{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.user.GetFirstName())
		})
	}
}

func TestUser_GetLastName(t *testing.T) {
	tests := []struct {
		name     string
		user     User
		expected string
	}{
		{
			name: "Full Name",
			user: User{
				Name: "John Doe",
			},
			expected: "Doe",
		},
		{
			name: "Single Name",
			user: User{
				Name: "John",
			},
			expected: "",
		},
		{
			name: "Three Names",
			user: User{
				Name: "John Middle Doe",
			},
			expected: "Doe",
		},
		{
			name:     "Empty",
			user:     User{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.user.GetLastName())
		})
	}
}

func TestUser_GetGroups(t *testing.T) {
	groups := []string{"admin", "user"}
	user := User{
		Groups: groups,
	}
	assert.Equal(t, groups, user.GetGroups())
}

func TestUser_GetDomain(t *testing.T) {
	tests := []struct {
		name     string
		user     User
		expected string
	}{
		{
			name: "Valid Email",
			user: User{
				Email: "john@example.com",
			},
			expected: "example.com",
		},
		{
			name: "Invalid Email",
			user: User{
				Email: "johnexample.com",
			},
			expected: "",
		},
		{
			name: "Empty Email",
			user: User{
				Email: "",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.user.GetDomain())
		})
	}
}

func TestUser_AsMap(t *testing.T) {
	user := User{
		ID:       "123",
		Username: "johndoe",
		Email:    "john@example.com",
		Name:     "John Doe",
	}

	m := user.AsMap()
	assert.NotNil(t, m)
	assert.Equal(t, "123", m["id"])
	assert.Equal(t, "johndoe", m["username"])
	assert.Equal(t, "john@example.com", m["email"])
	assert.Equal(t, "John Doe", m["name"])
}
