package models

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/go-version"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
)

// ─────────────────────────────────────────────────────────────────────────────
// Role lifecycle constants
//
// Roles follow one of two lifecycle patterns depending on whether they are
// composite (i.e. resolved from inherited thand roles) or non-composite:
//
//	Composite roles    – Unique per identity. Created on authorize, deleted on
//	                      revoke. Their CSP name includes a hash suffix so each
//	                      session gets an isolated role.
//
//	Non-composite roles – Persistent / shared. Created once with a version tag,
//	                      reused across sessions. On revoke only the user
//	                      binding is removed; the role itself is retained.
//	                      The version tag is checked on each authorize; if the
//	                      role definition has changed the CSP role is updated
//	                      in-place.
//
// ─────────────────────────────────────────────────────────────────────────────
const (
	// DefaultRoleVersion is used when a role has no explicit version set.
	DefaultRoleVersion = "1.0.0"

	// ThandVersionTagKey is the tag/label key used on CSP resources to record
	// the role definition version at the time the role was last created or
	// updated. Providers compare this value against the current role version
	// to decide whether an update is needed.
	ThandVersionTagKey = "thand:version"

	// ThandManagedTagKey is the tag/label key that marks a CSP resource as
	// managed by thand. Providers use this during cleanup and discovery.
	ThandManagedTagKey = "thand:managed"
)

type CompositeRole struct {

	// A unique identifier for this specific composite role
	// instance, generated based on the identity, base role,
	// and providers. This allows caching and reusing
	// composite roles for the same context without
	// needing to recompute them.
	UUID uuid.UUID `json:"uuid"`

	// Set the providers that this composite role has been resolved
	// for
	Providers []string `json:"composite_providers"` // Renamed to avoid collision with embedded Role.Providers

	// Composite indicates whether this role is a resolved, flattened composite
	// that includes inherited thand roles (true) or a non-composite, persistent
	// role representation without applied inheritance (false).
	// Defaults to false for non-composite roles; may be true when representing a composite role within CompositeRole.
	Composite bool `json:"composite"`

	Role `json:",inline" yaml:",inline"` // Embed the base
}

// IsComposite returns true when the role was produced by merging inherited
// thand roles. Composite roles are per-identity and should be deleted on
// revocation. Non-composite roles are persistent and shared.
func (r *CompositeRole) IsComposite() bool {
	return r.Composite
}

// MarshalJSON implements custom JSON marshaling for CompositeRole.
// This ensures that CompositeRole-specific fields (UUID, Providers, Composite)
// are explicitly included in the JSON output along with the embedded Role fields.
func (r *CompositeRole) MarshalJSON() ([]byte, error) {
	// Create a map to hold all fields
	result := make(map[string]any)

	// Marshal the embedded Role first
	roleBytes, err := json.Marshal(r.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedded role: %w", err)
	}

	// Unmarshal Role fields into the result map
	if err := json.Unmarshal(roleBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal role fields: %w", err)
	}

	// Add CompositeRole-specific fields (these will override if there are conflicts)
	result["uuid"] = r.UUID
	result["composite_providers"] = r.Providers
	result["composite"] = r.Composite

	return json.Marshal(result)
}

// UnmarshalJSON implements custom JSON unmarshaling for CompositeRole.
// This is CRITICAL for workflow.SideEffect serialization to work correctly in Temporal workflows.
//
// Without this custom unmarshaler, CompositeRole fields (UUID, Providers, Composite) are lost
// during Temporal's workflow.SideEffect serialization/deserialization cycle because:
// 1. The embedded Role struct (marked with json:",inline") has its own UnmarshalJSON method
// 2. Go's default JSON unmarshaler would call Role.UnmarshalJSON which creates a new Role instance
// 3. This new Role instance overwrites the CompositeRole fields that were already set
//
// This custom unmarshaler solves the problem by:
// 1. First extracting CompositeRole-specific fields (uuid, composite_providers, composite) directly
// 2. Then delegating to Role.UnmarshalJSON for the embedded Role fields
// 3. This ensures all CompositeRole fields survive the serialization round-trip
//
// See test/integration/workflows/sideeffect_serialization_test.go for verification tests.
func (r *CompositeRole) UnmarshalJSON(data []byte) error {
	// First, unmarshal into a temporary map to get all fields
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to unmarshal composite role: %w", err)
	}

	// Unmarshal CompositeRole-specific fields directly to prevent them from being overwritten
	if uuidData, ok := raw["uuid"]; ok {
		if err := json.Unmarshal(uuidData, &r.UUID); err != nil {
			return fmt.Errorf("failed to unmarshal uuid: %w", err)
		}
	}

	if providersData, ok := raw["composite_providers"]; ok {
		if err := json.Unmarshal(providersData, &r.Providers); err != nil {
			return fmt.Errorf("failed to unmarshal composite_providers: %w", err)
		}
	}

	if compositeData, ok := raw["composite"]; ok {
		if err := json.Unmarshal(compositeData, &r.Composite); err != nil {
			return fmt.Errorf("failed to unmarshal composite: %w", err)
		}
	}

	// Now unmarshal the embedded Role by delegating to Role.UnmarshalJSON
	if err := json.Unmarshal(data, &r.Role); err != nil {
		return fmt.Errorf("failed to unmarshal embedded role: %w", err)
	}

	return nil
}

