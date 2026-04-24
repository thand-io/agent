package temporaltest

import (
	"sync"

	"go.temporal.io/sdk/worker"
)

var binaryChecksumOnce sync.Once

// SeedBinaryChecksum seeds the Temporal SDK's process-global checksum cache once
// per test binary so testsuite activity workers do not hash the full test
// binary the first time a workflow executes an activity. This is safe for
// t.Parallel because all callers install the same deterministic checksum value.
func SeedBinaryChecksum() {
	binaryChecksumOnce.Do(func() {
		worker.SetBinaryChecksum("test-build-id")
	})
}
