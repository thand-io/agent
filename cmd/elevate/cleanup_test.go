package main

import (
	"context"
	"errors"
	"sync"
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
	return nil
}

func (g *stubGrantEngine) revokedCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.revoked)
}

type stubClock struct {
	mono int64
	wall time.Time
}

func (c *stubClock) NowMonoNS() int64      { return c.mono }
func (c *stubClock) NowWallUTC() time.Time { return c.wall }

func TestIsExpiredMonotonicAndWallFallback(t *testing.T) {
	baseWall := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		grant   domain.GrantState
		nowMono int64
		nowWall time.Time
		want    bool
	}{
		{
			name: "monotonic not expired",
			grant: domain.GrantState{
				GrantedAtMonoNS: 100,
				DurationSeconds: 10,
			},
			nowMono: 105,
			nowWall: baseWall,
			want:    false,
		},
		{
			name: "monotonic expired",
			grant: domain.GrantState{
				GrantedAtMonoNS: 100,
				DurationSeconds: 10,
			},
			nowMono: 100 + int64(10*time.Second),
			nowWall: baseWall,
			want:    true,
		},
		{
			name: "reboot fallback to wall clock",
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
			{RequestID: "expired", WorkflowID: "wf", Username: "alice", GrantedAtMonoNS: 1, DurationSeconds: 1},
			{RequestID: "active", WorkflowID: "wf", Username: "bob", GrantedAtMonoNS: 1000, DurationSeconds: 3600},
		},
	}
	engine := &stubGrantEngine{}
	clock := &stubClock{
		mono: 2_000_000_000,
		wall: time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	}

	runner, err := NewCleanupRunner(store, engine, clock, 10*time.Millisecond)
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
	if len(left) != 1 || left[0].RequestID != "active" {
		t.Fatalf("unexpected grants left: %+v", left)
	}
}

func TestCleanupRunPeriodicSweep(t *testing.T) {
	store := &stubStore{
		grants: []domain.GrantState{
			{
				RequestID:       "will-expire",
				WorkflowID:      "wf",
				Username:        "alice",
				GrantedAtMonoNS: 1,
				DurationSeconds: 1,
			},
		},
	}
	engine := &stubGrantEngine{}
	clock := &stubClock{
		mono: 0,
		wall: time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	}

	runner, err := NewCleanupRunner(store, engine, clock, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("NewCleanupRunner failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	time.Sleep(20 * time.Millisecond)
	clock.mono = 2 * int64(time.Second)
	time.Sleep(20 * time.Millisecond)
	cancel()

	if err := <-done; err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if engine.revokedCount() == 0 {
		t.Fatal("expected periodic revoke to occur")
	}
}
