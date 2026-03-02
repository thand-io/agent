package models_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/thand-io/agent/internal/models"
)

func newTestProvider(name string) *models.BaseProvider {
	return models.NewBaseProvider(
		name,
		models.ProviderConfig{Name: name},
		models.NewProviderCapabilities().
			WithDefaultRolesConfiguration(),
	)
}

func TestBaseProvider_InitialStateIsUninitialized(t *testing.T) {
	p := newTestProvider("test")
	assert.False(t, p.IsReady(), "new provider should not be ready")
}

func TestBaseProvider_SetPendingTransition(t *testing.T) {
	p := newTestProvider("test")

	p.SetPending()
	assert.False(t, p.IsReady(), "pending provider should not be ready")
}

func TestBaseProvider_SetReadyTransition(t *testing.T) {
	p := newTestProvider("test")

	p.SetReady()
	assert.True(t, p.IsReady(), "provider should be ready after SetReady")
}

func TestBaseProvider_SetReadyFromPending(t *testing.T) {
	p := newTestProvider("test")

	p.SetPending()
	assert.False(t, p.IsReady())

	p.SetReady()
	assert.True(t, p.IsReady())
}

func TestBaseProvider_SetReadyIsIdempotent(t *testing.T) {
	p := newTestProvider("test")

	// Should not panic on multiple calls
	p.SetReady()
	p.SetReady()
	p.SetReady()
	assert.True(t, p.IsReady())
}

func TestBaseProvider_ReadyChannelClosedOnSetReady(t *testing.T) {
	p := newTestProvider("test")

	// Channel should block before SetReady
	select {
	case <-p.Ready():
		t.Fatal("Ready channel should not be closed before SetReady")
	default:
		// expected
	}

	p.SetReady()

	// Channel should be closed now
	select {
	case <-p.Ready():
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Ready channel should be closed after SetReady")
	}
}

func TestBaseProvider_ReadyChannelSignalsWaiters(t *testing.T) {
	p := newTestProvider("test")

	var wg sync.WaitGroup
	const numWaiters = 5

	// Launch multiple goroutines waiting on Ready()
	for i := 0; i < numWaiters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-p.Ready():
				// success
			case <-time.After(2 * time.Second):
				t.Error("Timed out waiting for Ready()")
			}
		}()
	}

	// Give goroutines time to start waiting
	time.Sleep(50 * time.Millisecond)

	// Signal ready
	p.SetReady()

	// All waiters should complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("Not all waiters were unblocked")
	}
}

func TestBaseProvider_ReadyChannelClosedBeforeWait(t *testing.T) {
	p := newTestProvider("test")

	// Mark ready before anyone waits
	p.SetReady()

	// Waiting after ready should return immediately
	select {
	case <-p.Ready():
		// expected — channel was already closed
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Ready channel should already be closed")
	}
}

func TestBaseProvider_ConcurrentSetReady(t *testing.T) {
	p := newTestProvider("test")

	var wg sync.WaitGroup
	const numGoroutines = 50

	// Call SetReady concurrently — must not panic
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.SetReady()
		}()
	}

	wg.Wait()
	assert.True(t, p.IsReady())
}

func TestBaseProvider_SetPendingThenConcurrentSetReady(t *testing.T) {
	p := newTestProvider("test")
	p.SetPending()

	var wg sync.WaitGroup
	const numGoroutines = 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.SetReady()
		}()
	}

	wg.Wait()
	assert.True(t, p.IsReady())

	// Channel should be closed
	select {
	case <-p.Ready():
		// expected
	default:
		t.Fatal("Ready channel should be closed")
	}
}
