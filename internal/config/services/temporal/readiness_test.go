package temporal

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// newTestClient creates a minimal TemporalClient suitable for readiness tests.
// It does NOT call Initialize() — callers control readiness explicitly.
func newTestClient() *TemporalClient {
	return NewTemporalClient(
		&models.TemporalConfig{
			Host:              "localhost",
			Port:              7233,
			Namespace:         "default",
			DisableVersioning: true,
		},
		nil,
		"test-identity",
	)
}

func TestGetClient_BlocksUntilReady(t *testing.T) {
	t.Parallel()
	tc := newTestClient()

	done := make(chan struct{})
	go func() {
		_ = tc.GetClient() // should block
		close(done)
	}()

	// Verify it really is blocking
	select {
	case <-done:
		t.Fatal("GetClient returned before markReady was called")
	case <-time.After(100 * time.Millisecond):
		// still blocked — good
	}

	tc.markReady()

	select {
	case <-done:
		// unblocked — good
	case <-time.After(2 * time.Second):
		t.Fatal("GetClient did not unblock after markReady")
	}
}

func TestGetClient_ImmediateWhenAlreadyReady(t *testing.T) {
	t.Parallel()
	tc := newTestClient()
	tc.markReady()

	done := make(chan struct{})
	go func() {
		_ = tc.GetClient()
		close(done)
	}()

	select {
	case <-done:
		// returned immediately — good
	case <-time.After(2 * time.Second):
		t.Fatal("GetClient blocked even though client was already ready")
	}
}

func TestGetClient_MultipleWaiters(t *testing.T) {
	t.Parallel()
	tc := newTestClient()

	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)

	var unblocked int64

	for range n {
		go func() {
			defer wg.Done()
			_ = tc.GetClient()
			atomic.AddInt64(&unblocked, 1)
		}()
	}

	// None should have returned yet
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, int64(0), atomic.LoadInt64(&unblocked),
		"some waiters returned before markReady")

	tc.markReady()

	// All should return quickly
	allDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(allDone)
	}()

	select {
	case <-allDone:
		assert.Equal(t, int64(n), atomic.LoadInt64(&unblocked))
	case <-time.After(2 * time.Second):
		t.Fatalf("only %d/%d waiters unblocked", atomic.LoadInt64(&unblocked), n)
	}
}

func TestMarkReady_Idempotent(t *testing.T) {
	t.Parallel()
	tc := newTestClient()

	// Calling markReady multiple times must not panic
	require.NotPanics(t, func() {
		tc.markReady()
		tc.markReady()
		tc.markReady()
	})
}

func TestMarkReady_ConcurrentSafe(t *testing.T) {
	t.Parallel()
	tc := newTestClient()

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)

	for range n {
		go func() {
			defer wg.Done()
			tc.markReady()
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// all returned without panic — good
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent markReady calls deadlocked or panicked")
	}
}

func TestShutdown_UnblocksGetClient(t *testing.T) {
	t.Parallel()
	tc := newTestClient()

	done := make(chan struct{})
	go func() {
		_ = tc.GetClient() // blocked
		close(done)
	}()

	// Still blocked
	select {
	case <-done:
		t.Fatal("GetClient returned before Shutdown")
	case <-time.After(100 * time.Millisecond):
	}

	require.NoError(t, tc.Shutdown())

	select {
	case <-done:
		// unblocked by Shutdown — good
	case <-time.After(2 * time.Second):
		t.Fatal("GetClient was not unblocked by Shutdown")
	}
}

func TestGetClient_ShutdownNoDeadlock(t *testing.T) {
	t.Parallel()
	tc := newTestClient()

	// Launch many GetClient callers
	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = tc.GetClient()
		}()
	}

	// Let them pile up, then Shutdown
	time.Sleep(50 * time.Millisecond)

	shutdownDone := make(chan struct{})
	go func() {
		_ = tc.Shutdown()
		close(shutdownDone)
	}()

	allDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(allDone)
	}()

	select {
	case <-allDone:
		// all waiters freed — good
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: GetClient waiters not freed after Shutdown")
	}

	select {
	case <-shutdownDone:
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: Shutdown did not complete")
	}
}

func TestHasClient_DoesNotBlock(t *testing.T) {
	t.Parallel()
	tc := newTestClient()

	done := make(chan bool, 1)
	go func() {
		done <- tc.HasClient()
	}()

	select {
	case v := <-done:
		assert.False(t, v, "HasClient should be false before Initialize")
	case <-time.After(2 * time.Second):
		t.Fatal("HasClient blocked on a non-ready client")
	}
}

func TestGetWorker_DoesNotBlock(t *testing.T) {
	t.Parallel()
	tc := newTestClient()

	done := make(chan struct{})
	go func() {
		w := tc.GetWorker()
		assert.Nil(t, w, "GetWorker should return nil before Initialize")
		close(done)
	}()

	select {
	case <-done:
		// returned immediately — good
	case <-time.After(2 * time.Second):
		t.Fatal("GetWorker blocked on a non-ready client")
	}
}
