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
	Version     *version.Version `json:"version,omitempty"`
	Identifier  string           `json:"identifier"` // To be set by the system
	Name        string           `json:"name" validate:"required,min=1,max=100"`
	Description string           `json:"description" validate:"max=500"`

	Authenticators []string `json:"authenticators" validate:"dive,min=2,max=100"`            // All the auth providers that the role can use. If empty then any provider can be used
	Workflows      []string `json:"workflows,omitempty" validate:"max=5,dive,min=1,max=100"` // The workflows to execute
	Providers      []string `json:"providers" validate:"max=5,dive,min=2,max=100"`           // providers that can assign this role

	Inherits    []string        `json:"inherits,omitempty" validate:"max=50,dive,min=1,max=100"` // roles to inherit from or provider specific roles/policies etc
	Permissions RolePermissions `json:"permissions"`                                             // CSP-agnostic permission statements

	Scopes RoleScopes `json:"scopes"` // scope of who can be assigned this role

	Composite bool `json:"composite" default:"false"` // Whether this role is a composite role (i.e., aggregates other roles)
	Enabled   bool `json:"enabled" default:"true"`    // By default enable the role
}

// UnmarshalJSON provides backwards compatibility for Role.
// It handles deprecated Groups and Resources fields by:
// 1. Populating the deprecated Groups and Resources fields for existing code compatibility
// 2. Migrating them to Permissions Targets for the new approach
func (r *Role) UnmarshalJSON(data []byte) error {
	// Use an alias to avoid infinite recursion
	type RoleAlias Role

	// Define a struct that includes deprecated fields
	aux := &struct {
		*RoleAlias
		// Deprecated fields for backwards compatibility
		Groups    *RoleGroups    `json:"groups,omitempty"`
		Resources *RoleResources `json:"resources,omitempty"`
	}{
		RoleAlias: (*RoleAlias)(r),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to unmarshal role: %w", err)
	}

	if aux == nil {
		logrus.Debugln("Role.UnmarshalJSON: aux is nil")
		return fmt.Errorf("failed to unmarshal role: aux is nil")
	}

	// Migrate deprecated Groups and Resources to Permissions Targets
	// Collect all targets from deprecated fields
	var allowTargets, denyTargets []string

	// Add Groups.Allow and Resources.Allow to allowTargets
	if aux.Groups != nil && aux.Groups.Allow != nil {
		allowTargets = append(allowTargets, aux.Groups.Allow...)
	}
	if aux.Resources != nil && aux.Resources.Allow != nil {
		allowTargets = append(allowTargets, aux.Resources.Allow...)
	}

	// Add Groups.Deny and Resources.Deny to denyTargets
	if aux.Groups != nil && aux.Groups.Deny != nil {
		denyTargets = append(denyTargets, aux.Groups.Deny...)
	}
	if aux.Resources != nil && aux.Resources.Deny != nil {
		denyTargets = append(denyTargets, aux.Resources.Deny...)
	}

	// Migrate deprecated Groups and Resources targets to Permission statements
	if len(allowTargets) > 0 {
		r.Permissions.Allow = append(r.Permissions.Allow, Statement{
			Operations: []string{},
			Targets:    allowTargets,
		})
	}

	if len(denyTargets) > 0 {
		r.Permissions.Deny = append(r.Permissions.Deny, Statement{
			Operations: []string{},
			Targets:    denyTargets,
		})
	}

	return nil
}

