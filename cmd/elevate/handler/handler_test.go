package handler

import (
	"context"
	"testing"

	"github.com/thand-io/agent/cmd/elevate/domain"
)

func TestHandleConnectionUnsupportedActionWritesProtocolError(t *testing.T) {
	req := domain.RequestFrame{
		Type:       domain.FrameTypeRequest,
		Action:     domain.Action("unknown"),
		WorkflowID: "wf-1",
		RequestID:  "req-unsupported",
		Username:   "alice",
	}

	conn := &stubConn{
		readFrames: [][]byte{
			mustJSON(t, req),
		},
	}
	h := New(&stubGrantEngine{}, &stubVerifier{}, &stubStateStore{}, stubClock{})

	if err := h.HandleConnection(context.Background(), conn); err != nil {
		t.Fatalf("HandleConnection failed: %v", err)
	}

	if len(conn.writeFrames) != 1 {
		t.Fatalf("expected 1 result frame, got %d writes", len(conn.writeFrames))
	}
	assertResultErrorCode(t, conn.writeFrames[0], ErrorCodeInvalidRequest)
}
