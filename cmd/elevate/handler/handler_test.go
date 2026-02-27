package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/thand-io/agent/cmd/elevate/domain"
	"github.com/thand-io/agent/cmd/elevate/verify"
)

type stubConn struct {
	readFrames  [][]byte
	readIndex   int
	writeFrames [][]byte
}

func (c *stubConn) ReadFrame(ctx context.Context) ([]byte, error) {
	_ = ctx
	if c.readIndex >= len(c.readFrames) {
		return nil, errors.New("no more read frames")
	}
	out := c.readFrames[c.readIndex]
	c.readIndex++
	return out, nil
}

func (c *stubConn) WriteFrame(ctx context.Context, data []byte) error {
	_ = ctx
	c.writeFrames = append(c.writeFrames, data)
	return nil
}

func (c *stubConn) Close() error { return nil }

type stubGrantEngine struct {
	grantCalls  int
	revokeCalls int
	grantResult domain.GrantResult
	grantErr    error
	revokeErr   error
}

func (s *stubGrantEngine) Grant(ctx context.Context, req domain.GrantRequest) (domain.GrantResult, error) {
	_ = ctx
	_ = req
	s.grantCalls++
	return s.grantResult, s.grantErr
}

func (s *stubGrantEngine) Revoke(ctx context.Context, req domain.RevokeRequest) error {
	_ = ctx
	_ = req
	s.revokeCalls++
	return s.revokeErr
}

type stubVerifier struct {
	err error
}

func (s *stubVerifier) Verify(keyID string, payload []byte, signature []byte) error {
	_ = keyID
	_ = payload
	_ = signature
	return s.err
}

type stubStateStore struct {
	putCalls    int
	deleteCalls int
	putErr      error
	deleteErr   error
	grants      []domain.GrantState
}

func (s *stubStateStore) Put(ctx context.Context, grant domain.GrantState) error {
	_ = ctx
	s.putCalls++
	for i := range s.grants {
		if s.grants[i].RequestID == grant.RequestID {
			s.grants[i] = grant
			return s.putErr
		}
	}
	s.grants = append(s.grants, grant)
	return s.putErr
}

func (s *stubStateStore) Delete(ctx context.Context, requestID string) error {
	_ = ctx
	_ = requestID
	s.deleteCalls++
	return s.deleteErr
}

func (s *stubStateStore) List(ctx context.Context) ([]domain.GrantState, error) {
	_ = ctx
	out := make([]domain.GrantState, len(s.grants))
	copy(out, s.grants)
	return out, nil
}

type stubClock struct {
	mono int64
	wall time.Time
}

func (c stubClock) NowMonoNS() int64      { return c.mono }
func (c stubClock) NowWallUTC() time.Time { return c.wall }

func TestHandleConnectionGrantSuccess(t *testing.T) {
	req := domain.RequestFrame{
		Type:            domain.FrameTypeRequest,
		Action:          domain.ActionGrant,
		WorkflowID:      "wf-1",
		RequestID:       "req-1",
		Username:        "alice",
		DurationSeconds: 60,
	}
	nonce := "fixed-nonce"

	conn := &stubConn{
		readFrames: [][]byte{
			mustJSON(t, req),
			mustJSON(t, signedResponseFor(t, req, nonce)),
		},
	}
	grantEngine := &stubGrantEngine{grantResult: domain.GrantResult{WasAlreadyPrivileged: true}}
	state := &stubStateStore{}
	h := New(grantEngine, &stubVerifier{}, state, stubClock{
		mono: 123,
		wall: time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC),
	})
	h.generateNonce = func() (string, error) { return nonce, nil }

	if err := h.HandleConnection(context.Background(), conn); err != nil {
		t.Fatalf("HandleConnection failed: %v", err)
	}
	if grantEngine.grantCalls != 1 {
		t.Fatalf("expected 1 grant call, got %d", grantEngine.grantCalls)
	}
	if state.putCalls != 1 {
		t.Fatalf("expected 1 state put, got %d", state.putCalls)
	}
	assertChallengeAndResult(t, conn.writeFrames, nonce, req.RequestID, resultStatusOK)
}

