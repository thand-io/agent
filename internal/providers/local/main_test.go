package local

import (
	"encoding/json"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/thand-io/agent/internal/models"
)

func TestAuthorizeRoleUnixTimedCreatesAndRevokesGrant(t *testing.T) {
	tempDir := t.TempDir()
	provider := newTestLocalProvider(t, "linux", tempDir)

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:          models.LocalSudoModeTimed,
		GrantID:       "grant-1",
		DeviceID:      "device-alpha",
		LocalUsername: "tester",
	})
	duration := 30 * time.Minute
	req.Duration = &duration

	response, err := provider.AuthorizeRole(nil, req)
	if err != nil {
		t.Fatalf("AuthorizeRole returned error: %v", err)
	}

	sudoersPath, _ := response.Metadata["sudoers_path"].(string)
	if len(sudoersPath) == 0 {
		t.Fatal("expected sudoers_path metadata to be set")
	}
	leasePath, _ := response.Metadata["lease_path"].(string)
	if len(leasePath) == 0 {
		t.Fatal("expected lease_path metadata to be set")
	}

	content, err := os.ReadFile(sudoersPath)
	if err != nil {
		t.Fatalf("failed to read sudoers grant: %v", err)
	}
	if !strings.Contains(string(content), "tester ALL=(ALL:ALL) NOPASSWD: ALL") {
		t.Fatalf("unexpected sudoers content: %s", string(content))
	}

	leaseData, err := os.ReadFile(leasePath)
	if err != nil {
		t.Fatalf("failed to read lease metadata: %v", err)
	}
	var lease leaseRecord
	if err := json.Unmarshal(leaseData, &lease); err != nil {
		t.Fatalf("failed to decode lease metadata: %v", err)
	}
	if lease.GrantID != "grant-1" || lease.DeviceID != "device-alpha" {
		t.Fatalf("unexpected lease metadata: %#v", lease)
	}

	if _, err := provider.RevokeRole(nil, &models.RevokeRoleRequest{
		AuthorizeRoleResponse: response,
	}); err != nil {
		t.Fatalf("RevokeRole returned error: %v", err)
	}

	if _, err := os.Stat(sudoersPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected sudoers file to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(leasePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected lease file to be removed, stat err=%v", err)
	}
}

func TestAuthorizeRoleUnixCommandRunsThroughSudoAndCleansUp(t *testing.T) {
	tempDir := t.TempDir()
	provider := newTestLocalProvider(t, "linux", tempDir)

	var calls []string
	provider.runCommand = func(name string, args ...string) (commandResult, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		switch filepath.Base(name) {
		case "visudo":
			return commandResult{}, nil
		case "sudo":
			return commandResult{Stdout: "root\n"}, nil
		default:
			return commandResult{}, nil
		}
	}

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:          models.LocalSudoModeCommand,
		Command:       []string{"whoami"},
		GrantID:       "grant-2",
		DeviceID:      "device-alpha",
		LocalUsername: "tester",
	})

	response, err := provider.AuthorizeRole(nil, req)
	if err != nil {
		t.Fatalf("AuthorizeRole returned error: %v", err)
	}

	if response.Metadata["stdout"] != "root\n" {
		t.Fatalf("stdout = %#v, want %q", response.Metadata["stdout"], "root\n")
	}
	if _, exists := response.Metadata["sudoers_path"]; exists {
		t.Fatalf("sudoers_path metadata should not be returned for immediate command execution")
	}
	if _, exists := response.Metadata["lease_path"]; exists {
		t.Fatalf("lease_path metadata should not be returned for immediate command execution")
	}
	if len(calls) < 2 {
		t.Fatalf("expected visudo and sudo invocations, got %#v", calls)
	}
	if !strings.Contains(calls[len(calls)-1], "/usr/bin/whoami") {
		t.Fatalf("expected sudo invocation to use resolved command path, got %#v", calls[len(calls)-1])
	}
}

func TestAuthorizeRoleWindowsTimedIsUnsupported(t *testing.T) {
	provider := newTestLocalProvider(t, "windows", t.TempDir())
	duration := 10 * time.Minute

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:     models.LocalSudoModeTimed,
		GrantID:  "grant-3",
		DeviceID: "device-alpha",
	})
	req.Duration = &duration

	_, err := provider.AuthorizeRole(nil, req)
	if err == nil || !strings.Contains(err.Error(), "not supported on Windows") {
		t.Fatalf("expected unsupported timed Windows error, got %v", err)
	}
}

