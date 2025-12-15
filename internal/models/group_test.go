package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGroup_String(t *testing.T) {
	tests := []struct {
		name     string
		group    Group
		expected string
	}{
		{
			name: "Name and Email",
			group: Group{
				Name:  "Admins",
				Email: "admins@example.com",
			},
			expected: "Admins (admins@example.com)",
		},
		{
			name: "Name only",
			group: Group{
				Name: "Admins",
			},
			expected: "Admins",
		},
		{
			name: "Email only",
			group: Group{
				Email: "admins@example.com",
			},
			expected: "admins@example.com",
		},
		{
			name:     "Empty",
			group:    Group{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.group.String())
		})
	}
}

func TestGroup_Equals(t *testing.T) {
	tests := []struct {
		name     string
		group1   Group
		group2   *Group
		expected bool
	}{
		{
			name: "Same ID",
			group1: Group{
				ID: "123",
			},
			group2: &Group{
				ID: "123",
			},
			expected: true,
		},
		{
			name: "Different ID",
			group1: Group{
				ID: "123",
			},
			group2: &Group{
				ID: "456",
			},
			expected: false,
		},
		{
			name: "Same Name",
			group1: Group{
				Name: "Admins",
			},
			group2: &Group{
				Name: "Admins",
			},
			expected: true,
		},
		{
			name: "Same Name Case Insensitive",
			group1: Group{
				Name: "Admins",
			},
			group2: &Group{
				Name: "ADMINS",
			},
			expected: true,
		},
		{
			name: "Different Name",
			group1: Group{
				Name: "Admins",
			},
			group2: &Group{
				Name: "Users",
			},
			expected: false,
		},
		{
			name: "Same Email",
			group1: Group{
				Email: "admins@example.com",
			},
			group2: &Group{
				Email: "admins@example.com",
			},
			expected: true,
		},
		{
			name: "Same Email Case Insensitive",
			group1: Group{
				Email: "admins@example.com",
			},
			group2: &Group{
				Email: "ADMINS@EXAMPLE.COM",
			},
			expected: true,
		},
		{
			name: "Different Email",
			group1: Group{
				Email: "admins@example.com",
			},
			group2: &Group{
				Email: "users@example.com",
			},
			expected: false,
		},
		{
			name:     "Nil Other",
			group1:   Group{},
			group2:   nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.group1.Equals(tt.group2))
		})
	}
}

func TestGroup_GetID(t *testing.T) {
	group := Group{ID: "123"}
	assert.Equal(t, "123", group.GetID())
}

func TestGroup_GetName(t *testing.T) {
	group := Group{Name: "Admins"}
	assert.Equal(t, "Admins", group.GetName())
}

func TestGroup_GetEmail(t *testing.T) {
	group := Group{Email: "admins@example.com"}
	assert.Equal(t, "admins@example.com", group.GetEmail())
}
