package sessions

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"gopkg.in/yaml.v3"
)

var SESSION_MANAGER_PATH = "~/.config/thand/"
var RELOAD_TIME = 1 * time.Second

var sessionManager *SessionManager

type SessionManager struct {
	lock    sync.Mutex             // Ensure thread-safe access
	Servers map[string]LoginServer // hostname -> LoginServer
}

type LoginServer struct {
	// Map of session ID to Session object
	Version   string                         `json:"version" yaml:"version" default:"1.0"`
	Timestamp time.Time                      `json:"timestamp" yaml:"timestamp"`
	Sessions  map[string]models.LocalSession `json:"sessions" yaml:"sessions"`
}

func (l LoginServer) GetSessions() map[string]models.LocalSession {
	return l.Sessions
}

func (l LoginServer) GetFirstActiveSession() (*models.LocalSession, error) {

	if len(l.Sessions) == 0 {
		return nil, fmt.Errorf("error")
	}

	for _, sesh := range l.Sessions {
		// Return first non-expired session
		if sesh.Expiry.After(time.Now()) {
			return &sesh, nil
		}
	}

	return nil, fmt.Errorf("error")
}

func (m *SessionManager) AddSession(loginServer string, provider string, session models.LocalSession) error {

	logrus.WithFields(logrus.Fields{
		"loginServer":    loginServer,
		"provider":       provider,
		"sessionExpiry":  session.Expiry,
		"sessionVersion": session.Version,
	}).Debugln("Adding new provider session")

	m.createLoginServer(loginServer)

	m.Servers[loginServer].Sessions[provider] = session

	return m.Commit(loginServer)
}

func (m *SessionManager) RemoveSession(loginServer string, provider string) error {

	logrus.WithFields(logrus.Fields{
		"loginServer": loginServer,
		"provider":    provider,
	}).Debugln("Removing provider session")

	m.createLoginServer(loginServer)
	delete(m.Servers[loginServer].Sessions, provider)

	return m.Commit(loginServer)
}

func (m *SessionManager) GetSession(loginServer string, provider string) (*models.LocalSession, error) {

	logrus.WithFields(logrus.Fields{
		"loginServer": loginServer,
		"provider":    provider,
	}).Debugln("Getting provider session")

	m.createLoginServer(loginServer)
	session, ok := m.Servers[loginServer].Sessions[provider]

	if !ok {
		return nil, fmt.Errorf("session not found for provider: %s", provider)
	}

	return &session, nil
}

func (m *SessionManager) GetLoginServer(loginServer string) (*LoginServer, error) {

	// validate that the login server is a hostname
	if !common.IsValidLoginServer(loginServer) {
		return nil, fmt.Errorf("invalid login server hostname: %s", loginServer)
	}

	logrus.WithFields(logrus.Fields{
		"loginServer": loginServer,
	}).Debugln("Getting login server")

	m.createLoginServer(loginServer)
	server, ok := m.Servers[loginServer]
	if !ok {
		return nil, fmt.Errorf("no sessions found for login server: %s", loginServer)
	}
	return &server, nil
}

func (m *SessionManager) AwaitRefresh(loginServer string) *LoginServer {

	logrus.WithFields(logrus.Fields{
		"loginServer": loginServer,
	}).Debugln("Awaiting refresh for login server")

	// provide a context deadline to try and reload the sessions
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	lastLoginServer, _ := m.GetLoginServer(loginServer)

	for {

		// Wait for a while before checking again
		select {
		case <-ctx.Done():
			// Context deadline exceeded or cancelled
			return nil
		case <-time.After(RELOAD_TIME):
			// Continue with the refresh check after 1 second
			time.Sleep(RELOAD_TIME)
		}
		// Reload the sessions
		err := m.Load(loginServer)

		if err != nil {
			logrus.WithError(err).Error("Failed to load sessions")
		}

		currentLoginServer, _ := m.GetLoginServer(loginServer)

		if lastLoginServer == nil && currentLoginServer != nil {
			return currentLoginServer
		} else if lastLoginServer != nil &&
			currentLoginServer != nil &&
			len(currentLoginServer.Sessions) > 0 &&
			lastLoginServer.Timestamp.UTC().Before(currentLoginServer.Timestamp.UTC()) {
			return currentLoginServer
		}

	}

}

