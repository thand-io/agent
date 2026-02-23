//go:build linux || darwin

package ipc

import "fmt"

func (s *UnixServer) configureSocketDirAccess(dir string) error {
	uid, gid, has, err := s.resolveSocketOwnership()
	if err != nil {
		return err
	}
	if !has {
		return nil
	}
	if err := s.chown(dir, uid, gid); err != nil {
		return fmt.Errorf("set unix socket directory ownership: %w", err)
	}
	return nil
}

func (s *UnixServer) configureSocketFileAccess(path string) error {
	if err := s.chmod(path, s.socketPerm); err != nil {
		return fmt.Errorf("set unix socket permissions: %w", err)
	}
	uid, gid, has, err := s.resolveSocketOwnership()
	if err != nil {
		return err
	}
	if has {
		if err := s.chown(path, uid, gid); err != nil {
			return fmt.Errorf("set unix socket ownership: %w", err)
		}
	}
	return nil
}
