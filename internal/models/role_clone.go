package models

func CloneRole(role *Role) *Role {
	if role == nil {
		return nil
	}

	clone := *role
	clone.Authenticators = append([]string(nil), role.Authenticators...)
	clone.Workflows = append([]string(nil), role.Workflows...)
	clone.Providers = append([]string(nil), role.Providers...)
	clone.Inherits = append([]string(nil), role.Inherits...)

	return &clone
}
