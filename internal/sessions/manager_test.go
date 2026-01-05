package sessions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/internal/models"
)

func TestLoginServer_GetSessions(t *testing.T) {
	sessions := map[string]models.LocalSession{
		"provider1": {Version: 1, Expiry: time.Now().Add(1 * time.Hour)},
		"provider2": {Version: 1, Expiry: time.Now().Add(2 * time.Hour)},
	}

	ls := LoginServer{
		Version:   "1.0",
		Timestamp: time.Now(),
		Sessions:  sessions,
	}

	result := ls.GetSessions()

	if len(result) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(result))
	}

	if _, ok := result["provider1"]; !ok {
		t.Error("Expected provider1 to be in sessions")
	}

	if _, ok := result["provider2"]; !ok {
		t.Error("Expected provider2 to be in sessions")
	}
}

func TestLoginServer_GetFirstActiveSession(t *testing.T) {
	tests := []struct {
		name           string
		sessions       map[string]models.LocalSession
		providers      []string
		expectError    bool
		expectProvider string
	}{
		{
			name:        "empty sessions returns error",
			sessions:    map[string]models.LocalSession{},
			providers:   nil,
			expectError: true,
		},
		{
			name: "finds active session",
			sessions: map[string]models.LocalSession{
				"provider1": {Version: 1, Expiry: time.Now().Add(1 * time.Hour)},
			},
			providers:      nil,
			expectError:    false,
			expectProvider: "provider1",
		},
		{
			name: "expired session returns error",
			sessions: map[string]models.LocalSession{
				"provider1": {Version: 1, Expiry: time.Now().Add(-1 * time.Hour)},
			},
			providers:   nil,
			expectError: true,
		},
		{
			name: "filters by provider list",
			sessions: map[string]models.LocalSession{
				"provider1": {Version: 1, Expiry: time.Now().Add(1 * time.Hour)},
				"provider2": {Version: 1, Expiry: time.Now().Add(1 * time.Hour)},
			},
			providers:      []string{"provider2"},
			expectError:    false,
			expectProvider: "provider2",
		},
		{
			name: "provider not in filter list returns error",
			sessions: map[string]models.LocalSession{
				"provider1": {Version: 1, Expiry: time.Now().Add(1 * time.Hour)},
			},
			providers:   []string{"provider2"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls := LoginServer{
				Version:   "1.0",
				Timestamp: time.Now(),
				Sessions:  tt.sessions,
			}

			providerName, session, err := ls.GetFirstActiveSession(tt.providers...)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if session == nil {
				t.Error("Expected session, got nil")
				return
			}

			if len(tt.expectProvider) != 0 && providerName != tt.expectProvider {
				t.Errorf("Expected provider %s, got %s", tt.expectProvider, providerName)
			}
		})
	}
}

func TestSessionManager_AddAndGetSession(t *testing.T) {
	setupTempSessionDir(t)

	manager := &SessionManager{
		Servers: make(map[string]LoginServer),
	}

	loginServer := "test.example.com"
	provider := "test-provider"
	session := models.LocalSession{
		Version: 1,
		Expiry:  time.Now().Add(1 * time.Hour),
		Session: "test-session-token",
	}

	err := manager.AddSession(loginServer, provider, session)
	if err != nil {
		t.Fatalf("Failed to add session: %v", err)
	}

	retrievedSession, err := manager.GetSession(loginServer, provider)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	if retrievedSession.Version != session.Version {
		t.Errorf("Expected version %d, got %d", session.Version, retrievedSession.Version)
	}

	if retrievedSession.Session != session.Session {
		t.Errorf("Expected session token %s, got %s", session.Session, retrievedSession.Session)
	}
}

func TestSessionManager_GetSession_NotFound(t *testing.T) {
	manager := &SessionManager{
		Servers: make(map[string]LoginServer),
	}

	_, err := manager.GetSession("test.example.com", "nonexistent-provider")
	if err == nil {
		t.Error("Expected error for non-existent session, got nil")
	}
}

func TestSessionManager_RemoveSession(t *testing.T) {
	setupTempSessionDir(t)

	manager := &SessionManager{
		Servers: make(map[string]LoginServer),
	}

	loginServer := "test.example.com"
	provider := "test-provider"
	session := models.LocalSession{
		Version: 1,
		Expiry:  time.Now().Add(1 * time.Hour),
		Session: "test-session-token",
	}

	err := manager.AddSession(loginServer, provider, session)
	if err != nil {
		t.Fatalf("Failed to add session: %v", err)
	}

	_, err = manager.GetSession(loginServer, provider)
	if err != nil {
		t.Fatalf("Session should exist before removal: %v", err)
	}

	err = manager.RemoveSession(loginServer, provider)
	if err != nil {
		t.Fatalf("Failed to remove session: %v", err)
	}

	_, err = manager.GetSession(loginServer, provider)
	if err == nil {
		t.Error("Session should not exist after removal")
	}
}

