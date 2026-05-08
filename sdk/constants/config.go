package constants

// Mode represents the operational mode of the agent, such as "client", "agent" or "server".
// client - Local CLI operations without server connectivity.
// agent - Runs locally to manage local access and session management with server connectivity.
// server - Public endpoint to execute workflows without direct access to infrastructure.
type Mode string

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
