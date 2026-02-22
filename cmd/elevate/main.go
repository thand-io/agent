package main

import (
	"context"
	"fmt"
	"log/slog"
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
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps, err := buildDependencies(logger)
	if err != nil {
		logger.Error("failed to build dependencies", "err", err)
		os.Exit(1)
	}
	if deps.logger != nil {
		logger = deps.logger
	}

	logger.Info("elevate helper starting",
		"component", "elevate_main",
		"socket_path", deps.cfg.SocketPath,
		"state_path", deps.cfg.StatePath,
		"cleanup_interval", deps.cfg.CleanupInterval.String(),
		"request_timeout", deps.cfg.RequestTimeout.String(),
		"socket_gid", deps.cfg.SocketGID,
		"log_level", deps.cfg.LogLevel,
	)

	if err := run(ctx, deps); err != nil {
		logger.Error("elevate server error", "err", err)
		os.Exit(1)
	}
}

type dependencies struct {
	ipc     handler.IPCServer
	handler *handler.Handler
	cleanup *CleanupRunner
	logger  *slog.Logger
	cfg     *elevateconfig.Config
}

func buildDependencies(baseLogger *slog.Logger) (*dependencies, error) {
	cfg, err := elevateconfig.LoadFromEnv()
	if err != nil {
		return nil, err
	}
	logLevel, err := elevateconfig.ParseLogLevel(cfg.LogLevel)
	if err != nil {
		return nil, err
	}
	logger := baseLogger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	ipcOpts := []ipc.Option{}
	if cfg.SocketGID >= 0 {
		ipcOpts = append(ipcOpts, ipc.WithSocketGID(cfg.SocketGID))
	}
	ipcServer, err := ipc.NewUnixServer(cfg.SocketPath, ipcOpts...)
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

	router := handler.New(
		grantEngine,
		verifier,
		stateStore,
		clk,
		handler.WithLogger(logger),
		handler.WithRequestTimeout(cfg.RequestTimeout),
	)
	cleanupRunner, err := NewCleanupRunner(stateStore, grantEngine, clk, cfg.CleanupInterval)
	if err != nil {
		return nil, err
	}

	return &dependencies{
		ipc:     ipcServer,
		handler: router,
		cleanup: cleanupRunner,
		logger:  logger,
		cfg:     cfg,
	}, nil
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