func TestAuthorizeRoleWindowsCommandRequiresWindowsSudo(t *testing.T) {
	provider := newTestLocalProvider(t, "windows", t.TempDir())
	provider.lookPath = func(string) (string, error) {
		return "", errors.New("not found")
	}

	_, err := provider.AuthorizeRole(nil, newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:     models.LocalSudoModeCommand,
		Command:  []string{"netstat", "-ab"},
		GrantID:  "grant-4",
		DeviceID: "device-alpha",
	}))
	if err == nil || !strings.Contains(err.Error(), "Windows Sudo is unavailable") {
		t.Fatalf("expected Windows Sudo availability error, got %v", err)
	}
}

func TestAuthorizeRoleUnixFailsWithoutResolvedLocalUsername(t *testing.T) {
	provider := newTestLocalProvider(t, "linux", t.TempDir())
	duration := 15 * time.Minute

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:     models.LocalSudoModeTimed,
		GrantID:  "grant-missing-user",
		DeviceID: "device-alpha",
	})
	req.Duration = &duration

	_, err := provider.AuthorizeRole(nil, req)
	if err == nil || !strings.Contains(err.Error(), "missing a resolved local username") {
		t.Fatalf("expected missing local username error, got %v", err)
	}
}

func TestAuthorizeRoleUnixFailsForDeniedUsername(t *testing.T) {
	provider := newTestLocalProvider(t, "linux", t.TempDir())
	duration := 15 * time.Minute

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:             models.LocalSudoModeTimed,
		GrantID:          "grant-denied-user",
		DeviceID:         "device-alpha",
		LocalUsername:    "root",
		DeniedUsernames:  []string{"root"},
		AllowedUIDRanges: []string{"0-60000"},
	})
	req.Duration = &duration

	_, err := provider.AuthorizeRole(nil, req)
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("expected denied username error, got %v", err)
	}
}

func TestAuthorizeRoleUnixFailsForUIDOutsideAllowedRange(t *testing.T) {
	provider := newTestLocalProvider(t, "linux", t.TempDir())
	duration := 15 * time.Minute
	provider.lookupUser = func(username string) (*user.User, error) {
		return &user.User{Username: username, Uid: "999"}, nil
	}

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:             models.LocalSudoModeTimed,
		GrantID:          "grant-uid-outside-range",
		DeviceID:         "device-alpha",
		LocalUsername:    "fallbackuser",
		AllowedUIDRanges: []string{"1000-60000"},
	})
	req.Duration = &duration

	_, err := provider.AuthorizeRole(nil, req)
	if err == nil || !strings.Contains(err.Error(), "outside allowed UID ranges") {
		t.Fatalf("expected UID allow-range error, got %v", err)
	}
}

func TestAllowedUIDRangesFromLoginDefs(t *testing.T) {
	provider := newTestLocalProvider(t, "linux", t.TempDir())
	provider.readFile = func(string) ([]byte, error) {
		return []byte(`
# Comment
UID_MIN 1000
UID_MAX 60000
`), nil
	}

	ranges, err := provider.allowedUIDRanges(models.LocalSudoRequestMetadata{})
	if err != nil {
		t.Fatalf("allowedUIDRanges returned error: %v", err)
	}

	expected := []uidRange{{Min: 1000, Max: 60000}}
	if !reflect.DeepEqual(ranges, expected) {
		t.Fatalf("ranges = %#v, want %#v", ranges, expected)
	}
}

func TestAllowedUIDRangesConfigOverridesLoginDefs(t *testing.T) {
	provider := newTestLocalProvider(t, "linux", t.TempDir())
	(*provider.GetConfig())["allowed_uid_ranges"] = []string{"2000-2999"}
	provider.readFile = func(string) ([]byte, error) {
		return []byte("UID_MIN 1000\nUID_MAX 60000\n"), nil
	}

	ranges, err := provider.allowedUIDRanges(models.LocalSudoRequestMetadata{})
	if err != nil {
		t.Fatalf("allowedUIDRanges returned error: %v", err)
	}

	expected := []uidRange{{Min: 2000, Max: 2999}}
	if !reflect.DeepEqual(ranges, expected) {
		t.Fatalf("ranges = %#v, want %#v", ranges, expected)
	}
}

