package models

import (
	"fmt"
	"strings"
)

type DeviceLocalElevationPolicy struct {
	Enabled          bool                          `json:"enabled" yaml:"enabled" mapstructure:"enabled"`
	AllowedModes     []string                      `json:"allowed_modes,omitempty" yaml:"allowed_modes,omitempty" mapstructure:"allowed_modes"`
	Accounts         []DeviceLocalElevationAccount `json:"accounts,omitempty" yaml:"accounts,omitempty" mapstructure:"accounts"`
	DeniedUsernames  []string                      `json:"denied_usernames,omitempty" yaml:"denied_usernames,omitempty" mapstructure:"denied_usernames"`
	AllowedUIDRanges []string                      `json:"allowed_uid_ranges,omitempty" yaml:"allowed_uid_ranges,omitempty" mapstructure:"allowed_uid_ranges"`
}

type DeviceLocalElevationAccount struct {
	Identity      string `json:"identity,omitempty" yaml:"identity,omitempty" mapstructure:"identity"`
	Email         string `json:"email,omitempty" yaml:"email,omitempty" mapstructure:"email"`
	Username      string `json:"username,omitempty" yaml:"username,omitempty" mapstructure:"username"`
	LocalUsername string `json:"local_username" yaml:"local_username" mapstructure:"local_username"`
}

func (p *DeviceLocalElevationPolicy) AllowsMode(mode string) bool {
	if p == nil || !p.Enabled {
		return false
	}
	if len(p.AllowedModes) == 0 {
		return true
	}
	for _, allowed := range p.AllowedModes {
		if strings.EqualFold(strings.TrimSpace(allowed), strings.TrimSpace(mode)) {
			return true
		}
	}
	return false
}

func (p *DeviceLocalElevationPolicy) ResolveLocalUsername(identityID string, identity *Identity) (string, error) {
	if p == nil || !p.Enabled {
		return "", fmt.Errorf("local elevation is not enabled for this device")
	}

	trimmedIdentityID := strings.TrimSpace(identityID)
	var email string
	var username string
	if identity != nil && identity.User != nil {
		email = strings.TrimSpace(identity.User.Email)
		username = strings.TrimSpace(identity.User.Username)
	}

	for _, account := range p.Accounts {
		if account.matches(trimmedIdentityID, email, username) {
			localUsername := strings.TrimSpace(account.LocalUsername)
			if localUsername == "" {
				return "", fmt.Errorf("device account mapping matched without a local username")
			}
			return localUsername, nil
		}
	}

	return "", fmt.Errorf("identity %q is not eligible for local sudo on this device", trimmedIdentityID)
}

func (a DeviceLocalElevationAccount) matches(identityID, email, username string) bool {
	if strings.TrimSpace(a.Identity) != "" && strings.EqualFold(strings.TrimSpace(a.Identity), identityID) {
		return true
	}
	if strings.TrimSpace(a.Email) != "" && strings.EqualFold(strings.TrimSpace(a.Email), email) {
		return true
	}
	if strings.TrimSpace(a.Username) != "" && strings.EqualFold(strings.TrimSpace(a.Username), username) {
		return true
	}
	return false
}
