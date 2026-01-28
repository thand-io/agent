package models

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
)

type Role struct {
	Version        *version.Version `json:"version,omitempty"`
	Identifier     string           `json:"-"`
	Name           string           `json:"name" validate:"required,min=1,max=100"`
	Description    string           `json:"description" validate:"max=500"`
	Authenticators []string         `json:"authenticators" validate:"dive,min=2,max=100"`            // All the auth providers that the role can use. If empty then any provider can be used
	Workflows      []string         `json:"workflows,omitempty" validate:"max=5,dive,min=1,max=100"` // The workflows to execute
	Inherits       []string         `json:"inherits,omitempty" validate:"max=50,dive,min=1,max=100"` // roles to inherit from or provider specific roles/policies etc
	Groups         Groups           `json:"groups"`                                                  // groups to add the user to
	Permissions    Permissions      `json:"permissions"`                                             // CSP-agnostic permission statements (replaces Permissions)
	Resources      Resources        `json:"resources"`                                               // resource access rules, apis, files, systems etc
	Scopes         *RoleScopes      `json:"scopes,omitempty"`                                        // scope of who can be assigned this role
	Providers      []string         `json:"providers" validate:"max=5,dive,min=2,max=100"`           // providers that can assign this role
	Enabled        bool             `json:"enabled" default:"true"`                                  // By default enable the role
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

func (r *Role) GetVersion() *version.Version {
	return r.Version
}

func (r *Role) GetIdentifier() string {
	return r.Identifier
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
	Allow []string `json:"allow,omitempty" validate:"max=100,dive,min=1,max=200"`
	Deny  []string `json:"deny,omitempty" validate:"max=100,dive,min=1,max=200"`
}

// Permissions defines permission-based access controls with allow and deny lists.
type Permissions struct {
	Allow Statements `json:"allow,omitempty" validate:"max=500,dive,min=1,max=500"`
	Deny  Statements `json:"deny,omitempty" validate:"max=500,dive,min=1,max=500"`
}

type Statements []Statement

// UnmarshalJSON provides backwards compatibility for Statements.
// It accepts both the old format (array of strings) and new format (array of Statement objects).
// When a string is encountered, it is converted to a Statement with the string as an Operation.
func (s *Statements) UnmarshalJSON(data []byte) error {
	// First, try to unmarshal as an array of raw messages
	var rawItems []json.RawMessage
	if err := json.Unmarshal(data, &rawItems); err != nil {
		return err
	}

	result := make(Statements, 0, len(rawItems))
	for _, raw := range rawItems {
		// Try to unmarshal as a string first (backwards compatibility)
		var str string
		if err := json.Unmarshal(raw, &str); err == nil {
			// It's a string, convert to Statement with the string as an operation
			result = append(result, Statement{
				Operations: []string{str},
				Targets:    []string{},
				Conditions: nil,
			})
			continue
		}

		// Try to unmarshal as a Statement object
		var stmt Statement
		if err := json.Unmarshal(raw, &stmt); err != nil {
			return fmt.Errorf("failed to unmarshal statement: %w", err)
		}
		result = append(result, stmt)
	}

	*s = result
	return nil
}

// Statement represents a CSP-agnostic permission statement
// Field names are provider-agnostic, but values are provider-specific
type Statement struct { // "allow" or "deny" (agnostic terminology)
	Operations []string       `json:"operations" validate:"required,min=1,max=500,dive,min=1,max=500"` // Provider-specific operations: ["s3:GetObject"] for AWS, ["storage.buckets.get"] for GCP
	Targets    []string       `json:"targets,omitempty" validate:"max=100,dive,min=1,max=1000"`        // Provider-specific resource identifiers
	Conditions map[string]any `json:"conditions,omitempty" validate:"max=10,dive,keys,min=1,max=100"`  // Optional provider-specific conditions
}

// RoleScopes defines the scope of a role in terms of users, groups, and domains (identities).
// Only the specified users, groups, or users belonging to the specified domains can be assigned this role.
// The Domains field allows restricting role assignment to users from particular domains (e.g., email domains or organizational domains),
// and can be used in conjunction with Groups and Users for more granular access control.
type RoleScopes struct {
	Groups  []string `json:"groups,omitempty" validate:"max=100,dive,min=1,max=200"`
	Users   []string `json:"users,omitempty" validate:"max=500,dive,min=1,max=320"`
	Domains []string `json:"domains,omitempty" validate:"max=50,dive,min=2,max=253,hostname"`
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
	Allow []string `json:"allow,omitempty" validate:"max=500,dive,min=1,max=1000"`
	Deny  []string `json:"deny,omitempty" validate:"max=500,dive,min=1,max=1000"`
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

// Validate validates all roles in the definition using struct validation tags
func (h *RoleDefinitions) Validate() error {
	validate := common.GetValidator()

	const (
		MaxInherits  = 50
		MaxProviders = 5
		MaxWorkflows = 5
	)

	for roleKey, role := range h.Roles {
		// Validate struct tags
		if err := validate.Struct(&role); err != nil {
			return fmt.Errorf("role '%s' validation failed: %w", roleKey, err)
		}

		// Additional business logic validations
		if len(role.Inherits) > MaxInherits {
			return fmt.Errorf("role '%s' exceeds maximum inherits limit (%d > %d)", roleKey, len(role.Inherits), MaxInherits)
		}
		if len(role.Providers) > MaxProviders {
			return fmt.Errorf("role '%s' exceeds maximum providers limit (%d > %d)", roleKey, len(role.Providers), MaxProviders)
		}
		if len(role.Workflows) > MaxWorkflows {
			return fmt.Errorf("role '%s' exceeds maximum workflows limit (%d > %d)", roleKey, len(role.Workflows), MaxWorkflows)
		}

	}

	return nil
}
