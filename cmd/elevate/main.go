package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	if err := run(ctx, deps); err != nil {
		fmt.Fprintf(os.Stderr, "elevate server error: %v\n", err)
		os.Exit(1)
	}
}

type dependencies struct {
	ipc     handler.IPCServer
	handler *handler.Handler
	cleanup *CleanupRunner
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

	grantEngine, err := grant.NewLinuxEngine(grant.LinuxEngineConfig{
		SudoersDir:  cfg.SudoersDir,
		SudoersFile: cfg.SudoersFile,
		VisudoBin:   cfg.VisudoBin,
	})
	if err != nil {
		return nil, err
	}
	verifier, err := verify.NewVerifier()
	if err != nil {
		return nil, err
	}
	stateStore, err := state.NewFileStore(cfg.StatePath)
	if err != nil {
		return nil, err
	}
	clk := clock.NewPlaceholderClock()

	router := handler.New(grantEngine, verifier, stateStore, clk)
	cleanupRunner, err := NewCleanupRunner(stateStore, grantEngine, clk, cfg.CleanupInterval)
	if err != nil {
		return nil, err
	}

	return &dependencies{ipc: ipcServer, handler: router, cleanup: cleanupRunner}, nil
}

func run(ctx context.Context, deps *dependencies) error {
	if deps == nil {
		return fmt.Errorf("dependencies are required")
	}
	if deps.cleanup == nil {
		return fmt.Errorf("cleanup runner is required")
	}

	server := NewServer(deps.ipc, deps.handler)

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Run(ctx)
	}()

	cleanupErr := make(chan error, 1)
	go func() {
		cleanupErr <- deps.cleanup.Run(ctx)
	}()

	select {
	case err := <-serverErr:
		return err
	case err := <-cleanupErr:
		return err
	case <-ctx.Done():
		// Give goroutines a brief chance to observe cancellation and stop.
		select {
		case err := <-serverErr:
			return err
		case err := <-cleanupErr:
			return err
		case <-time.After(250 * time.Millisecond):
			return nil
		}
	}
}
