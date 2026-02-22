package config

import "testing"

func TestLoadFromEnvUsesDefaultSocketPath(t *testing.T) {
	t.Setenv(EnvSocketPath, "")
	t.Setenv(EnvSudoersDir, "")
	t.Setenv(EnvVisudoBin, "")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv failed: %v", err)
	}

	if cfg.SocketPath != DefaultSocketPath {
		t.Fatalf("unexpected socket path: got %q want %q", cfg.SocketPath, DefaultSocketPath)
	}
	if cfg.SudoersDir != DefaultSudoersDir {
		t.Fatalf("unexpected sudoers dir: got %q want %q", cfg.SudoersDir, DefaultSudoersDir)
	}
	if cfg.VisudoBin != DefaultVisudoBin {
		t.Fatalf("unexpected visudo bin: got %q want %q", cfg.VisudoBin, DefaultVisudoBin)
	}
}

func TestLoadFromEnvUsesConfiguredSocketPath(t *testing.T) {
	wantSocket := "/tmp/custom-elevate.sock"
	wantSudoers := "/tmp/custom-sudoers.d"
	wantVisudo := "/usr/local/bin/visudo"
	t.Setenv(EnvSocketPath, wantSocket)
	t.Setenv(EnvSudoersDir, wantSudoers)
	t.Setenv(EnvVisudoBin, wantVisudo)

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv failed: %v", err)
	}

	if cfg.SocketPath != wantSocket {
		t.Fatalf("unexpected socket path: got %q want %q", cfg.SocketPath, wantSocket)
	}
	if cfg.SudoersDir != wantSudoers {
		t.Fatalf("unexpected sudoers dir: got %q want %q", cfg.SudoersDir, wantSudoers)
	}
	if cfg.VisudoBin != wantVisudo {
		t.Fatalf("unexpected visudo bin: got %q want %q", cfg.VisudoBin, wantVisudo)
	}
}

func TestValidateRejectsEmptySocketPath(t *testing.T) {
	cfg := &Config{SocketPath: "   ", SudoersDir: "/etc/sudoers.d", VisudoBin: "visudo"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for empty socket path")
	}
}

func TestValidateRejectsEmptySudoersDir(t *testing.T) {
	cfg := &Config{SocketPath: "/var/run/thand/elevate.sock", SudoersDir: "   ", VisudoBin: "visudo"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for empty sudoers dir")
	}
}

func TestValidateRejectsEmptyVisudoBin(t *testing.T) {
	cfg := &Config{SocketPath: "/var/run/thand/elevate.sock", SudoersDir: "/etc/sudoers.d", VisudoBin: "   "}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for empty visudo binary")
	}
}
