//go:build !windows

package state

import "os"

func syncStateDir(dir string) error {
	dirFD, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer dirFD.Close()
	return dirFD.Sync()
}
