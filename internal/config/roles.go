package config

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

// LoadRoles loads roles from a file or URL
func (c *Config) LoadRoles() (map[string]models.Role, error) {

	vaultData := ""

	if len(c.Roles.Vault) > 0 {

		if !c.HasVault() {
			return nil, fmt.Errorf("vault configuration is missing. Cannot load roles from vault")
		}

		logrus.Debugln("Loading roles from vault: ", c.Roles.Vault)

		// Load roles from Vault
		data, err := c.GetVault().GetSecret(c.Roles.Vault)

		if err != nil {
			logrus.WithError(err).Errorln("Error loading roles from vault")
			return nil, fmt.Errorf("failed to get secret from vault: %w", err)
		}

		logrus.Debugln("Loaded roles from vault: ", len(data), " bytes")

		vaultData = string(data)
	}

	foundRoles, err := loadDataFromSource(
		c.Roles.Path,
		c.Roles.URL,
		vaultData,
		RoleDefinitions{},
	)

	if err != nil {
		logrus.WithError(err).Errorln("Failed to load roles data")
		return nil, fmt.Errorf("failed to load roles data: %w", err)
	}

	defs := make(map[string]models.Role)

	logrus.Debugln("Processing loaded roles: ", len(foundRoles))

	for _, role := range foundRoles {
		for roleKey, r := range role.Roles {

			if !r.Enabled {
				logrus.Infoln("Role disabled:", roleKey)
				continue
			}

			if _, exists := defs[roleKey]; exists {
				logrus.Warningln("Duplicate role key found, skipping:", roleKey)
				continue
			}

			if len(r.Name) == 0 {
				r.Name = roleKey
			}

			defs[roleKey] = r
		}
	}

	return defs, nil
}