func (r *CompositeRole) SetUniqueIdentifier(uuid uuid.UUID) {
	r.UUID = uuid
}

func (r *CompositeRole) SetUniqueIdentifierFromString(idStr string) {

	// Create uuid from string that may already contain a hash suffix (e.g., "baseRole_abcdef123456")
	r.UUID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(idStr))

}

func (r *CompositeRole) SetProviders(providers []string) {
	r.Providers = providers
}

// CompositeRoleWorkflowIdentifier builds the unique string used to derive a composite
// role's UUID (and therefore its CSP resource name via GetName) for a specific
// workflow execution. Both the config layer (GetCompositeRoleForWorkflow) and any
// test code that needs to predict the resulting role name should use this function.
func CompositeRoleWorkflowIdentifier(workflowID string, role *Role, identity *Identity) string {
	userIdentity := "unknown"
	if identity != nil {
		userIdentity = identity.GetMappableIdentifier()
	}
	return fmt.Sprintf("%s:%s:%s:%s:%s",
		workflowID,
		role.Identifier,
		role.GetVersionString(),
		role.Name,
		userIdentity,
	)
}

func (r *CompositeRole) GetUniqueIdentifier() uuid.UUID {
	if r.UUID != uuid.Nil {
		return r.UUID
	}
	// Fallback to base role identifier if UUID is not set
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(r.Role.GetIdentifier()))
}

// GetName returns the name of the composite role, which includes a hash suffix for uniqueness.
// For composite roles, the name is generated by hashing the workflow ID, base role identifier, version, and user identity to ensure uniqueness per workflow execution and user. For non-composite roles, it returns the base role's identifier.
// This approach allows composite roles to be uniquely identified and managed in the CSP, while non-composite roles remain stable and shared.
// Note: The hash is truncated to 6 hex characters to keep the name concise while still providing a high degree of uniqueness.
// The resulting name format for composite roles is: "{baseRoleIdentifier}_{6CharHashSuffix}"
// This is critical functionality for role naming. DO NOT MODIFY. THIS WILL BE A BREAKING CHANGE.
func (r *CompositeRole) GetName() string {
	if r.Composite {
		return r.getUniqueName()
	}
	return r.Role.GetIdentifier()
}

func (r *CompositeRole) getUniqueName() string {

	roleIdentifier := r.GetUniqueIdentifier()

	// Create FNV-1a hash (non-cryptographic, fast, 6 hex chars)
	h := fnv.New32a()
	h.Write([]byte(roleIdentifier.String()))

	// Conform to snake_case for consistency
	newIdentifier := fmt.Sprintf("%s_%06x", r.Role.GetIdentifier(), h.Sum32()&0xFFFFFF)

	return newIdentifier
}

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

	Enabled bool `json:"enabled" default:"true"` // By default enable the role
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

// GetVersionString returns the role version as a string suitable for tagging.
// Returns "1.0.0" when no version is explicitly set.
func (r *Role) GetVersionString() string {
	if r.Version != nil {
		return r.Version.String()
	}
	return DefaultRoleVersion
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
	// ID is an optional identifier for this statement, used to derive
	// deterministic per-statement custom role IDs in providers like GCP.
	// Must be strict snake_case (lowercase alphanumeric and underscores,
	// starting with a letter). When omitted, the statement's index in the
	// list is used as a fallback suffix.
	ID string `json:"id,omitempty" validate:"omitempty,snake_case,min=1,max=64"`

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

	// Binding declares the explicit CSP resource at which this permission statement
	// should be created and assigned, independent of the tenant used at request time.
	//
	// Format is provider-specific:
	//   GCP:   "projects/{id}" — the project where the custom role is created and
	//          where the IAM binding is applied.
	//          Note: organization-scope via this field is not currently supported;
	//          use the provider-level 'organization_id' config for org-scoped roles.
	//   Azure: "/subscriptions/{id}" or "/subscriptions/{id}/resourceGroups/{rg}"
	//   AWS:   "arn:aws:iam::{account-id}:root"
	//
	// When set, the provider uses this value to determine where a custom role is
	// created and where the IAM binding is applied, regardless of the request tenant
	// (e.g. regardless of whether the tenant is a folder, project, or org).
	//
	// When omitted, the provider falls back to the request tenant for binding scope.
	// Providers may additionally attempt to infer a binding resource from Targets
	// for backwards compatibility.
	Binding string `json:"binding,omitempty" validate:"omitempty,csp_binding,max=500"`
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
