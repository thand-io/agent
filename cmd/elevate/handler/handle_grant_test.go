package handler

import (
	"context"
	"testing"
	"time"

	"github.com/thand-io/agent/cmd/elevate/domain"
)

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

func TestHandleConnectionGrantRejectsDuplicateRequestID(t *testing.T) {
	req := domain.RequestFrame{
		Type:            domain.FrameTypeRequest,
		Action:          domain.ActionGrant,
		WorkflowID:      "wf-1",
		RequestID:       "req-dup",
		Username:        "alice",
		DurationSeconds: 60,
	}
	nonce := "fixed-nonce-dup"

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
		DurationSeconds:  req.DurationSeconds,
	}}}
	h := New(grantEngine, &stubVerifier{}, state, stubClock{mono: 123, wall: time.Now().UTC()})
	h.generateNonce = func() (string, error) { return nonce, nil }

	if err := h.HandleConnection(context.Background(), conn); err != nil {
		t.Fatalf("HandleConnection failed: %v", err)
	}
	if grantEngine.grantCalls != 0 {
		t.Fatalf("expected duplicate request replay to skip grant, got %d grant calls", grantEngine.grantCalls)
	}

	assertChallengeAndResult(t, conn.writeFrames, nonce, req.RequestID, resultStatusOK)
}

func TestHandleConnectionGrantRejectsConflictingDuplicateRequestID(t *testing.T) {
	req := domain.RequestFrame{
		Type:            domain.FrameTypeRequest,
		Action:          domain.ActionGrant,
		WorkflowID:      "wf-1",
		RequestID:       "req-dup-conflict",
		Username:        "alice",
		DurationSeconds: 60,
	}
	nonce := "fixed-nonce-dup-conflict"

	conn := &stubConn{
		readFrames: [][]byte{
			mustJSON(t, req),
			mustJSON(t, signedResponseFor(t, req, nonce)),
		},
	}
	grantEngine := &stubGrantEngine{}
	state := &stubStateStore{grants: []domain.GrantState{{
		RequestID:        req.RequestID,
		WorkflowID:       "wf-existing",
		Username:         "bob",
		GrantedAtWallUTC: time.Now().UTC(),
		GrantedAtMonoNS:  1,
		DurationSeconds:  60,
	}}}
	h := New(grantEngine, &stubVerifier{}, state, stubClock{mono: 123, wall: time.Now().UTC()})
	h.generateNonce = func() (string, error) { return nonce, nil }

	if err := h.HandleConnection(context.Background(), conn); err != nil {
		t.Fatalf("HandleConnection failed: %v", err)
	}
	if grantEngine.grantCalls != 0 {
		t.Fatalf("expected conflicting duplicate request ID to be rejected before grant, got %d grant calls", grantEngine.grantCalls)
	}

	assertChallengeAndResult(t, conn.writeFrames, nonce, req.RequestID, resultStatusError)
	assertResultErrorCode(t, conn.writeFrames[1], ErrorCodeRequestConflict)
}

func TestHandleConnectionGrantRejectsActiveGrantForSameUser(t *testing.T) {
	req := domain.RequestFrame{
		Type:            domain.FrameTypeRequest,
		Action:          domain.ActionGrant,
		WorkflowID:      "wf-1",
		RequestID:       "req-new",
		Username:        "alice",
		DurationSeconds: 60,
	}
	nonce := "fixed-nonce-user-active"

	conn := &stubConn{
		readFrames: [][]byte{
			mustJSON(t, req),
			mustJSON(t, signedResponseFor(t, req, nonce)),
		},
	}
	grantEngine := &stubGrantEngine{}
	state := &stubStateStore{grants: []domain.GrantState{{
		RequestID:        "req-existing",
		WorkflowID:       "wf-existing",
		Username:         req.Username,
		GrantedAtWallUTC: time.Now().UTC(),
		GrantedAtMonoNS:  1,
		DurationSeconds:  60,
	}}}
	h := New(grantEngine, &stubVerifier{}, state, stubClock{mono: 123, wall: time.Now().UTC()})
	h.generateNonce = func() (string, error) { return nonce, nil }

	if err := h.HandleConnection(context.Background(), conn); err != nil {
		t.Fatalf("HandleConnection failed: %v", err)
	}
	if grantEngine.grantCalls != 0 {
		t.Fatalf("expected active same-user grant to be rejected before grant, got %d grant calls", grantEngine.grantCalls)
	}

	assertChallengeAndResult(t, conn.writeFrames, nonce, req.RequestID, resultStatusError)
	assertResultErrorCode(t, conn.writeFrames[1], ErrorCodeActiveGrantExists)
}

