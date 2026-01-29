package models

import internal "github.com/thand-io/agent/internal/models"

// Role defines access permissions and configurations for users.
// It includes authentication providers, workflows, inherited roles,
// groups, permissions, resources, and scopes for role assignment.
type Role = internal.Role

// RoleGroups defines group-based access controls with allow and deny lists.
type RoleGroups = internal.RoleGroups

// RolePermissions defines permission-based access controls with allow and deny lists.
type RolePermissions = internal.RolePermissions

// RoleScopes defines the scope of a role in terms of users, groups, and domains.
// Only the specified users, groups, or users belonging to the specified domains
// can be assigned this role.
type RoleScopes = internal.RoleScopes

// RoleResources defines resource-based access controls with allow and deny lists.
type RoleResources = internal.RoleResources
