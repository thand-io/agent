package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
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

	deps, err := buildDependencies()
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
		"state_retention", deps.cfg.StateRetention.String(),
		"request_timeout", deps.cfg.RequestTimeout.String(),
		"socket_user", deps.cfg.SocketUser,
		"socket_group", deps.cfg.SocketGroup,
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

type platformDependencies struct {
	ipc         handler.IPCServer
	grantEngine handler.GrantEngine
	clock       handler.Clock
}

func buildDependencies() (*dependencies, error) {
	cfg, err := elevateconfig.LoadFromEnv()
	if err != nil {
		return nil, err
	}
	logLevel, err := elevateconfig.ParseLogLevel(cfg.LogLevel)
	if err != nil {
		return nil, err
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	platformDeps, err := buildPlatformDependencies(cfg)
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

	router := handler.New(
		platformDeps.grantEngine,
		verifier,
		stateStore,
		platformDeps.clock,
		handler.WithLogger(logger),
		handler.WithRequestTimeout(cfg.RequestTimeout),
	)
	cleanupRunner, err := NewCleanupRunner(stateStore, platformDeps.grantEngine, platformDeps.clock, cfg.CleanupInterval, cfg.StateRetention, logger)
	if err != nil {
		return nil, err
	}

	return &dependencies{
		ipc:     platformDeps.ipc,
		handler: router,
		cleanup: cleanupRunner,
		logger:  logger,
		cfg:     cfg,
	}, nil
}

func buildPlatformDependencies(cfg *elevateconfig.Config) (*platformDependencies, error) {
	switch runtime.GOOS {
	case "linux":
		return buildLinuxPlatformDependencies(cfg)
	case "darwin":
		return nil, fmt.Errorf("unsupported operating system %q: darwin implementation not added yet", runtime.GOOS)
	case "windows":
		return buildWindowsPlatformDependencies(cfg)
	default:
		return nil, fmt.Errorf("unsupported operating system %q", runtime.GOOS)
	}
}

func buildLinuxPlatformDependencies(cfg *elevateconfig.Config) (*platformDependencies, error) {
	ipcOpts := []ipc.Option{}
	if cfg.SocketUser != "" {
		ipcOpts = append(ipcOpts, ipc.WithSocketUser(cfg.SocketUser))
	}
	if cfg.SocketGroup != "" {
		ipcOpts = append(ipcOpts, ipc.WithSocketGroup(cfg.SocketGroup))
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

	return &platformDependencies{
		ipc:         ipcServer,
		grantEngine: grantEngine,
		clock:       clock.NewClock(),
	}, nil
}

func buildWindowsPlatformDependencies(cfg *elevateconfig.Config) (*platformDependencies, error) {
	ipcOpts := []ipc.Option{}
	if cfg.SocketUser != "" {
		ipcOpts = append(ipcOpts, ipc.WithSocketUser(cfg.SocketUser))
	}
	ipcServer, err := ipc.NewUnixServer(cfg.SocketPath, ipcOpts...)
	if err != nil {
		return nil, err
	}

	grantEngine, err := grant.NewWindowsEngine(grant.WindowsEngineConfig{
		AdminGroup: cfg.WindowsAdminGroup,
	})
	if err != nil {
		return nil, err
	}

	return &platformDependencies{
		ipc:         ipcServer,
		grantEngine: grantEngine,
		clock:       clock.NewClock(),
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
