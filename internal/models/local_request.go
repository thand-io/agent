package models

import (
	"fmt"
	"slices"
	"strings"

	"github.com/thand-io/agent/internal/common"
)

type LocalSudoMode string

const (
	LocalSudoModeTimed   LocalSudoMode = "timed"
	LocalSudoModeCommand LocalSudoMode = "command"

	LocalSudoRoleIdentifier      = "local_sudo"
	LocalSudoTimedWorkflowName   = "local_sudo_timed_elevation"
	LocalSudoCommandWorkflowName = "local_sudo_command_elevation"
	LocalSudoCommandDuration     = "1m"
)

type LocalSudoRequestMetadata struct {
	Mode             LocalSudoMode `json:"mode"`
	Command          []string      `json:"command,omitempty"`
	GrantID          string        `json:"grant_id,omitempty"`
	DeviceID         string        `json:"device_id,omitempty"`
	LocalUsername    string        `json:"local_username,omitempty"`
	DeniedUsernames  []string      `json:"denied_usernames,omitempty"`
	AllowedUIDRanges []string      `json:"allowed_uid_ranges,omitempty"`
}

func (m LocalSudoRequestMetadata) AsMap() map[string]any {
	result := map[string]any{
		"mode": string(m.Mode),
	}

	if len(m.Command) > 0 {
		result["command"] = append([]string(nil), m.Command...)
	}
	if len(m.GrantID) > 0 {
		result["grant_id"] = m.GrantID
	}
	if len(m.DeviceID) > 0 {
		result["device_id"] = m.DeviceID
	}
	if len(m.LocalUsername) > 0 {
		result["local_username"] = m.LocalUsername
	}
	if len(m.DeniedUsernames) > 0 {
		result["denied_usernames"] = append([]string(nil), m.DeniedUsernames...)
	}
	if len(m.AllowedUIDRanges) > 0 {
		result["allowed_uid_ranges"] = append([]string(nil), m.AllowedUIDRanges...)
	}

	return result
}

func DecodeLocalSudoRequestMetadata(value map[string]any) (LocalSudoRequestMetadata, error) {
	meta := LocalSudoRequestMetadata{}
	if err := common.ConvertInterfaceToInterface(value, &meta); err != nil {
		return meta, fmt.Errorf("failed to parse local sudo request metadata: %w", err)
	}
	if len(meta.Mode) == 0 {
		meta.Mode = LocalSudoModeTimed
	}
	return meta, nil
}

type LocalSudoAuthorizationMetadata struct {
	Platform       string   `json:"platform,omitempty"`
	Mode           string   `json:"mode,omitempty"`
	GrantID        string   `json:"grant_id,omitempty"`
	DeviceID       string   `json:"device_id,omitempty"`
	Username       string   `json:"username,omitempty"`
	BrokerHandle   string   `json:"broker_handle,omitempty"`
	SudoersPath    string   `json:"sudoers_path,omitempty"`
	LeasePath      string   `json:"lease_path,omitempty"`
	Command        []string `json:"command,omitempty"`
	Stdout         string   `json:"stdout,omitempty"`
	Stderr         string   `json:"stderr,omitempty"`
	ExitCode       int      `json:"exit_code,omitempty"`
	Immediate      bool     `json:"immediate,omitempty"`
	RevokedLocally bool     `json:"revoked_locally,omitempty"`
}

func DecodeLocalSudoAuthorizationMetadata(value map[string]any) (LocalSudoAuthorizationMetadata, error) {
	meta := LocalSudoAuthorizationMetadata{}
	if err := common.ConvertInterfaceToInterface(value, &meta); err != nil {
		return meta, fmt.Errorf("failed to parse local sudo authorization metadata: %w", err)
	}
	return meta, nil
}

func IsLocalSudoRequest(request *ElevateRequest) bool {
	if request == nil {
		return false
	}

	switch strings.TrimSpace(request.GetWorkflow()) {
	case LocalSudoTimedWorkflowName, LocalSudoCommandWorkflowName:
		return true
	}

	return request.Role != nil && strings.TrimSpace(request.Role.GetIdentifier()) == LocalSudoRoleIdentifier
}

