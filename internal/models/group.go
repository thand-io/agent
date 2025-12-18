package models

import (
	"strings"
)

// Group represents a collection of users that share common access permissions.
// Groups are used to manage access control at scale by assigning permissions to
// groups rather than individual users.
type Group struct {
	// ID is the unique identifier for the group.
	ID string `json:"id,omitempty"`
	// Name is the human-readable name of the group.
	Name string `json:"name"`
	// Email is the email address associated with the group (e.g., a mailing list).
	Email string `json:"email"`
}

func (g *Group) String() string {
	if len(g.Name) > 0 && len(g.Email) > 0 {
		return g.Name + " (" + g.Email + ")"
	} else if len(g.Name) > 0 {
		return g.Name
	} else if len(g.Email) > 0 {
		return g.Email
	}
	return ""
}

func (g *Group) Equals(other *Group) bool {
	if other == nil {
		return false
	}

	// Try ID first
	if len(g.ID) > 0 && len(other.ID) > 0 {
		if strings.EqualFold(g.ID, other.ID) {
			return true
		}
	}

	// Try Name
	if len(g.Name) > 0 && len(other.Name) > 0 {
		if strings.EqualFold(g.Name, other.Name) {
			return true
		}
	}

	// Try Email
	if len(g.Email) > 0 && len(other.Email) > 0 {
		if strings.EqualFold(g.Email, other.Email) {
			return true
		}
	}

	return false
}

func (g *Group) GetID() string {
	return g.ID
}

func (g *Group) GetName() string {
	return g.Name
}

func (g *Group) GetEmail() string {
	return g.Email
}
