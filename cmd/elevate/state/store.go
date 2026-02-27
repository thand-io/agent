package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/thand-io/agent/cmd/elevate/domain"
	"github.com/thand-io/agent/cmd/elevate/handler"
)

// stateSchemaVersion guards on-disk compatibility for the state file.
const stateSchemaVersion = 1

// fileState is the on-disk JSON representation for persisted grants.
type fileState struct {
	Version int                 `json:"version"`
	Grants  []domain.GrantState `json:"grants"`
}

// FileStore persists active grants to a local JSON file using atomic writes.
type FileStore struct {
	path string
	mu   sync.Mutex
}

// NewFileStore constructs a StateStore backed by a single JSON file.
func NewFileStore(path string) (handler.StateStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("state path is required")
	}

	return &FileStore{path: path}, nil
}

// Put upserts a grant state by request ID and atomically persists it.
func (s *FileStore) Put(ctx context.Context, grant domain.GrantState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(grant.RequestID) == "" {
		return errors.New("request ID is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.readState()
	if err != nil {
		return err
	}

	found := false
	for i := range st.Grants {
		if st.Grants[i].RequestID == grant.RequestID {
			st.Grants[i] = grant
			found = true
			break
		}
	}
	if !found {
		st.Grants = append(st.Grants, grant)
	}

	return s.writeState(st)
}

// Delete removes a grant state by request ID and atomically persists.
func (s *FileStore) Delete(ctx context.Context, requestID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return errors.New("request ID is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.readState()
	if err != nil {
		return err
	}

	filtered := st.Grants[:0]
	for _, g := range st.Grants {
		if g.RequestID != requestID {
			filtered = append(filtered, g)
		}
	}
	st.Grants = filtered

	return s.writeState(st)
}

// List returns all persisted grant states.
func (s *FileStore) List(ctx context.Context) ([]domain.GrantState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.readState()
	if err != nil {
		return nil, err
	}

	out := make([]domain.GrantState, len(st.Grants))
	copy(out, st.Grants)
	return out, nil
}

// readState loads and validates the on-disk state payload.
func (s *FileStore) readState() (*fileState, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return &fileState{Version: stateSchemaVersion, Grants: []domain.GrantState{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state file: %w", err)
	}

	var st fileState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, fmt.Errorf("decode state file: %w", err)
	}
	if st.Version != stateSchemaVersion {
		return nil, fmt.Errorf("unsupported state version: %d", st.Version)
	}
	if st.Grants == nil {
		st.Grants = []domain.GrantState{}
	}
	return &st, nil
}

// writeState persists state using temp-file + fsync + rename for atomic updates.
func (s *FileStore) writeState(st *fileState) error {
	st.Version = stateSchemaVersion

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	b, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("encode state file: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp state file: %w", err)
	}
	tmpPath := tmpFile.Name()
	keepTmp := true
	defer func() {
		if keepTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(b); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temp state file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("sync temp state file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp state file: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("rename temp state file: %w", err)
	}
	keepTmp = false

	if err := syncStateDir(dir); err != nil {
		return fmt.Errorf("sync state directory: %w", err)
	}

	return nil
}
