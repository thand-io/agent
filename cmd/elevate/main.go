package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/thand-io/agent/cmd/elevate/clock"
	"github.com/thand-io/agent/cmd/elevate/grant"
	"github.com/thand-io/agent/cmd/elevate/handler"
	"github.com/thand-io/agent/cmd/elevate/ipc"
	"github.com/thand-io/agent/cmd/elevate/state"
	"github.com/thand-io/agent/cmd/elevate/verify"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps := buildDependencies()

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

func buildDependencies() *dependencies {
	ipcServer := ipc.NewPlaceholderServer()
	grantEngine := grant.NewPlaceholderEngine()
	verifier := verify.NewPlaceholderVerifier()
	stateStore := state.NewPlaceholderStore()
	clk := clock.NewPlaceholderClock()

	router := handler.New(grantEngine, verifier, stateStore, clk)

	return &dependencies{ipc: ipcServer, handler: router}
}
