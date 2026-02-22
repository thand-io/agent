package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/thand-io/agent/cmd/elevate/clock"
	elevateconfig "github.com/thand-io/agent/cmd/elevate/config"
	"github.com/thand-io/agent/cmd/elevate/grant"
	"github.com/thand-io/agent/cmd/elevate/handler"
	"github.com/thand-io/agent/cmd/elevate/ipc"
	"github.com/thand-io/agent/cmd/elevate/state"
	"github.com/thand-io/agent/cmd/elevate/verify"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps, err := buildDependencies()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build dependencies: %v\n", err)
		os.Exit(1)
	}

	server := NewServer(deps.ipc, deps.handler)
	if err := server.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "elevate server error: %v\n", err)
		os.Exit(1)
	}
}

type dependencies struct {
	ipc     handler.IPCServer
	handler *handler.Handler
}

func buildDependencies() (*dependencies, error) {
	cfg, err := elevateconfig.LoadFromEnv()
	if err != nil {
		return nil, err
	}

	ipcServer, err := ipc.NewUnixServer(cfg.SocketPath)
	if err != nil {
		return nil, err
	}

	grantEngine := grant.NewPlaceholderEngine()
	verifier := verify.NewPlaceholderVerifier()
	stateStore := state.NewPlaceholderStore()
	clk := clock.NewPlaceholderClock()

	router := handler.New(grantEngine, verifier, stateStore, clk)

	return &dependencies{ipc: ipcServer, handler: router}, nil
}
