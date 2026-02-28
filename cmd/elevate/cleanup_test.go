package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thand-io/agent/cmd/elevate/domain"
)

type stubStore struct {
	mu     sync.Mutex
	grants []domain.GrantState
}

func (s *stubStore) Put(ctx context.Context, grant domain.GrantState) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.grants {
		if s.grants[i].RequestID == grant.RequestID {
			s.grants[i] = grant
			return nil
		}
	}
	s.grants = append(s.grants, grant)
	return nil
}

func (s *stubStore) Delete(ctx context.Context, requestID string) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.grants[:0]
	for _, g := range s.grants {
		if g.RequestID != requestID {
			filtered = append(filtered, g)
		}
	}
	s.grants = filtered
	return nil
}

func (s *stubStore) List(ctx context.Context) ([]domain.GrantState, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.GrantState, len(s.grants))
	copy(out, s.grants)
	return out, nil
}

type stubGrantEngine struct {
	mu      sync.Mutex
	revoked []string
	err     error
	revokeC chan string
}

func (g *stubGrantEngine) Grant(ctx context.Context, req domain.GrantRequest) (domain.GrantResult, error) {
	_ = ctx
	_ = req
	return domain.GrantResult{}, errors.New("not implemented")
}

func (g *stubGrantEngine) Revoke(ctx context.Context, req domain.RevokeRequest) error {
	_ = ctx
	if g.err != nil {
		return g.err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.revoked = append(g.revoked, req.RequestID)
	if g.revokeC != nil {
		select {
		case g.revokeC <- req.RequestID:
		default:
		}
	}
	return nil
}

func (g *stubGrantEngine) revokedCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.revoked)
}

type stubClock struct {
	mono atomic.Int64
	wall time.Time
}

func (c *stubClock) NowMonoNS() int64      { return c.mono.Load() }
func (c *stubClock) NowWallUTC() time.Time { return c.wall }