func TestHandleConnectionRevokeSuccess(t *testing.T) {
	req := domain.RequestFrame{
		Type:       domain.FrameTypeRequest,
		Action:     domain.ActionRevoke,
		WorkflowID: "wf-1",
		RequestID:  "req-2",
		Username:   "alice",
	}
	nonce := "fixed-nonce-revoke"

	conn := &stubConn{
		readFrames: [][]byte{
			mustJSON(t, req),
			mustJSON(t, signedResponseFor(t, req, nonce)),
		},
	}
	grantEngine := &stubGrantEngine{}
	state := &stubStateStore{grants: []domain.GrantState{{
		RequestID:        req.RequestID,
		WorkflowID:       req.WorkflowID,
		Username:         req.Username,
		GrantedAtWallUTC: time.Now().UTC(),
		GrantedAtMonoNS:  1,
		DurationSeconds:  60,
	}}}
	h := New(grantEngine, &stubVerifier{}, state, stubClock{wall: time.Now().UTC()})
	h.generateNonce = func() (string, error) { return nonce, nil }

	if err := h.HandleConnection(context.Background(), conn); err != nil {
		t.Fatalf("HandleConnection failed: %v", err)
	}
	if grantEngine.revokeCalls != 1 {
		t.Fatalf("expected 1 revoke call, got %d", grantEngine.revokeCalls)
	}
	if state.putCalls != 1 {
		t.Fatalf("expected 1 state put, got %d", state.putCalls)
	}
	if len(state.grants) != 1 || state.grants[0].CompletedAtWallUTC.IsZero() {
		t.Fatalf("expected revoke to mark stored grant completed, got %+v", state.grants)
	}
	assertChallengeAndResult(t, conn.writeFrames, nonce, req.RequestID, resultStatusOK)
}

func TestHandleConnectionRevokeWithoutStateStillRevokes(t *testing.T) {
	req := domain.RequestFrame{
		Type:       domain.FrameTypeRequest,
		Action:     domain.ActionRevoke,
		WorkflowID: "wf-1",
		RequestID:  "req-missing",
		Username:   "alice",
	}
	nonce := "fixed-nonce-revoke-missing"

	conn := &stubConn{
		readFrames: [][]byte{
			mustJSON(t, req),
			mustJSON(t, signedResponseFor(t, req, nonce)),
		},
	}
	grantEngine := &stubGrantEngine{}
	state := &stubStateStore{}
	h := New(grantEngine, &stubVerifier{}, state, stubClock{wall: time.Now().UTC()})
	h.generateNonce = func() (string, error) { return nonce, nil }

	if err := h.HandleConnection(context.Background(), conn); err != nil {
		t.Fatalf("HandleConnection failed: %v", err)
	}
	if grantEngine.revokeCalls != 1 {
		t.Fatalf("expected revoke without state to still revoke, got %d", grantEngine.revokeCalls)
	}
	assertChallengeAndResult(t, conn.writeFrames, nonce, req.RequestID, resultStatusOK)
}

func TestHandleConnectionRevokeSkipsPrivilegeRemovalForBaselineGrant(t *testing.T) {
	req := domain.RequestFrame{
		Type:       domain.FrameTypeRequest,
		Action:     domain.ActionRevoke,
		WorkflowID: "wf-1",
		RequestID:  "req-baseline",
		Username:   "alice",
	}
	nonce := "fixed-nonce-revoke-baseline"

	conn := &stubConn{
		readFrames: [][]byte{
			mustJSON(t, req),
			mustJSON(t, signedResponseFor(t, req, nonce)),
		},
	}
	grantEngine := &stubGrantEngine{}
	state := &stubStateStore{grants: []domain.GrantState{{
		RequestID:            req.RequestID,
		WorkflowID:           req.WorkflowID,
		Username:             req.Username,
		GrantedAtWallUTC:     time.Now().UTC(),
		GrantedAtMonoNS:      1,
		DurationSeconds:      60,
		WasAlreadyPrivileged: true,
	}}}
	h := New(grantEngine, &stubVerifier{}, state, stubClock{wall: time.Now().UTC()})
	h.generateNonce = func() (string, error) { return nonce, nil }

	if err := h.HandleConnection(context.Background(), conn); err != nil {
		t.Fatalf("HandleConnection failed: %v", err)
	}
	if grantEngine.revokeCalls != 0 {
		t.Fatalf("expected baseline revoke to skip privilege removal, got %d", grantEngine.revokeCalls)
	}
	if len(state.grants) != 1 || state.grants[0].CompletedAtWallUTC.IsZero() {
		t.Fatalf("expected baseline grant to be marked completed, got %+v", state.grants)
	}
	assertChallengeAndResult(t, conn.writeFrames, nonce, req.RequestID, resultStatusOK)
}

