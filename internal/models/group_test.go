package models_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thand-io/agent/internal/models"
)

func TestGroup_String(t *testing.T) {
	tests := []struct {
		name     string
		group    models.Group
		expected string
	}{
		{
			name: "Name and Email",
			group: models.Group{
				Name:  "Admins",
				Email: "admins@example.com",
			},
			expected: "Admins (admins@example.com)",
		},
		{
			name: "Name only",
			group: models.Group{
				Name: "Admins",
			},
			expected: "Admins",
		},
		{
			name: "Email only",
			group: models.Group{
				Email: "admins@example.com",
			},
			expected: "admins@example.com",
		},
		{
			name:     "Empty",
			group:    models.Group{},
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
		group1   models.Group
		group2   *models.Group
		expected bool
	}{
		{
			name: "Same ID",
			group1: models.Group{
				ID: "123",
			},
			group2: &models.Group{
				ID: "123",
			},
			expected: true,
		},
		{
			name: "Different ID",
			group1: models.Group{
				ID: "123",
			},
			group2: &models.Group{
				ID: "456",
			},
			expected: false,
		},
		{
			name: "Same Name",
			group1: models.Group{
				Name: "Admins",
			},
			group2: &models.Group{
				Name: "Admins",
			},
			expected: true,
		},
		{
			name: "Same Name Case Insensitive",
			group1: models.Group{
				Name: "Admins",
			},
			group2: &models.Group{
				Name: "ADMINS",
			},
			expected: true,
		},
		{
			name: "Different Name",
			group1: models.Group{
				Name: "Admins",
			},
			group2: &models.Group{
				Name: "Users",
			},
			expected: false,
		},
		{
			name: "Same Email",
			group1: models.Group{
				Email: "admins@example.com",
			},
			group2: &models.Group{
				Email: "admins@example.com",
			},
			expected: true,
		},
		{
			name: "Same Email Case Insensitive",
			group1: models.Group{
				Email: "admins@example.com",
			},
			group2: &models.Group{
				Email: "ADMINS@EXAMPLE.COM",
			},
			expected: true,
		},
		{
			name: "Different Email",
			group1: models.Group{
				Email: "admins@example.com",
			},
			group2: &models.Group{
				Email: "users@example.com",
			},
			expected: false,
		},
		{
			name: "ID and Email vs Email only",
			group1: models.Group{
				ID:    "123",
				Email: "admins@example.com",
			},
			group2: &models.Group{
				Email: "admins@example.com",
			},
			expected: true,
		},
		{
			name: "Different IDs but same Name",
			group1: models.Group{
				ID:   "123",
				Name: "Admins",
			},
			group2: &models.Group{
				ID:   "456",
				Name: "Admins",
			},
			expected: true,
		},
		{
			name: "Different IDs but same Email",
			group1: models.Group{
				ID:    "123",
				Email: "admins@example.com",
			},
			group2: &models.Group{
				ID:    "456",
				Email: "admins@example.com",
			},
			expected: true,
		},
		{
			name: "Different IDs and Names but same Email",
			group1: models.Group{
				ID:    "123",
				Name:  "Admins",
				Email: "admins@example.com",
			},
			group2: &models.Group{
				ID:    "456",
				Name:  "Users",
				Email: "admins@example.com",
			},
			expected: true,
		},
		{
			name: "All different fields",
			group1: models.Group{
				ID:    "123",
				Name:  "Admins",
				Email: "admins@example.com",
			},
			group2: &models.Group{
				ID:    "456",
				Name:  "Users",
				Email: "users@example.com",
			},
			expected: false,
		},
		{
			name:     "Nil Other",
			group1:   models.Group{},
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
	group := models.Group{ID: "123"}
	assert.Equal(t, "123", group.GetID())
}

func TestGroup_GetName(t *testing.T) {
	group := models.Group{Name: "Admins"}
	assert.Equal(t, "Admins", group.GetName())
}

func TestGroup_GetEmail(t *testing.T) {
	group := models.Group{Email: "admins@example.com"}
	assert.Equal(t, "admins@example.com", group.GetEmail())
}
