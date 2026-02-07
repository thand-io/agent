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

// RoleResponse represents the role object returned in API responses.
// It is currently defined as an alias of Role.
type RoleResponse = Role