func (r *Role) HasPermission(user *User) bool {

	if user == nil {
		logrus.Debugln("Role.HasPermission: user is nil")
		return false
	}

	// Check if any scopes are defined
	if r.Scopes.IsEmpty() {
		logrus.Debugln("Role.HasPermission: no scopes defined, allowing access")
		return true
	}

	userDomain := user.GetDomain()

	// Check deny lists first (deny takes precedence)
	// Check user deny list
	for _, deniedUser := range r.Scopes.Deny.Users {
		if strings.EqualFold(deniedUser, user.Username) ||
			strings.EqualFold(deniedUser, user.ID) ||
			strings.EqualFold(deniedUser, user.Email) {
			logrus.Debugln("Role.HasPermission: user explicitly denied")
			return false
		}
	}

	// Check group deny list
	for _, userGroup := range user.Groups {
		for _, deniedGroup := range r.Scopes.Deny.Groups {
			if strings.EqualFold(userGroup, deniedGroup) {
				logrus.Debugln("Role.HasPermission: user's group explicitly denied")
				return false
			}
		}
	}

	// Check domain deny list
	for _, deniedDomain := range r.Scopes.Deny.Domains {
		if strings.EqualFold(userDomain, deniedDomain) {
			logrus.Debugln("Role.HasPermission: user's domain explicitly denied")
			return false
		}
	}

	// Check allow lists
	// If no allow lists are defined, user passes (only deny lists matter)
	if r.Scopes.Allow.IsEmpty() {
		logrus.Debugln("Role.HasPermission: no allow scopes, user not denied, allowing access")
		return true
	}

	// Check user allow list (case-insensitive)
	for _, allowedUser := range r.Scopes.Allow.Users {
		if strings.EqualFold(allowedUser, user.Username) ||
			strings.EqualFold(allowedUser, user.ID) ||
			strings.EqualFold(allowedUser, user.Email) {
			return true
		}
	}

	// Check group allow list (case-insensitive)
	for _, userGroup := range user.Groups {
		for _, allowedGroup := range r.Scopes.Allow.Groups {
			if strings.EqualFold(userGroup, allowedGroup) {
				return true
			}
		}
	}

	// Check domain allow list (case-insensitive)
	for _, allowedDomain := range r.Scopes.Allow.Domains {
		if strings.EqualFold(userDomain, allowedDomain) {
			return true
		}
	}

	// Allow lists exist but user didn't match any
	return false
}

