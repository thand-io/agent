//go:build windows

package state

func syncStateDir(string) error {
	// Windows directory handles are not fsync'd the same way as Unix here.
	// Keep atomic temp-file rename behavior and skip directory sync.
	return nil
}