func TestIsExpired(t *testing.T) {
	baseWall := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		grant   domain.GrantState
		nowMono int64
		nowWall time.Time
		want    bool
	}{
		{
			name: "not expired when wall and monotonic are both active",
			grant: domain.GrantState{
				GrantedAtMonoNS:  100,
				GrantedAtWallUTC: baseWall,
				DurationSeconds:  10,
			},
			nowMono: 105,
			nowWall: baseWall,
			want:    false,
		},
		{
			name: "expired when wall clock has expired",
			grant: domain.GrantState{
				GrantedAtMonoNS:  100,
				GrantedAtWallUTC: baseWall,
				DurationSeconds:  10,
			},
			nowMono: 105,
			nowWall: baseWall.Add(11 * time.Second),
			want:    true,
		},
		{
			name: "expired when wall clock is active but monotonic has expired",
			grant: domain.GrantState{
				GrantedAtMonoNS:  100,
				GrantedAtWallUTC: baseWall,
				DurationSeconds:  10,
			},
			nowMono: 100 + int64(10*time.Second),
			nowWall: baseWall,
			want:    true,
		},
		{
			name: "not expired after restart when wall clock is still active",
			grant: domain.GrantState{
				GrantedAtMonoNS:  1_000_000,
				GrantedAtWallUTC: baseWall,
				DurationSeconds:  10,
			},
			nowMono: 1,
			nowWall: baseWall.Add(5 * time.Second),
			want:    false,
		},
		{
			name: "expired after restart when wall clock has expired",
			grant: domain.GrantState{
				GrantedAtMonoNS:  1_000_000,
				GrantedAtWallUTC: baseWall,
				DurationSeconds:  10,
			},
			nowMono: 1,
			nowWall: baseWall.Add(11 * time.Second),
			want:    true,
		},
		{
			name: "invalid duration expires",
			grant: domain.GrantState{
				DurationSeconds: 0,
			},
			nowMono: 1,
			nowWall: baseWall,
			want:    true,
		},
	}

	for _, tc := range tests {
		if got := isExpired(tc.grant, tc.nowMono, tc.nowWall); got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestCleanupRunStartupSweep(t *testing.T) {
	store := &stubStore{
		grants: []domain.GrantState{
			{RequestID: "expired", WorkflowID: "wf", Username: "alice", GrantedAtMonoNS: 1, GrantedAtWallUTC: time.Date(2026, 2, 22, 11, 59, 58, 0, time.UTC), DurationSeconds: 1},
			{RequestID: "active", WorkflowID: "wf", Username: "bob", GrantedAtMonoNS: 1000, GrantedAtWallUTC: time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC), DurationSeconds: 3600},
		},
	}
	engine := &stubGrantEngine{}
	clock := &stubClock{
		wall: time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	}
	clock.mono.Store(2_000_000_000)

	runner, err := NewCleanupRunner(store, engine, clock, 10*time.Millisecond, 24*time.Hour, slog.Default())
	if err != nil {
		t.Fatalf("NewCleanupRunner failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := runner.Run(ctx); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if engine.revokedCount() != 1 {
		t.Fatalf("expected 1 revoke, got %d", engine.revokedCount())
	}

	left, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(left) != 2 {
		t.Fatalf("unexpected grants left: %+v", left)
	}
	expired, foundExpired := findGrantStateByID(left, "expired")
	active, foundActive := findGrantStateByID(left, "active")
	if !foundExpired || !foundActive {
		t.Fatalf("unexpected grants left: %+v", left)
	}
	if expired.CompletedAtWallUTC.IsZero() {
		t.Fatalf("expected expired grant to be retained as completed: %+v", expired)
	}
	if !active.CompletedAtWallUTC.IsZero() {
		t.Fatalf("expected active grant to remain active: %+v", active)
	}
}

func TestCleanupRunPeriodicSweep(t *testing.T) {
	store := &stubStore{
		grants: []domain.GrantState{
			{
				RequestID:        "will-expire",
				WorkflowID:       "wf",
				Username:         "alice",
				GrantedAtMonoNS:  1,
				GrantedAtWallUTC: time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
				DurationSeconds:  1,
			},
		},
	}
	engine := &stubGrantEngine{revokeC: make(chan string, 1)}
	clock := &stubClock{
		wall: time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	}
	clock.mono.Store(0)

	runner, err := NewCleanupRunner(store, engine, clock, 10*time.Millisecond, 24*time.Hour, slog.Default())
	if err != nil {
		t.Fatalf("NewCleanupRunner failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	clock.mono.Store(2 * int64(time.Second))

	select {
	case requestID := <-engine.revokeC:
		if requestID != "will-expire" {
			t.Fatalf("unexpected revoke request id: got %q want %q", requestID, "will-expire")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for periodic revoke")
	}

	cancel()

	if err := <-done; err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if engine.revokedCount() == 0 {
		t.Fatal("expected periodic revoke to occur")
	}
}

func TestCleanupPurgesCompletedGrantAfterRetention(t *testing.T) {
	completedAt := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	store := &stubStore{
		grants: []domain.GrantState{
			{
				RequestID:          "completed",
				WorkflowID:         "wf",
				Username:           "alice",
				GrantedAtMonoNS:    1,
				GrantedAtWallUTC:   completedAt.Add(-time.Hour),
				DurationSeconds:    60,
				CompletedAtWallUTC: completedAt,
			},
		},
	}
	engine := &stubGrantEngine{}
	clock := &stubClock{wall: completedAt.Add(25 * time.Hour)}
	clock.mono.Store(1)

	runner, err := NewCleanupRunner(store, engine, clock, time.Minute, 24*time.Hour, slog.Default())
	if err != nil {
		t.Fatalf("NewCleanupRunner failed: %v", err)
	}

	if err := runner.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce failed: %v", err)
	}

	left, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(left) != 0 {
		t.Fatalf("expected retained grant to be purged, got %+v", left)
	}
}

func findGrantStateByID(grants []domain.GrantState, requestID string) (domain.GrantState, bool) {
	for _, grant := range grants {
		if grant.RequestID == requestID {
			return grant, true
		}
	}
	return domain.GrantState{}, false
}