func (r *Role) Validate() error {
	const (
		MaxInherits  = 50
		MaxProviders = 5
		MaxWorkflows = 5
	)

	validate := common.GetValidator()
	// Validate struct tags
	if err := validate.Struct(r); err != nil {
		return fmt.Errorf("role '%s' validation failed: %w", r.GetName(), err)
	}

	// Additional business logic validations
	if len(r.Inherits) > MaxInherits {
		return fmt.Errorf("role '%s' exceeds maximum inherits limit (%d > %d)", r.GetName(), len(r.Inherits), MaxInherits)
	}
	if len(r.Providers) > MaxProviders {
		return fmt.Errorf("role '%s' exceeds maximum providers limit (%d > %d)", r.GetName(), len(r.Providers), MaxProviders)
	}
	if len(r.Workflows) > MaxWorkflows {
		return fmt.Errorf("role '%s' exceeds maximum workflows limit (%d > %d)", r.GetName(), len(r.Workflows), MaxWorkflows)
	}

	return nil
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

func (r *Role) GetDescription() string {
	return r.Description
}

// Groups defines group-based access controls with allow and deny lists.
type RoleGroups struct {
	Allow []string `json:"allow,omitempty" validate:"max=100,dive,min=1,max=200"`
	Deny  []string `json:"deny,omitempty" validate:"max=100,dive,min=1,max=200"`
}

// Permissions defines permission-based access controls with allow and deny lists.
type RolePermissions struct {
	Allow RoleStatements `json:"allow,omitempty" validate:"max=500,dive"`
	Deny  RoleStatements `json:"deny,omitempty" validate:"max=500,dive"`
}

type RoleStatements []Statement

// UnmarshalJSON provides backwards compatibility for RoleStatements.
// It accepts both the old format (array of strings) and new format (array of Statement objects).
// When a string is encountered, it is converted to a Statement with the string as an Operation.
func (s *RoleStatements) UnmarshalJSON(data []byte) error {
	// First, try to unmarshal as an array of raw messages
	var rawItems []json.RawMessage
	if err := json.Unmarshal(data, &rawItems); err != nil {
		return err
	}

	result := make(RoleStatements, 0, len(rawItems))
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

// Statement represents a CSP-agnostic permission statement.
// Field names are provider-agnostic, but values are provider-specific.
//
// IMPORTANT: The Conditions field is preserved for provider-specific use but is
// NOT evaluated during permission resolution by this system. Condition enforcement
// is delegated to the target cloud provider (AWS IAM, GCP IAM, Azure RBAC, etc.).
// This design allows passing through provider-native conditions without requiring
// this system to understand every provider's condition syntax.
type Statement struct {
	// Operations contains provider-specific actions/permissions.
	// Examples: ["s3:GetObject", "s3:PutObject"] for AWS, ["storage.buckets.get"] for GCP
	Operations []string `json:"operations" validate:"max=500,dive,min=1,max=500"`

	// Targets contains provider-specific resource identifiers.
	// Examples: ["arn:aws:s3:::my-bucket/*"] for AWS, ["projects/my-project/buckets/my-bucket"] for GCP
	Targets []string `json:"targets,omitempty" validate:"max=100,dive,min=1,max=1000"`

	// Conditions contains optional provider-specific conditions.
	// WARNING: Conditions are PRESERVED but NOT EVALUATED by this system.
	// Enforcement is delegated to the target provider's IAM system.
	// Examples: {"IpAddress": {"aws:SourceIp": "10.0.0.0/8"}} for AWS
	Conditions map[string]any `json:"conditions,omitempty" validate:"max=10,dive,keys,min=1,max=100"`
}

// ScopeIdentities defines identity-based restrictions for users, groups, and domains.
type ScopeIdentities struct {
	Users   []string `json:"users,omitempty" validate:"max=500,dive,min=1,max=320"`
	Groups  []string `json:"groups,omitempty" validate:"max=100,dive,min=1,max=200"`
	Domains []string `json:"domains,omitempty" validate:"max=50,dive,min=2,max=253,hostname"`
}

// IsEmpty returns true if all identity lists are empty
func (s *ScopeIdentities) IsEmpty() bool {
	return len(s.Users) == 0 && len(s.Groups) == 0 && len(s.Domains) == 0
}

// RoleScopes defines the scope of a role in terms of users, groups, and domains (identities).
// Only the specified users, groups, or users belonging to the specified domains can be assigned this role.
// The Domains field allows restricting role assignment to users from particular domains (e.g., email domains or organizational domains),
// and can be used in conjunction with Groups and Users for more granular access control.
// Deny takes precedence over Allow.
type RoleScopes struct {
	Allow ScopeIdentities `json:"allow"`
	Deny  ScopeIdentities `json:"deny"`
}

// UnmarshalJSON provides backwards compatibility for RoleScopes.
// If the old format (users/groups/domains at root) is used, they are moved to Allow.
func (r *RoleScopes) UnmarshalJSON(data []byte) error {
	// Define a struct that can capture both old and new formats
	type rawScopes struct {
		// New format
		Allow ScopeIdentities `json:"allow"`
		Deny  ScopeIdentities `json:"deny"`
		// Old format (backwards compatibility)
		Users   []string `json:"users,omitempty"`
		Groups  []string `json:"groups,omitempty"`
		Domains []string `json:"domains,omitempty"`
	}

	var aux rawScopes
	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to unmarshal role scopes: %w", err)
	}

	r.Allow = aux.Allow
	r.Deny = aux.Deny

	// Backwards compatibility: move root-level fields to Allow
	if len(aux.Users) > 0 {
		r.Allow.Users = append(r.Allow.Users, aux.Users...)
	}
	if len(aux.Groups) > 0 {
		r.Allow.Groups = append(r.Allow.Groups, aux.Groups...)
	}
	if len(aux.Domains) > 0 {
		r.Allow.Domains = append(r.Allow.Domains, aux.Domains...)
	}

	return nil
}

// IsEmpty returns true if both Allow and Deny are empty
func (r *RoleScopes) IsEmpty() bool {
	return r.Allow.IsEmpty() && r.Deny.IsEmpty()
}

type RoleResources struct {
	Allow []string `json:"allow,omitempty" validate:"max=500,dive,min=1,max=1000"`
	Deny  []string `json:"deny,omitempty" validate:"max=500,dive,min=1,max=1000"`
}