func NormalizeLocalSudoRequest(
	request *ElevateRequest,
	providers map[string]ProviderConfig,
) error {
	if !IsLocalSudoRequest(request) {
		return nil
	}
	if request.Role == nil {
		return fmt.Errorf("local sudo request must include a role")
	}

	meta, err := DecodeLocalSudoRequestMetadata(request.Metadata)
	if err != nil {
		return err
	}

	if len(meta.Command) > 0 {
		meta.Mode = LocalSudoModeCommand
	}

	if len(strings.TrimSpace(request.Workflow)) == 0 {
		switch meta.Mode {
		case LocalSudoModeCommand:
			request.Workflow = LocalSudoCommandWorkflowName
		default:
			request.Workflow = LocalSudoTimedWorkflowName
		}
	}

	if meta.Mode == LocalSudoModeTimed && strings.TrimSpace(request.Workflow) == LocalSudoCommandWorkflowName {
		meta.Mode = LocalSudoModeCommand
	}
	if meta.Mode == LocalSudoModeCommand && strings.TrimSpace(request.Workflow) == LocalSudoTimedWorkflowName {
		request.Workflow = LocalSudoCommandWorkflowName
	}
	if meta.Mode == LocalSudoModeTimed && len(meta.Command) > 0 {
		meta.Mode = LocalSudoModeCommand
		request.Workflow = LocalSudoCommandWorkflowName
	}

	if meta.Mode == LocalSudoModeCommand && len(strings.TrimSpace(request.Duration)) == 0 {
		request.Duration = LocalSudoCommandDuration
	}
	if meta.Mode == LocalSudoModeTimed && len(strings.TrimSpace(request.Duration)) == 0 {
		return fmt.Errorf("local sudo timed access requires a duration")
	}

	request.Device = strings.TrimSpace(request.Device)
	if request.Device == "" {
		return fmt.Errorf("local sudo requires a device_id")
	}
	meta.DeviceID = request.Device

	providerName, providerType, err := resolveLocalSudoProvider(request, providers)
	if err != nil {
		return err
	}

	requestRole := CloneRole(request.Role)
	requestRole.Workflows = EnsureStringPresent(requestRole.Workflows, request.Workflow)
	requestRole.Providers = EnsureStringPresent(requestRole.Providers, providerType)
	requestRole.Providers = EnsureStringPresent(requestRole.Providers, providerName)

	request.Role = requestRole
	request.Providers = []string{providerName}
	request.Metadata = meta.AsMap()
	return nil
}

func resolveLocalSudoProvider(request *ElevateRequest, definitions map[string]ProviderConfig) (string, string, error) {
	for _, providerName := range request.Providers {
		if trimmed := strings.TrimSpace(providerName); trimmed != "" {
			return trimmed, "local", nil
		}
	}

	providerName, providerType, err := PreferredLocalProvider(definitions)
	if err == nil {
		return providerName, providerType, nil
	}

	for _, providerName := range request.Role.Providers {
		if trimmed := strings.TrimSpace(providerName); trimmed != "" && trimmed != "local" {
			return trimmed, "local", nil
		}
	}
	if slices.Contains(request.Role.Providers, "local") {
		return "local", "local", nil
	}

	return "", "", err
}

func PreferredLocalProvider(definitions map[string]ProviderConfig) (string, string, error) {
	if len(definitions) == 0 {
		return "", "", fmt.Errorf("no providers are configured")
	}

	if provider, ok := definitions["local-elevation"]; ok && provider.Provider == "local" {
		return "local-elevation", provider.Provider, nil
	}
	if provider, ok := definitions["local"]; ok && provider.Provider == "local" {
		return "local", provider.Provider, nil
	}

	for name, provider := range definitions {
		if provider.Provider == "local" {
			return name, provider.Provider, nil
		}
	}

	return "", "", fmt.Errorf("no local provider is configured")
}

func EnsureStringPresent(values []string, value string) []string {
	if len(value) == 0 || slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}
