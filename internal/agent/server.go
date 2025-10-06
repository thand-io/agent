package agent

import (
	"fmt"

	"github.com/thand-io/agent/internal/config"
	server "github.com/thand-io/agent/internal/daemon"
)

// This file creates a local server to handle CLI requests.
// these CLI requests are primarily used to authenticate
// and authorize the user to access various resources defined
// by the roles and workflows.

func StartWebService(cfg *config.Config) (*server.Server, error) {
	// Initialize the web server with the provided configuration
	webServer := server.NewServer(cfg)

	// Start the server - this is blocking
	if err := webServer.Start(); err != nil {
		return nil, fmt.Errorf("failed to start web service: %w", err)
	}

	return webServer, nil
}
