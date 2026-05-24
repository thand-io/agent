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
	s.deleteCalls++
	if s.deleteErr != nil {
		return s.deleteErr
	}
	filtered := s.grants[:0]
	for _, grant := range s.grants {
		if grant.RequestID != requestID {
			filtered = append(filtered, grant)
		}
	}
	s.grants = filtered
	return nil
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

func assertResultErrorCode(t *testing.T, frame []byte, expected ErrorCode) {
	t.Helper()
	var result domain.ResultFrame
	if err := json.Unmarshal(frame, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Error != string(expected) {
		t.Fatalf("unexpected error code: got %q want %q", result.Error, expected)
	}
}
