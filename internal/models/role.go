package models

import (
	"encoding/json"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
)

type Role struct {
	Version        *version.Version `json:"version,omitempty"`
	Name           string           `json:"name"`
	Description    string           `json:"description"`
	Authenticators []string         `json:"authenticators"`         // All the auth providers that the role can use. If empty then any provider can be used
	Workflows      []string         `json:"workflows,omitempty"`    // The workflows to execute
	Inherits       []string         `json:"inherits,omitempty"`     // roles to inherit from or provider specific roles/policies etc
	Groups         Groups           `json:"groups,omitempty"`       // groups to add the user to
	Permissions    Permissions      `json:"permissions,omitempty"`  // granular permissions for the role
	Resources      Resources        `json:"resources,omitempty"`    // resource access rules, apis, files, systems etc
	Scopes         *RoleScopes      `json:"scopes,omitempty"`       // scope of who can be assigned this role
	Providers      []string         `json:"providers"`              // providers that can assign this role
	Enabled        bool             `json:"enabled" default:"true"` // By default enable the role
}

func (r *Role) HasPermission(user *User) bool {

	if user == nil {
		logrus.Debugln("Role.HasPermission: user is nil")
		return false
	}

	if r.Scopes == nil {
		logrus.Debugln("Role.HasPermission: no scopes defined, allowing access")
		return true
	}

	// Check user scopes (case-insensitive)
	if len(r.Scopes.Users) > 0 {
		for _, allowedUser := range r.Scopes.Users {
			if strings.EqualFold(allowedUser, user.Username) ||
				strings.EqualFold(allowedUser, user.ID) ||
				strings.EqualFold(allowedUser, user.Email) {
				return true
			}
		}
	}

	// Check group scopes (case-insensitive)
	if len(r.Scopes.Groups) > 0 {
		for _, userGroup := range user.Groups {
			for _, allowedGroup := range r.Scopes.Groups {
				if strings.EqualFold(userGroup, allowedGroup) {
					return true
				}
			}
		}
	}

	// Check domain scopes (case-insensitive)
	if len(r.Scopes.Domains) > 0 {
		userDomain := user.GetDomain()
		for _, allowedDomain := range r.Scopes.Domains {
			if strings.EqualFold(userDomain, allowedDomain) {
				return true
			}
		}
	}

	// If scopes are defined but no match found, role doesn't apply
	if len(r.Scopes.Users) > 0 || len(r.Scopes.Groups) > 0 || len(r.Scopes.Domains) > 0 {
		return false
	}

	// No scopes defined means open to all users
	return true
}

func (r *Role) AsMap() map[string]any {

	role, err := common.ConvertInterfaceToMap(r)
	if err != nil {
		logrus.WithError(err).Errorln("Failed to convert role to map")
		return nil
	}
	return role

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

// Groups defines group-based access controls with allow and deny lists.
type Groups struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// Permissions defines permission-based access controls with allow and deny lists.
type Permissions struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// RoleScopes defines the scope of a role in terms of users, groups, and domains (identities).
// Only the specified users, groups, or users belonging to the specified domains can be assigned this role.
// The Domains field allows restricting role assignment to users from particular domains (e.g., email domains or organizational domains),
// and can be used in conjunction with Groups and Users for more granular access control.
type RoleScopes struct {
	Groups  []string `json:"groups,omitempty"`
	Users   []string `json:"users,omitempty"`
	Domains []string `json:"domains,omitempty"`
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

// RoleDefinitions represents the structure for roles YAML/JSON
type RoleDefinitions struct {
	Version *version.Version `yaml:"version" json:"version"`
	Roles   map[string]Role  `yaml:"roles" json:"roles"`
}

// UnmarshalJSON converts Version to string from any type
func (h *RoleDefinitions) UnmarshalJSON(data []byte) error {
	aux := &struct {
		Version any             `json:"version"`
		Roles   map[string]Role `json:"roles"`
	}{
		Roles: make(map[string]Role),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	parsedVersion, err := version.NewVersion(ConvertVersionToString(aux.Version))
	if err != nil {
		return err
	}

	h.Version = parsedVersion
	h.Roles = aux.Roles

	return nil
}

// UnmarshalYAML converts Version to string from any type
func (h *RoleDefinitions) UnmarshalYAML(unmarshal func(any) error) error {
	aux := &struct {
		Version any             `yaml:"version"`
		Roles   map[string]Role `yaml:"roles"`
	}{
		Roles: make(map[string]Role),
	}

	if err := unmarshal(&aux); err != nil {
		return err
	}

	parsedVersion, err := version.NewVersion(ConvertVersionToString(aux.Version))
	if err != nil {
		return err
	}

	h.Version = parsedVersion
	h.Roles = aux.Roles

	return nil
}
