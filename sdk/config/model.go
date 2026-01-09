// Package config provides public SDK types for agent configuration and registration.
// These types are re-exported from the internal config package to provide
// a stable public API for external consumers.
package config

import (
	internal "github.com/thand-io/agent/internal/config"
)

// Mode represents the operational mode of the agent, such as "client", "agent" or "server".
// client - Local CLI operations without server connectivity.
// agent - Runs locally to manage local access and session management with server connectivity.
// server - Public endpoint to execute workflows without direct access to infrastructure.
type Mode = internal.Mode

// PreflightRequest represents the request sent before registration to validate
// configuration and check prerequisites for agent setup.
type PreflightRequest = internal.PreflightRequest

// PreflightResponse contains the server's response to a preflight check,
// including validation results and any required configuration adjustments.
type PreflightResponse = internal.PreflightResponse

// RegistrationRequest contains the data required to register an agent
// with the server, including identity and configuration information.
type RegistrationRequest = internal.RegistrationRequest

// RegistrationResponse contains the server's response after a successful
// agent registration, including assigned identifiers and initial credentials.
type RegistrationResponse = internal.RegistrationResponse

// PostflightRequest represents the request sent after registration to finalize
// the setup process and confirm agent activation.
type PostflightRequest = internal.PostflightRequest

// PostflightResponse contains the server's confirmation of completed registration
// and any final configuration or status information.
type PostflightResponse = internal.PostflightResponse

// RoleDefinitionsConfig defines a role configuration that specifies access permissions
// and constraints for users requesting access through the agent.
type RoleDefinitionsConfig = internal.RoleConfig

// ProviderDefinitionsConfig defines the configuration for a provider integration,
// specifying how the agent connects to and manages external services.
type ProviderDefinitionsConfig = internal.ProviderDefinitionsConfig

// WorkflowDefinitionsConfig defines a workflow configuration that specifies the
// approval process and steps for handling access requests.
type WorkflowDefinitionsConfig = internal.WorkflowConfig

const (

	// Runs in cloud environment as a login server
	// allows agents to sync roles and policies and get tasking
	ModeServer Mode = "server"

	// Runs as a background agent to store session data and
	// exec platform specific elevations
	ModeAgent Mode = "agent"

	// Just the CLI mode - used to connect to login-servers
	ModeClient Mode = "client"
)
