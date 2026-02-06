package models

import (
	"github.com/hashicorp/go-version"
)

// RolesResponse represents the response for /roles endpoint
// These are used for user facing API responses.
type RolesResponse struct {
	Version *version.Version             `json:"version"`
	Roles   []SearchResult[RoleResponse] `json:"roles"`
	Meta    ResponseMeta                 `json:"meta"`
}

// RoleResponse is a simplified response object that only includes
// the identifier, name and description of a role
type RoleResponse = Role