func TestAuthorizeRoleUnixTimedOverlappingGrantsUseDifferentFragments(t *testing.T) {
	tempDir := t.TempDir()
	provider := newTestLocalProvider(t, "linux", tempDir)
	duration := 30 * time.Minute

	reqA := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:          models.LocalSudoModeTimed,
		GrantID:       "grant-a",
		DeviceID:      "device-alpha",
		LocalUsername: "tester",
	})
	reqA.Duration = &duration

	reqB := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:          models.LocalSudoModeTimed,
		GrantID:       "grant-b",
		DeviceID:      "device-alpha",
		LocalUsername: "tester",
	})
	reqB.Duration = &duration

	respA, err := provider.AuthorizeRole(nil, reqA)
	if err != nil {
		t.Fatalf("AuthorizeRole A returned error: %v", err)
	}
	respB, err := provider.AuthorizeRole(nil, reqB)
	if err != nil {
		t.Fatalf("AuthorizeRole B returned error: %v", err)
	}

	pathA, _ := respA.Metadata["sudoers_path"].(string)
	pathB, _ := respB.Metadata["sudoers_path"].(string)
	if pathA == "" || pathB == "" {
		t.Fatalf("expected both sudoers paths, got %q and %q", pathA, pathB)
	}
	if pathA == pathB {
		t.Fatalf("expected different sudoers paths, both were %q", pathA)
	}

	if _, err := provider.RevokeRole(nil, &models.RevokeRoleRequest{AuthorizeRoleResponse: respA}); err != nil {
		t.Fatalf("RevokeRole A returned error: %v", err)
	}
	if _, err := os.Stat(pathA); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected first sudoers file to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(pathB); err != nil {
		t.Fatalf("expected second sudoers file to remain, stat err=%v", err)
	}
}

func TestReconcileRemovesExpiredLease(t *testing.T) {
	tempDir := t.TempDir()
	provider := newTestLocalProvider(t, "linux", tempDir)
	provider.readFile = os.ReadFile

	sudoersPath := filepath.Join(tempDir, "thand-local-sudo-expired")
	if err := os.WriteFile(sudoersPath, []byte("tester ALL=(ALL:ALL) NOPASSWD: ALL\n"), 0440); err != nil {
		t.Fatalf("failed to seed sudoers fragment: %v", err)
	}

	leasePath := filepath.Join(tempDir, "lease-expired.json")
	record := leaseRecord{
		GrantID:     "expired",
		DeviceID:    "device-alpha",
		Username:    "tester",
		SudoersPath: sudoersPath,
		ExpiresAt:   time.Now().UTC().Add(-1 * time.Minute),
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("failed to marshal lease record: %v", err)
	}
	if err := os.WriteFile(leasePath, data, 0600); err != nil {
		t.Fatalf("failed to seed lease record: %v", err)
	}

	if err := provider.enforcer.Reconcile(); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if _, err := os.Stat(sudoersPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected expired sudoers file to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(leasePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected expired lease file to be removed, stat err=%v", err)
	}
}

func newTestLocalProvider(t *testing.T, goos, tempDir string) *localProvider {
	t.Helper()

	config := models.BasicConfig{
		"sudoers_dir": tempDir,
		"lease_dir":   tempDir,
		"visudo_path": "visudo",
		"sudo_path":   "sudo",
	}

	provider := &localProvider{}
	if err := provider.Initialize("local", models.ProviderConfig{
		Name:     "Local",
		Provider: "local",
		Enabled:  true,
		Config:   &config,
	}); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	provider.goos = func() string { return goos }
	provider.lookupUser = func(username string) (*user.User, error) {
		return &user.User{Username: username, Uid: "1000"}, nil
	}
	provider.readFile = func(string) ([]byte, error) {
		return nil, os.ErrNotExist
	}
	provider.lookPath = func(name string) (string, error) {
		switch name {
		case "visudo":
			return "/usr/sbin/visudo", nil
		case "sudo":
			return "/usr/bin/sudo", nil
		case "whoami":
			return "/usr/bin/whoami", nil
		case "netstat":
			return "C:\\Windows\\System32\\netstat.exe", nil
		default:
			return "", os.ErrNotExist
		}
	}
	provider.runCommand = func(name string, args ...string) (commandResult, error) {
		return commandResult{}, nil
	}

	return provider
}

func newAuthorizeRoleRequest(metadata models.LocalSudoRequestMetadata) *models.AuthorizeRoleRequest {
	return &models.AuthorizeRoleRequest{
		Identity: &models.Identity{
			User: &models.User{
				Email: "user@example.com",
			},
		},
		Role: &models.CompositeRole{
			Role: models.Role{
				Name:       "Local Sudo",
				Identifier: "local_sudo",
			},
		},
		Metadata: metadata.AsMap(),
	}
}