func (m *SessionManager) AwaitProviderRefresh(loginServer string, provider string) error {

	// provide a context deadline to try and reload the sessions
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	lastTimestamp := m.GetLastTimestamp(loginServer, provider)

	// Wait for the user to re-authenticate and return the new session
	for {
		select {
		case <-ctx.Done():
			// Context deadline exceeded or cancelled
			return nil
		case <-time.After(RELOAD_TIME):
			// Continue with the refresh check after 1 second
			time.Sleep(RELOAD_TIME)
		}

		m.Load(loginServer)

		currentTimestamp := m.GetLastTimestamp(loginServer, provider)

		logrus.WithFields(logrus.Fields{
			"loginServer":      loginServer,
			"provider":         provider,
			"lastTimestamp":    lastTimestamp,
			"currentTimestamp": currentTimestamp,
		}).Debugln("Validating session")

		if lastTimestamp == nil && currentTimestamp != nil {
			return nil
		} else if lastTimestamp != nil &&
			currentTimestamp != nil &&
			lastTimestamp.UTC().Before(currentTimestamp.UTC()) {
			return nil
		}
	}
}

func (m *SessionManager) GetLastTimestamp(loginServer string, provider string) *time.Time {
	var lastTimestamp time.Time

	// We want to check the session file date
	if len(provider) > 0 {

		// Load the existing session to get the latest state
		lastProviderSession, err := m.GetSession(loginServer, provider)

		if err != nil {
			logrus.WithError(err).Error("Failed to get last session")
			return nil
		}

		lastTimestamp = lastProviderSession.Expiry

	} else {

		loginServer, err := m.GetLoginServer(loginServer)

		if err != nil {
			logrus.WithError(err).Error("Failed to get login server")
			return nil
		}

		lastTimestamp = loginServer.Timestamp
	}

	return &lastTimestamp
}

func (m *SessionManager) Commit(loginServer string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	file := loadSessionFile(loginServer)
	defer file.Close()

	// Truncate the file to ensure clean write
	err := file.Truncate(0)
	if err != nil {
		return err
	}

	// Seek to the beginning of the file
	_, err = file.Seek(0, 0)
	if err != nil {
		return err
	}

	encoder := yaml.NewEncoder(file)
	encoder.SetIndent(2)
	server := m.Servers[loginServer]
	server.Timestamp = time.Now().UTC()
	err = encoder.Encode(server)
	if err != nil {
		return err
	}
	defer encoder.Close()

	return nil
}

func (m *SessionManager) Load(loginServer string) error {

	logrus.Debugln("Checking sessions for provider:", loginServer)

	m.lock.Lock()
	defer m.lock.Unlock()

	file := loadSessionFile(loginServer)
	defer file.Close()

	// Check if file is empty
	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	if fileInfo.Size() == 0 {
		// File is empty, initialize with default LoginServer
		m.Servers[loginServer] = LoginServer{
			Version:   "1.0",
			Timestamp: time.Now().UTC(),
			Sessions:  make(map[string]models.LocalSession),
		}
		return nil
	}

	decoder := yaml.NewDecoder(file)
	var config LoginServer
	err = decoder.Decode(&config)
	if err != nil {
		// If YAML parsing fails, log the error and reinitialize
		logrus.WithError(err).Errorf("Failed to parse YAML for login server %s, reinitializing", loginServer)
		m.Servers[loginServer] = LoginServer{
			Version:   "1.0",
			Timestamp: time.Now().UTC(),
			Sessions:  make(map[string]models.LocalSession),
		}
		return nil
	}

	m.Servers[loginServer] = config

	return nil
}

func init() {
	sessionManager = &SessionManager{
		Servers: make(map[string]LoginServer),
	}
}

func GetSessionManager() *SessionManager {
	return sessionManager
}

func loadSessionFile(logonServerHostName string) *os.File {

	// Get the user's home directory
	usr, err := user.Current()
	if err != nil {
		log.Fatalf("Failed to get current user: %v", err)
	}

	// Expand the session manager path to use the actual home directory
	sessionPath := filepath.Join(usr.HomeDir, ".config", "thand")

	// Write session data to ~/.config/thand/sessions.yaml
	// first check if folder exists and if not then create it
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		err := os.MkdirAll(sessionPath, os.ModePerm)
		if err != nil {
			log.Fatalf("Failed to create session manager directory: %v", err)
		}
	}

	logonServer := fmt.Sprintf("%s.yaml", logonServerHostName)

	// Only allow read/write access to the owner
	file, err := os.OpenFile(
		filepath.Join(sessionPath, logonServer), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		log.Fatalf("Failed to open session manager file: %v", err)
	}
	return file
}

func (m *SessionManager) createLoginServer(loginServer string) {
	// check if logon server exists
	if _, ok := m.Servers[loginServer]; !ok {
		m.Servers[loginServer] = LoginServer{
			Version:   "1.0",
			Timestamp: time.Now().UTC(),
			Sessions:  make(map[string]models.LocalSession),
		}
	}
}
