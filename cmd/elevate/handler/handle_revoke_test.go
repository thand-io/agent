package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/thand-io/agent/cmd/elevate/domain"
)

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

func TestHandleConnectionRevokeDeletesStateWhenCompletionPersistFails(t *testing.T) {
	req := domain.RequestFrame{
		Type:       domain.FrameTypeRequest,
		Action:     domain.ActionRevoke,
		WorkflowID: "wf-1",
		RequestID:  "req-2",
		Username:   "alice",
	}
	nonce := "fixed-nonce-revoke-delete-fallback"

	conn := &stubConn{
		readFrames: [][]byte{
			mustJSON(t, req),
			mustJSON(t, signedResponseFor(t, req, nonce)),
		},
	}
	grantEngine := &stubGrantEngine{}
	state := &stubStateStore{
		putErr: errors.New("put failed"),
		grants: []domain.GrantState{{
			RequestID:        req.RequestID,
			WorkflowID:       req.WorkflowID,
			Username:         req.Username,
			GrantedAtWallUTC: time.Now().UTC(),
			GrantedAtMonoNS:  1,
			DurationSeconds:  60,
		}},
	}
	h := New(grantEngine, &stubVerifier{}, state, stubClock{wall: time.Now().UTC()})
	h.generateNonce = func() (string, error) { return nonce, nil }

	if err := h.HandleConnection(context.Background(), conn); err != nil {
		t.Fatalf("HandleConnection failed: %v", err)
	}
	if grantEngine.revokeCalls != 1 {
		t.Fatalf("expected 1 revoke call, got %d", grantEngine.revokeCalls)
	}
	if state.deleteCalls != 1 {
		t.Fatalf("expected delete fallback after failed completion put, got %d delete calls", state.deleteCalls)
	}
	if len(state.grants) != 0 {
		t.Fatalf("expected state to be deleted after fallback, got %+v", state.grants)
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

func TestHandleConnectionRevokeRejectsUnsafeUsername(t *testing.T) {
	req := domain.RequestFrame{
		Type:       domain.FrameTypeRequest,
		Action:     domain.ActionRevoke,
		WorkflowID: "wf-1",
		RequestID:  "req-unsafe-revoke",
		Username:   " alice ",
	}
	nonce := "fixed-nonce-unsafe-revoke"

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
	if grantEngine.revokeCalls != 0 {
		t.Fatalf("expected unsafe username to be rejected before revoke, got %d revoke calls", grantEngine.revokeCalls)
	}

	assertChallengeAndResult(t, conn.writeFrames, nonce, req.RequestID, resultStatusError)
	assertResultErrorCode(t, conn.writeFrames[1], ErrorCodeInvalidRequest)
}

func TestHandleConnectionRevokeRejectsOverlongUsername(t *testing.T) {
	req := domain.RequestFrame{
		Type:       domain.FrameTypeRequest,
		Action:     domain.ActionRevoke,
		WorkflowID: "wf-1",
		RequestID:  "req-long-user",
		Username:   "abcdefghijklmnopqrstuvwxyzabcdefg",
	}
	nonce := "fixed-nonce-long-revoke"

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
	if grantEngine.revokeCalls != 0 {
		t.Fatalf("expected overlong username to be rejected before revoke, got %d revoke calls", grantEngine.revokeCalls)
	}

	assertChallengeAndResult(t, conn.writeFrames, nonce, req.RequestID, resultStatusError)
	assertResultErrorCode(t, conn.writeFrames[1], ErrorCodeInvalidRequest)
}