func TestSessionManager_GetFirstActiveSession(t *testing.T) {
	setupTempSessionDir(t)

	manager := &SessionManager{
		Servers: make(map[string]LoginServer),
	}

	loginServer := "test.example.com"
	activeSession := models.LocalSession{
		Version: 1,
		Expiry:  time.Now().Add(1 * time.Hour),
		Session: "active-session-token",
	}

	err := manager.AddSession(loginServer, "active-provider", activeSession)
	if err != nil {
		t.Fatalf("Failed to add session: %v", err)
	}

	providerName, session, err := manager.GetFirstActiveSession(loginServer)
	if err != nil {
		t.Fatalf("Failed to get first active session: %v", err)
	}

	if providerName != "active-provider" {
		t.Errorf("Expected provider 'active-provider', got '%s'", providerName)
	}

	if session == nil {
		t.Error("Expected session, got nil")
	}
}

func TestSessionManager_GetLoginServer_NormalizesHostname(t *testing.T) {
	tests := []struct {
		name             string
		loginServer      string
		expectedHostname string
	}{
		{
			name:             "URL with scheme is normalized to hostname",
			loginServer:      "https://test.example.com",
			expectedHostname: "test.example.com",
		},
		{
			name:             "plain hostname remains unchanged",
			loginServer:      "test.example.com",
			expectedHostname: "test.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := setupTempSessionDir(t)

			manager := &SessionManager{
				Servers: make(map[string]LoginServer),
			}

			server, err := manager.GetLoginServer(tt.loginServer)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if server == nil {
				t.Fatal("Expected server, got nil")
			}

			// Add a session to trigger file write
			session := models.LocalSession{
				Version: 1,
				Expiry:  time.Now().Add(1 * time.Hour),
				Session: "test-token",
			}
			err = manager.AddSession(tt.loginServer, "test-provider", session)
			if err != nil {
				t.Fatalf("Failed to add session: %v", err)
			}

			// Verify file was created with normalized hostname
			expectedFileName := tt.expectedHostname + ".yaml"
			filePath := filepath.Join(tmpDir, expectedFileName)
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				t.Errorf("Expected session file %s to exist on disk", filePath)
			}
		})
	}
}

func TestSessionManager_LoadAndCommit(t *testing.T) {
	tmpDir := setupTempSessionDir(t)

	loginServer := "test.example.com"
	provider := "test-provider"
	session := models.LocalSession{
		Version: 1,
		Expiry:  time.Now().Add(1 * time.Hour),
		Session: "test-session-token",
	}

	// Create first manager and add session
	manager1 := &SessionManager{
		Servers: make(map[string]LoginServer),
	}

	err := manager1.AddSession(loginServer, provider, session)
	if err != nil {
		t.Fatalf("Failed to add session: %v", err)
	}

	// Verify file exists
	filePath := filepath.Join(tmpDir, loginServer+".yaml")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("Session file should exist at %s", filePath)
	}

	// Create second manager and load the session from disk
	manager2 := &SessionManager{
		Servers: make(map[string]LoginServer),
	}

	err = manager2.Load(loginServer)
	if err != nil {
		t.Fatalf("Failed to load session: %v", err)
	}

	// Verify session was loaded
	retrievedSession, err := manager2.GetSession(loginServer, provider)
	if err != nil {
		t.Fatalf("Failed to get session after load: %v", err)
	}

	if retrievedSession.Session != session.Session {
		t.Errorf("Expected session token %s, got %s", session.Session, retrievedSession.Session)
	}
}

func TestSessionManager_Load_NonExistentFile(t *testing.T) {
	setupTempSessionDir(t)

	manager := &SessionManager{
		Servers: make(map[string]LoginServer),
	}

	// Loading a non-existent server should not error (creates empty server)
	err := manager.Load("nonexistent.example.com")
	if err != nil {
		t.Fatalf("Load should not error for non-existent file: %v", err)
	}
}

