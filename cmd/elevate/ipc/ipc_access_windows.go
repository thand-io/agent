//go:build windows

package ipc

import "fmt"

func (s *UnixServer) configureSocketDirAccess(dir string) error {
	return s.applyWindowsACL(dir)
}

func (s *UnixServer) configureSocketFileAccess(path string) error {
	return s.applyWindowsACL(path)
}

func (s *UnixServer) applyWindowsACL(path string) error {
	if s.socketUser == "" && s.socketGrp == "" {
		return nil
	}

	args := []string{
		path,
		"/inheritance:r",
		"/grant:r", "SYSTEM:(F)",
	}
	if s.socketUser != "" {
		args = append(args, "/grant", fmt.Sprintf("%s:(M)", s.socketUser))
	}
	if s.socketGrp != "" {
		args = append(args, "/grant", fmt.Sprintf("%s:(M)", s.socketGrp))
	}

	if err := s.runCommand("icacls", args...); err != nil {
		return fmt.Errorf("set windows socket ACL on %q: %w", path, err)
	}
	return nil
}