func TestHandleConnectionGrantPrefersMatchingRequestIDOverEarlierActiveUserGrant(t *testing.T) {
	req := domain.RequestFrame{
		Type:            domain.FrameTypeRequest,
		Action:          domain.ActionGrant,
		WorkflowID:      "wf-1",
		RequestID:       "req-dup",
		Username:        "alice",
		DurationSeconds: 60,
	}
	nonce := "fixed-nonce-dup-order"

	conn := &stubConn{
		readFrames: [][]byte{
			mustJSON(t, req),
			mustJSON(t, signedResponseFor(t, req, nonce)),
		},
	}
	grantEngine := &stubGrantEngine{}
	state := &stubStateStore{grants: []domain.GrantState{
		{
			RequestID:        "req-existing",
			WorkflowID:       "wf-existing",
			Username:         req.Username,
			GrantedAtWallUTC: time.Now().UTC(),
			GrantedAtMonoNS:  1,
			DurationSeconds:  60,
		},
		{
			RequestID:        req.RequestID,
			WorkflowID:       req.WorkflowID,
			Username:         req.Username,
			GrantedAtWallUTC: time.Now().UTC(),
			GrantedAtMonoNS:  1,
			DurationSeconds:  req.DurationSeconds,
		},
	}}
	h := New(grantEngine, &stubVerifier{}, state, stubClock{mono: 123, wall: time.Now().UTC()})
	h.generateNonce = func() (string, error) { return nonce, nil }

	if err := h.HandleConnection(context.Background(), conn); err != nil {
		t.Fatalf("HandleConnection failed: %v", err)
	}
	if grantEngine.grantCalls != 0 {
		t.Fatalf("expected duplicate request replay to skip grant, got %d grant calls", grantEngine.grantCalls)
	}

	assertChallengeAndResult(t, conn.writeFrames, nonce, req.RequestID, resultStatusOK)
}

func TestHandleConnectionGrantAllowsNewGrantWhenExistingOneIsCompleted(t *testing.T) {
	req := domain.RequestFrame{
		Type:            domain.FrameTypeRequest,
		Action:          domain.ActionGrant,
		WorkflowID:      "wf-1",
		RequestID:       "req-new",
		Username:        "alice",
		DurationSeconds: 60,
	}
	nonce := "fixed-nonce-user-completed"

	conn := &stubConn{
		readFrames: [][]byte{
			mustJSON(t, req),
			mustJSON(t, signedResponseFor(t, req, nonce)),
		},
	}
	grantEngine := &stubGrantEngine{grantResult: domain.GrantResult{}}
	state := &stubStateStore{grants: []domain.GrantState{{
		RequestID:          "req-old",
		WorkflowID:         "wf-existing",
		Username:           req.Username,
		GrantedAtWallUTC:   time.Now().UTC(),
		GrantedAtMonoNS:    1,
		DurationSeconds:    60,
		CompletedAtWallUTC: time.Now().UTC(),
	}}}
	h := New(grantEngine, &stubVerifier{}, state, stubClock{mono: 123, wall: time.Now().UTC()})
	h.generateNonce = func() (string, error) { return nonce, nil }

	if err := h.HandleConnection(context.Background(), conn); err != nil {
		t.Fatalf("HandleConnection failed: %v", err)
	}
	if grantEngine.grantCalls != 1 {
		t.Fatalf("expected completed grant not to block new grant, got %d grant calls", grantEngine.grantCalls)
	}
	assertChallengeAndResult(t, conn.writeFrames, nonce, req.RequestID, resultStatusOK)
}

func TestHandleConnectionGrantRejectsUnsafeUsername(t *testing.T) {
	req := domain.RequestFrame{
		Type:            domain.FrameTypeRequest,
		Action:          domain.ActionGrant,
		WorkflowID:      "wf-1",
		RequestID:       "req-unsafe-user",
		Username:        "/delete",
		DurationSeconds: 60,
	}
	nonce := "fixed-nonce-unsafe-grant"

	conn := &stubConn{
		readFrames: [][]byte{
			mustJSON(t, req),
			mustJSON(t, signedResponseFor(t, req, nonce)),
		},
	}
	grantEngine := &stubGrantEngine{}
	state := &stubStateStore{}
	h := New(grantEngine, &stubVerifier{}, state, stubClock{mono: 123, wall: time.Now().UTC()})
	h.generateNonce = func() (string, error) { return nonce, nil }

	if err := h.HandleConnection(context.Background(), conn); err != nil {
		t.Fatalf("HandleConnection failed: %v", err)
	}
	if grantEngine.grantCalls != 0 {
		t.Fatalf("expected unsafe username to be rejected before grant, got %d grant calls", grantEngine.grantCalls)
	}

	assertChallengeAndResult(t, conn.writeFrames, nonce, req.RequestID, resultStatusError)
	assertResultErrorCode(t, conn.writeFrames[1], ErrorCodeInvalidRequest)
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
	assertResultErrorCode(t, conn.writeFrames[1], ErrorCodeUnauthorized)
}
