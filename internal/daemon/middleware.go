package daemon

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	sessionManager "github.com/thand-io/agent/internal/sessions"
)

const (
	// Context keys
	SessionContextKey  = "session"
	ProviderContextKey = "provider"
)

// AuthMiddleware sets user context if authenticated, but doesn't require it
func (s *Server) AuthMiddleware() gin.HandlerFunc {
	encryptionServer := s.GetConfig().GetServices().GetEncryption()

	return func(c *gin.Context) {
		foundSessions := map[string]*models.Session{}
		cookie := sessions.Default(c)

		// Process different authentication sources
		s.processProviderCookies(cookie, encryptionServer, foundSessions)
		s.processBearerToken(c, encryptionServer, foundSessions)
		s.processAPIKey(c, encryptionServer, foundSessions)

		// Handle agent/client mode if no sessions found
		if len(foundSessions) == 0 && (s.Config.IsAgent() || s.Config.IsClient()) {
			s.handleAgentMode(c, cookie)
			return
		}

		// Set session context if sessions were found
		if len(foundSessions) > 0 {
			logrus.WithField("providers", foundSessions).Debugln("User sessions found in context")
			c.Set(SessionContextKey, foundSessions)
		}

		c.Next()
	}
}

// processProviderCookies extracts sessions from provider cookies
func (s *Server) processProviderCookies(
	cookie sessions.Session,
	encryptionServer models.EncryptionImpl,
	foundSessions map[string]*models.Session,
) {
	allProviders := s.Config.GetProvidersByCapability(models.ProviderCapabilityAuthorizor)

	for providerName := range allProviders {
		providerSessionData, ok := cookie.Get(providerName).(string)
		if !ok {
			continue
		}

		decodedSession, err := getDecodedSession(encryptionServer, providerSessionData)
		if err != nil {
			logrus.WithError(err).
				WithField("provider", providerName).
				Warnln("Failed to decode session from cookie")
			continue
		}

		foundSessions[providerName] = decodedSession.Session
	}
}

// processBearerToken extracts session from Authorization Bearer token
func (s *Server) processBearerToken(
	c *gin.Context,
	encryptionServer models.EncryptionImpl,
	foundSessions map[string]*models.Session,
) {
	authHeader := c.GetHeader("Authorization")
	if len(authHeader) == 0 || !strings.HasPrefix(authHeader, "Bearer ") {
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	decodedSession, err := getDecodedSession(encryptionServer, token)
	if err != nil {
		logrus.WithError(err).Warnln("Failed to decode bearer token from Authorization header")
		return
	}

	if len(decodedSession.Provider) == 0 {
		logrus.Warnln("Decoded session from bearer token has no provider information")
		return
	}

	foundSessions[decodedSession.Provider] = decodedSession.Session
}

// processAPIKey extracts session from X-API-Key header
func (s *Server) processAPIKey(
	c *gin.Context,
	encryptionServer models.EncryptionImpl,
	foundSessions map[string]*models.Session,
) {
	apiHeader := c.GetHeader("X-API-Key")
	if len(apiHeader) == 0 {
		return
	}

	decodedSession, err := getDecodedSession(encryptionServer, apiHeader)
	if err != nil {
		logrus.WithError(err).Warnln("Failed to decode API key from X-API-Key header")
		return
	}

	if len(decodedSession.Provider) == 0 {
		logrus.Warnln("Decoded session from API key has no provider information")
		return
	}

	foundSessions[decodedSession.Provider] = decodedSession.Session
}

// handleAgentMode processes sessions for agent/client mode
func (s *Server) handleAgentMode(c *gin.Context, cookie sessions.Session) {
	sm := sessionManager.GetSessionManager()
	loginServer, err := sm.GetLoginServer(s.Config.GetLoginServerHostname())
	if err != nil {
		logrus.WithError(err).Warnln("Failed to get login server for session")
		return
	}

	agentSessions := loginServer.GetSessions()
	for providerName, remoteSession := range agentSessions {
		cookie.Set(providerName, remoteSession.GetEncodedLocalSession())
	}

	err = cookie.Save()
	if err != nil {
		logrus.WithError(err).Warnln("Failed to save session cookie")
		return
	}

	// Redirect to reload the page with new cookies
	c.Redirect(http.StatusTemporaryRedirect, c.Request.RequestURI)
}

func getDecodedSession(encryptor models.EncryptionImpl, session string) (*models.ExportableSession, error) {

	localSession, err := models.DecodedLocalSession(session)

	if err != nil {
		return nil, fmt.Errorf("failed to decode local session: %w", err)
	}

	remoteSession, err := localSession.GetDecodedSession(encryptor)

	if err != nil {
		return nil, fmt.Errorf("failed to decode remote session from local session: %w", err)
	}

	return remoteSession, nil
}

func (s *Server) getUser(c *gin.Context, providerName ...string) (*models.Session, error) {

	if !s.Config.IsServer() {
		return nil, fmt.Errorf("getUser can only be called in server mode")
	}

	session, hasSession := c.Get(SessionContextKey)

	if !hasSession {
		return nil, fmt.Errorf("no user session found in context")
	}

	remoteSession, ok := session.(map[string]*models.Session)

	if !ok {
		return nil, fmt.Errorf("invalid session type found in context")
	}

	if len(providerName) > 0 {
		if session, ok := remoteSession[providerName[0]]; ok {
			return session, nil
		}
		return nil, fmt.Errorf("no user session found for provider: %s", providerName[0])
	}

	// Return the first session we find
	for _, remoteSession := range remoteSession {
		return remoteSession, nil
	}

	return nil, fmt.Errorf("no user session found")
}
