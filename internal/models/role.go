package models

import (
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
)

type Role struct {
	Name           string       `json:"name"`
	Description    string       `json:"description"`
	Authenticators []string     `json:"authenticators"`      // All the auth providers that the role can use. If empty then any provider can be used
	Workflows      []string     `json:"workflows,omitempty"` // The workflows to execute
	Inherits       []string     `json:"inherits,omitempty"`
	Permissions    Permissions  `json:"permissions,omitempty"`
	Resources      Resources    `json:"resources,omitempty"`
	Applies        *RoleApplies `json:"applies,omitempty"`
	Providers      []string     `json:"providers"`
	Enabled        bool         `json:"enabled" default:"true"` // By default enable the role
}

func (r *Role) HasPermission(user *User) bool {

	if user == nil {
		logrus.Debugln("Role.HasPermission: user is nil")
		return false
	}

	return true
}

func (r *Role) IsValid() bool {
	return len(r.Name) > 0 && len(r.Description) > 0
}

func (r *Role) GetName() string {
	return r.Name
}

func (r *Role) GetSnakeCaseName() string {
	return common.ConvertToSnakeCase(r.Name)
}

func (r *Role) GetDescription() string {
	return r.Description
}

type Permissions struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

type RoleApplies struct {
	Groups []string `json:"groups,omitempty"`
	Users  []string `json:"users,omitempty"`
}

// RolesResponse represents the response for /roles endpoint
type RolesResponse struct {
	Version string                  `json:"version"`
	Roles   map[string]RoleResponse `json:"roles"`
}

type RoleResponse struct {
	Role
}

type Resources struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}