func TestSessionManager_Load_NormalizesHostname(t *testing.T) {
	tmpDir := setupTempSessionDir(t)

	loginServer := "test.example.com"
	loginServerWithScheme := "https://test.example.com"

	// Create a session file with the normalized hostname
	manager1 := &SessionManager{
		Servers: make(map[string]LoginServer),
	}

	session := models.LocalSession{
		Version: 1,
		Expiry:  time.Now().Add(1 * time.Hour),
		Session: "test-token",
	}

	err := manager1.AddSession(loginServer, "provider", session)
	if err != nil {
		t.Fatalf("Failed to add session: %v", err)
	}

	// Verify file was created with normalized name
	filePath := filepath.Join(tmpDir, loginServer+".yaml")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("Session file should exist at %s", filePath)
	}

	// Create second manager and load using URL with scheme
	manager2 := &SessionManager{
		Servers: make(map[string]LoginServer),
	}

	err = manager2.Load(loginServerWithScheme)
	if err != nil {
		t.Fatalf("Failed to load session with URL scheme: %v", err)
	}

	// Should be able to get the session using the URL with scheme
	retrievedSession, err := manager2.GetSession(loginServerWithScheme, "provider")
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	if retrievedSession.Session != session.Session {
		t.Errorf("Expected session token %s, got %s", session.Session, retrievedSession.Session)
	}
}

func TestSessionManager_Commit(t *testing.T) {
	tmpDir := setupTempSessionDir(t)

	loginServer := "test.example.com"

	manager := &SessionManager{
		Servers: make(map[string]LoginServer),
	}

	// Create server manually without adding session
	manager.createLoginServer(loginServer)

	// Commit should create the file
	err := manager.Commit(loginServer)
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Verify file exists
	filePath := filepath.Join(tmpDir, loginServer+".yaml")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Session file should exist at %s after commit", filePath)
	}
}

func TestSessionManager_GetLastTimestamp(t *testing.T) {
	setupTempSessionDir(t)

	manager := &SessionManager{
		Servers: make(map[string]LoginServer),
	}

	loginServer := "test.example.com"
	provider := "test-provider"
	expiry := time.Now().Add(1 * time.Hour)
	session := models.LocalSession{
		Version: 1,
		Expiry:  expiry,
		Session: "test-session-token",
	}

	err := manager.AddSession(loginServer, provider, session)
	if err != nil {
		t.Fatalf("Failed to add session: %v", err)
	}

	timestamp := manager.GetLastTimestamp(loginServer, provider)
	if timestamp == nil {
		t.Fatal("Expected timestamp, got nil")
	}

	if timestamp.Unix() != expiry.Unix() {
		t.Errorf("Expected timestamp %v, got %v", expiry, *timestamp)
	}
}

func TestSessionManager_GetLastTimestamp_NoProvider(t *testing.T) {
	setupTempSessionDir(t)

	manager := &SessionManager{
		Servers: make(map[string]LoginServer),
	}

	loginServer := "test.example.com"
	session := models.LocalSession{
		Version: 1,
		Expiry:  time.Now().Add(1 * time.Hour),
		Session: "test-session-token",
	}

	err := manager.AddSession(loginServer, "provider1", session)
	if err != nil {
		t.Fatalf("Failed to add session: %v", err)
	}

	timestamp := manager.GetLastTimestamp(loginServer, "")
	if timestamp == nil {
		t.Fatal("Expected timestamp, got nil")
	}
}

func TestSessionManager_CreateLoginServer(t *testing.T) {
	manager := &SessionManager{
		Servers: make(map[string]LoginServer),
	}

	loginServer := "new-server.example.com"

	if _, ok := manager.Servers[loginServer]; ok {
		t.Error("Server should not exist initially")
	}

	manager.createLoginServer(loginServer)

	server, ok := manager.Servers[loginServer]
	if !ok {
		t.Fatal("Server should exist after creation")
	}

	if server.Version != "1.0" {
		t.Errorf("Expected version '1.0', got '%s'", server.Version)
	}

	if server.Sessions == nil {
		t.Error("Sessions map should be initialized")
	}

	// Calling createLoginServer again should not overwrite existing server
	existingTimestamp := server.Timestamp
	manager.createLoginServer(loginServer)

	if manager.Servers[loginServer].Timestamp != existingTimestamp {
		t.Error("Existing server should not be overwritten")
	}
}

