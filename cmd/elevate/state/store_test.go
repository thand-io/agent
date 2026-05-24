package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/thand-io/agent/cmd/elevate/domain"
)

func TestFileStorePutListDeleteAndRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	storeA, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}

	grant := domain.GrantState{
		RequestID:            "req-1",
		WorkflowID:           "wf-1",
		Username:             "alice",
		GrantedAtWallUTC:     time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC),
		GrantedAtMonoNS:      100,
		DurationSeconds:      60,
		WasAlreadyPrivileged: false,
	}

	if err := storeA.Put(context.Background(), grant); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, err := storeA.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(got) != 1 || got[0].RequestID != "req-1" {
		t.Fatalf("unexpected list after put: %+v", got)
	}

	// Simulate process restart by creating a new store instance on same path.
	storeB, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore (recovery) failed: %v", err)
	}
	recovered, err := storeB.List(context.Background())
	if err != nil {
		t.Fatalf("List after recovery failed: %v", err)
	}
	if len(recovered) != 1 || recovered[0].RequestID != "req-1" {
		t.Fatalf("unexpected recovered state: %+v", recovered)
	}

	if err := storeB.Delete(context.Background(), "req-1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	empty, err := storeB.List(context.Background())
	if err != nil {
		t.Fatalf("List after delete failed: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty state after delete, got %+v", empty)
	}
}

func TestFileStoreDeleteMissingIsNoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}

	if err := store.Delete(context.Background(), "missing"); err != nil {
		t.Fatalf("Delete missing should be nil, got %v", err)
	}
}

func TestFileStoreDeleteRequiresExactRequestID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}

	grant := domain.GrantState{
		RequestID:            "req-1",
		WorkflowID:           "wf-1",
		Username:             "alice",
		GrantedAtWallUTC:     time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC),
		GrantedAtMonoNS:      100,
		DurationSeconds:      60,
		WasAlreadyPrivileged: false,
	}
	if err := store.Put(context.Background(), grant); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	if err := store.Delete(context.Background(), " req-1 "); err != nil {
		t.Fatalf("Delete with non-matching request ID should be nil, got %v", err)
	}

	got, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(got) != 1 || got[0].RequestID != "req-1" {
		t.Fatalf("expected exact request ID match to be required, got %+v", got)
	}
}
