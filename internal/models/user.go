package models

import (
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
)

type User struct {
	ID       string   `json:"id,omitempty"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Name     string   `json:"name"`
	Verified *bool    `json:"verified,omitempty"`
	Source   string   `json:"source,omitempty"`
	Groups   []string `json:"groups,omitempty"`
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

func (u *User) AsMap() map[string]any {
	// Convert User struct to a map[string]any
	var mapUser map[string]any
	err := common.ConvertInterfaceToInterface(u, &mapUser)
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
