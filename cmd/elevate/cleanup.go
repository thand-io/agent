package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/thand-io/agent/cmd/elevate/domain"
	"github.com/thand-io/agent/cmd/elevate/handler"
)

// CleanupRunner revokes and removes expired grants on startup and periodically.
type CleanupRunner struct {
	store    handler.StateStore
	grants   handler.GrantEngine
	clock    handler.Clock
	interval time.Duration
	logger   *slog.Logger
}

// NewCleanupRunner builds a cleanup runner that sweeps expired grants.
func NewCleanupRunner(store handler.StateStore, grants handler.GrantEngine, clock handler.Clock, interval time.Duration, logger *slog.Logger) (*CleanupRunner, error) {
	if store == nil {
		return nil, fmt.Errorf("state store is required")
	}
	if grants == nil {
		return nil, fmt.Errorf("grant engine is required")
	}
	if clock == nil {
		return nil, fmt.Errorf("clock is required")
	}
	if interval <= 0 {
		return nil, fmt.Errorf("cleanup interval must be > 0")
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &CleanupRunner{
		store:    store,
		grants:   grants,
		clock:    clock,
		interval: interval,
		logger:   logger,
	}, nil
}

// Run executes one startup sweep and then continues periodic sweeps until context cancellation.
func (c *CleanupRunner) Run(ctx context.Context) error {
	if err := c.runOnce(ctx); err != nil {
		return err
	}

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := c.runOnce(ctx); err != nil {
				return err
			}
		}
	}
}

func (c *CleanupRunner) runOnce(ctx context.Context) error {
	grants, err := c.store.List(ctx)
	if err != nil {
		return fmt.Errorf("list state grants: %w", err)
	}

	nowMono := c.clock.NowMonoNS()
	nowWall := c.clock.NowWallUTC()

	for _, g := range grants {
		if !isExpired(g, nowMono, nowWall) {
			continue
		}

		if err := c.grants.Revoke(ctx, domain.RevokeRequest{
			RequestID:  g.RequestID,
			WorkflowID: g.WorkflowID,
			Username:   g.Username,
		}); err != nil {
			return fmt.Errorf("revoke expired grant %q: %w", g.RequestID, err)
		}
		c.logger.Info("admin revoked by cleanup",
			"component", "elevate_cleanup",
			"request_id", g.RequestID,
			"workflow_id", g.WorkflowID,
			"username", g.Username,
			"reason", "expired",
		)

		if err := c.store.Delete(ctx, g.RequestID); err != nil {
			return fmt.Errorf("delete expired grant %q: %w", g.RequestID, err)
		}
	}

	return nil
}

func isExpired(grant domain.GrantState, nowMonoNS int64, nowWallUTC time.Time) bool {
	if grant.DurationSeconds <= 0 {
		return true
	}

	durationNS := grant.DurationSeconds * int64(time.Second)
	if grant.GrantedAtMonoNS > 0 && nowMonoNS >= grant.GrantedAtMonoNS {
		return nowMonoNS-grant.GrantedAtMonoNS >= durationNS
	}

	if grant.GrantedAtWallUTC.IsZero() {
		return false
	}
	return !nowWallUTC.Before(grant.GrantedAtWallUTC.Add(time.Duration(durationNS)))
}