func TestSessionManager_AwaitRefresh_ContextCancellation(t *testing.T) {
	setupTempSessionDir(t)

	manager := &SessionManager{
		Servers: make(map[string]LoginServer),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result := manager.AwaitRefresh(ctx, "test.example.com")

	if result != nil {
		t.Error("Expected nil result when context is cancelled")
	}
}

func TestSessionManager_AwaitProviderRefresh_ContextCancellation(t *testing.T) {
	setupTempSessionDir(t)

	manager := &SessionManager{
		Servers: make(map[string]LoginServer),
	}

	loginServer := "test.example.com"
	provider := "test-provider"

	session := models.LocalSession{
		Version: 1,
		Expiry:  time.Now().Add(1 * time.Hour),
		Session: "test-session-token",
	}
	err := manager.AddSession(loginServer, provider, session)
	if err != nil {
		t.Fatalf("Failed to add session: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result := manager.AwaitProviderRefresh(ctx, loginServer, provider)

	if result != nil {
		t.Error("Expected nil result when context is cancelled")
	}
}

func TestGetSessionManager(t *testing.T) {
	manager := GetSessionManager()

	if manager == nil {
		t.Fatal("Expected session manager, got nil")
	}

	// Calling again should return the same instance
	manager2 := GetSessionManager()

	if manager != manager2 {
		t.Error("GetSessionManager should return the same instance")
	}
}

func TestSessionManager_YAMLFormat(t *testing.T) {
	tmpDir := setupTempSessionDir(t)

	loginServer := "test.example.com"

	manager := &SessionManager{
		Servers: make(map[string]LoginServer),
	}

	// Create a session with endpoint and authentication
	sessionWithAuth := models.LocalSession{
		Version: 1,
		Expiry:  time.Date(2026, 1, 4, 10, 33, 8, 0, time.UTC),
		Session: "token-with-auth-endpoint",
		Endpoint: &model.Endpoint{
			EndpointConfig: &model.EndpointConfiguration{
				URI: &model.LiteralUri{Value: "https://agent.example.com"},
				Authentication: &model.ReferenceableAuthenticationPolicy{
					AuthenticationPolicy: &model.AuthenticationPolicy{
						Basic: &model.BasicAuthenticationPolicy{
							Username: "user",
							Password: "password123",
						},
					},
				},
			},
		},
	}

	err := manager.AddSession(loginServer, "test-provider", sessionWithAuth)
	if err != nil {
		t.Fatalf("Failed to add session: %v", err)
	}

	err = manager.Commit(loginServer)
	if err != nil {
		t.Fatalf("Failed to commit sessions: %v", err)
	}

	// Read the YAML file
	filePath := filepath.Join(tmpDir, loginServer+".yaml")
	yamlContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read session file: %v", err)
	}

	actualYAML := string(yamlContent)

	// Expected clean YAML output (without null values)
	expectedYAML := `version: "1.0"
timestamp: <TIMESTAMP>
sessions:
  test-provider:
    version: 1
    expiry: 2026-01-04T10:33:08Z
    session: token-with-auth-endpoint
    endpoint:
      uri: https://agent.example.com
      authentication:
        basic:
          username: user
          password: password123
`

	// Compare (ignoring timestamp line which varies)
	if !yamlMatches(actualYAML, expectedYAML) {
		t.Errorf("YAML output does not match expected format.\n\nExpected:\n%s\n\nGot:\n%s", expectedYAML, actualYAML)
	}
}

// yamlMatches compares YAML strings, ignoring the timestamp line
func yamlMatches(actual, expected string) bool {
	// Simple approach: check if actual contains all non-timestamp lines from expected
	expectedLines := []string{
		`version: "1.0"`,
		"sessions:",
		"  test-provider:",
		"    version: 1",
		"    expiry: 2026-01-04T10:33:08Z",
		"    session: token-with-auth-endpoint",
		"    endpoint:",
		"      uri: https://agent.example.com",
		"      authentication:",
		"        basic:",
		"          username: user",
		"          password: password123",
	}

	for _, line := range expectedLines {
		if !strings.Contains(actual, line) {
			return false
		}
	}

	// Check that null doesn't appear
	if strings.Contains(actual, "null") {
		return false
	}

	return true
}

func TestNormalizeHostname(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://test.example.com", "test.example.com"},
		{"http://test.example.com", "test.example.com"},
		{"https://api.test.example.com:8080", "api.test.example.com"},
		{"test.example.com", "test.example.com"},
		{"localhost", "localhost"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeHostname(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeHostname(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// Helper function to setup a temporary session directory for testing
func setupTempSessionDir(t *testing.T) string {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "thand-sessions-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	originalPath := SESSION_MANAGER_PATH
	SESSION_MANAGER_PATH = tmpDir

	t.Cleanup(func() {
		SESSION_MANAGER_PATH = originalPath
		os.RemoveAll(tmpDir)
	})

	return tmpDir
}
