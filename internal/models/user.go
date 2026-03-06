package models

import (
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
)

// User represents an individual user in the system.
// Users can be authenticated through various identity providers and may belong
// to one or more groups for access control purposes.
type User struct {
	// ID is the unique identifier for the user.
	ID string `json:"id,omitempty"`
	// Username is the user's login name or handle.
	Username string `json:"username"`
	// Email is the user's email address, often used as a primary identifier.
	Email string `json:"email"`
	// Name is the user's full display name.
	Name string `json:"name"`
	// Verified indicates whether the user's identity has been verified.
	Verified *bool `json:"verified,omitempty"`
	// Source identifies the identity provider or system where this user originated.
	Source string `json:"source,omitempty"`
	// Groups is a list of group names or IDs that this user belongs to.
	Groups []string `json:"groups,omitempty"`
}

func (u *User) String() string {

	if len(u.Name) > 0 && len(u.Email) > 0 {
		return u.Name + " (" + u.Email + ")"
	} else if len(u.Name) > 0 && len(u.Username) > 0 {
		return u.Name + " (" + u.Username + ")"
	} else if len(u.Name) > 0 {
		return u.Name
	} else if len(u.Email) > 0 {
		return u.Email
	} else if len(u.Username) > 0 {
		return u.Username
	}

	return ""
}

func (u *User) Equals(other *User) bool {
	if other == nil {
		return false
	}

	// Try ID first
	if len(u.ID) > 0 && len(other.ID) > 0 {
		if strings.EqualFold(u.ID, other.ID) {
			return true
		}
	}

	// Try Email
	if len(u.Email) > 0 && len(other.Email) > 0 {
		if strings.EqualFold(u.Email, other.Email) {
			return true
		}
	}

	// Try Username
	if len(u.Username) > 0 && len(other.Username) > 0 {
		if strings.EqualFold(u.Username, other.Username) {
			return true
		}
	}

	return false
}

func (u *User) GetName() string {
	if len(u.Name) > 0 {
		return u.Name
	} else if len(u.Username) > 0 {
		return u.Username
	} else if len(u.Email) > 0 {
		return u.Email
	}
	return "Unknown"
}

func (u *User) GetUsername() string {
	if len(u.Username) > 0 {
		return u.Username
	} else if len(u.Name) > 0 {
		return common.ConvertToSnakeCase(u.Name)
	} else if len(u.Email) > 0 {
		// Use the part before @ in email
		return common.ConvertToSnakeCase(u.Email[:strings.Index(u.Email, "@")])
	} else if len(u.ID) > 0 {
		return common.ConvertToSnakeCase(u.ID)
	}

	return ""
}

func (u *User) GetMappableIdentifier() string {
	if len(u.Email) > 0 {
		return u.Email
	} else if len(u.Username) > 0 {
		return u.Username
	} else if len(u.ID) > 0 {
		return u.ID
	}
	return common.ConvertToSnakeCase(u.Name)
}

func (u *User) GetFirstName() string {
	if len(u.Name) > 0 {
		parts := strings.Split(u.Name, " ")
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return ""
}

func (u *User) GetLastName() string {
	if len(u.Name) > 0 {
		parts := strings.Split(u.Name, " ")
		if len(parts) > 1 {
			return parts[len(parts)-1]
		}
	}
	return ""
}

func (u *User) GetGroups() []string {
	return u.Groups
}

func (u *User) GetDomain() string {
	if len(u.Email) > 0 {
		atIdx := strings.LastIndex(u.Email, "@")
		if atIdx != -1 && atIdx < len(u.Email)-1 {
			return u.Email[atIdx+1:]
		}
	}
	return ""
}

func (u *User) AsIdentity() Identity {
	return Identity{
		ID:    u.ID,
		Label: u.GetName(),
		User:  u,
	}
}

func (u *User) AsMap() map[string]any {
	// Convert User struct to a map[string]any
	mapUser, err := common.ConvertInterfaceToMap(u)
	if err != nil {

		logrus.WithError(err).Error("Failed to convert User struct to map")
		return nil

	}
	return mapUser
}

type AuthorizeUser struct {
	Scopes      []string `json:"scopes"`
	State       string   `json:"state"`
	RedirectUri string   `json:"redirect_uri"`
	Code        string   `json:"code"`
}