func TestHandleConnectionPayloadMismatchWritesError(t *testing.T) {
	req := domain.RequestFrame{
		Type:            domain.FrameTypeRequest,
		Action:          domain.ActionGrant,
		WorkflowID:      "wf-1",
		RequestID:       "req-3",
		Username:        "alice",
		DurationSeconds: 60,
	}
	nonce := "fixed-nonce-mismatch"
	bad := req
	bad.Username = "mallory"

	conn := &stubConn{
		readFrames: [][]byte{
			mustJSON(t, req),
			mustJSON(t, signedResponseFor(t, bad, nonce)),
		},
	}
	grantEngine := &stubGrantEngine{}
	state := &stubStateStore{}
	h := New(grantEngine, &stubVerifier{}, state, stubClock{})
	h.generateNonce = func() (string, error) { return nonce, nil }

	if err := h.HandleConnection(context.Background(), conn); err != nil {
		t.Fatalf("HandleConnection failed: %v", err)
	}
	if grantEngine.grantCalls != 0 {
		t.Fatalf("expected no grant call, got %d", grantEngine.grantCalls)
	}

	assertChallengeAndResult(t, conn.writeFrames, nonce, req.RequestID, resultStatusError)

	var result domain.ResultFrame
	if err := json.Unmarshal(conn.writeFrames[1], &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Error != string(ErrorCodeUnauthorized) {
		t.Fatalf("unexpected error code: got %q want %q", result.Error, ErrorCodeUnauthorized)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func signedResponseFor(t *testing.T, req domain.RequestFrame, nonce string) domain.SignedResponseFrame {
	t.Helper()
	payload := verify.SignedPayload{
		Nonce:           nonce,
		Action:          string(req.Action),
		WorkflowID:      req.WorkflowID,
		RequestID:       req.RequestID,
		Username:        req.Username,
		DurationSeconds: req.DurationSeconds,
	}
	canonical, err := verify.CanonicalPayload(payload)
	if err != nil {
		t.Fatalf("canonical payload: %v", err)
	}
	return domain.SignedResponseFrame{
		Type:          domain.FrameTypeSignedResponse,
		KeyID:         "test-key",
		Signature:     base64.StdEncoding.EncodeToString([]byte{0}),
		SignedPayload: base64.StdEncoding.EncodeToString(canonical),
	}
}

func assertChallengeAndResult(t *testing.T, writes [][]byte, expectedNonce, expectedRequestID, expectedStatus string) {
	t.Helper()
	if len(writes) != 2 {
		t.Fatalf("expected 2 writes (challenge+result), got %d", len(writes))
	}

	var challenge domain.ChallengeFrame
	if err := json.Unmarshal(writes[0], &challenge); err != nil {
		t.Fatalf("decode challenge: %v", err)
	}
	if challenge.Type != domain.FrameTypeChallenge || challenge.Nonce != expectedNonce {
		t.Fatalf("unexpected challenge: %+v", challenge)
	}

	var result domain.ResultFrame
	if err := json.Unmarshal(writes[1], &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Type != domain.FrameTypeResult {
		t.Fatalf("unexpected result type: %s", result.Type)
	}
	if result.RequestID != expectedRequestID {
		t.Fatalf("unexpected result request_id: got %s want %s", result.RequestID, expectedRequestID)
	}
	if result.Status != expectedStatus {
		t.Fatalf("unexpected result status: got %s want %s", result.Status, expectedStatus)
	}
}
