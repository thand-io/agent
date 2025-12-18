package models

import internal "github.com/thand-io/agent/internal/models"

// Role defines access permissions and configurations for users.
// It includes authentication providers, workflows, inherited roles,
// groups, permissions, resources, and scopes for role assignment.
type Role = internal.Role

// Groups defines group-based access controls with allow and deny lists.
type Groups = internal.Groups

// Permissions defines permission-based access controls with allow and deny lists.
type Permissions = internal.Permissions

// RoleScopes defines the scope of a role in terms of users, groups, and domains.
// Only the specified users, groups, or users belonging to the specified domains
// can be assigned this role.
type RoleScopes = internal.RoleScopes

// Resources defines resource-based access controls with allow and deny lists.
type Resources = internal.Resources
