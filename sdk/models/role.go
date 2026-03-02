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

// RoleStatements defines a set of statements that make up the role's access control policies.
type RoleStatements = internal.RoleStatements

// RoleStatement defines a single statement within a role's access control policies.
type RoleStatement = internal.Statement

// RoleDefinitions is a map of role identifiers to their corresponding Role definitions.
type RoleDefinitions = internal.RoleDefinitions

// RolesResponse is the response object for API endpoints that return role information.
type RolesResponse = internal.RolesResponse

// RoleResponse is the response object for API endpoints that return a single role's information.
type RoleResponse = internal.RoleResponse
