package daemon

import (
	"fmt"
	"net/http"
	"slices"
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

		// Process different authentication sources
		s.processProviderCookies(c, encryptionServer, foundSessions)
		s.processBearerToken(c, encryptionServer, foundSessions)
		s.processAPIKey(c, encryptionServer, foundSessions)

		// Handle agent/client mode if no sessions found
		if len(foundSessions) == 0 && (s.Config.IsAgent() || s.Config.IsClient()) {
			s.handleAgentMode(c)
			return
		}

		// Set session context if sessions were found
		if len(foundSessions) > 0 {
			logrus.WithFields(logrus.Fields{
				"providers": len(foundSessions),
			}).Debugln("User sessions found in context")
			c.Set(SessionContextKey, foundSessions)
		}

		c.Next()
	}
}

// processProviderCookies extracts sessions from provider cookies
func (s *Server) processProviderCookies(
	c *gin.Context,
	encryptionServer models.EncryptionImpl,
	foundSessions map[string]*models.Session,
) {
	allProviders := s.Config.GetProvidersByCapability(models.ProviderCapabilityAuthorizor)

	for providerName := range allProviders {

		cookie := sessions.DefaultMany(c, CreateCookieName(providerName))

		if cookie == nil {
			continue
		}

		providerSessionData, ok := cookie.Get(ThandCookieAttributeSessionName).(string)

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
func (s *Server) handleAgentMode(c *gin.Context) {
	sm := sessionManager.GetSessionManager()
	loginServer, err := sm.GetLoginServer(s.Config.GetLoginServerHostname())
	if err != nil {
		logrus.WithError(err).Warnln("Failed to get login server for session")
		return
	}

	agentSessions := loginServer.GetSessions()
	for providerName, remoteSession := range agentSessions {

		cookie := sessions.DefaultMany(c, CreateCookieName(providerName))
		cookie.Set(ThandCookieAttributeSessionName, remoteSession.GetEncodedLocalSession())

		err = cookie.Save()

		if err != nil {
			logrus.WithError(err).Warnln("Failed to save session cookie")
			return
		}

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

func (s *Server) getUserFromElevationRequest(c *gin.Context, request models.ElevateRequest) (string, *models.Session, error) {

	// Get a list of providers we want to auth against
	findAuthProviders := []string{}

	// TODO: If specified should be enforced to be used
	if len(request.Authenticator) > 0 {
		findAuthProviders = append(findAuthProviders, request.Authenticator)
	}

	// Add role providers
	if request.Role != nil && len(request.Role.Authenticators) > 0 {

		findAuthProviders = append(findAuthProviders, request.Role.Authenticators...)

		// Check if request.Authenticator is in the list of elevation request's role authenticators
		if len(request.Authenticator) > 0 {
			if !slices.Contains(request.Role.Authenticators, request.Authenticator) {
				return "", nil, fmt.Errorf("authenticator %s is not allowed for the specified role", request.Authenticator)
			}
		}

	}

	return s.getUser(c, findAuthProviders...)

}

func (s *Server) getUser(c *gin.Context, authProviders ...string) (string, *models.Session, error) {

	remoteSessions, err := s.getUserSessions(c)

	if err != nil {
		return "", nil, err
	}

	if len(authProviders) > 0 {

		// Return the first session we find
		for _, providerName := range authProviders {
			if session, ok := remoteSessions[providerName]; ok {
				return providerName, session, nil
			}
		}

		return "", nil, fmt.Errorf("no user session found for the requested providers: %s", strings.Join(authProviders, ", "))

	}

	// Otherwise return the primary session if it exists
	primaryCookie := sessions.DefaultMany(c, ThandCookieName)

	if primaryCookie != nil {

		activeProvider, ok := primaryCookie.Get(ThandCookieAttributeActiveName).(string)

		if ok && len(activeProvider) > 0 {
			if session, exists := remoteSessions[activeProvider]; exists {
				return activeProvider, session, nil
			}
		}
	}

	// Otherwise return the session that is the most recently active

	var latestProvider string
	var latestSession *models.Session

	for providerName, session := range remoteSessions {
		if latestSession == nil || session.Expiry.After(latestSession.Expiry) {
			latestProvider = providerName
			latestSession = session
		}
	}

	if latestSession != nil {
		return latestProvider, latestSession, nil
	}

	return "", nil, fmt.Errorf("no user session found")
}

func (s *Server) getUserSessions(c *gin.Context) (map[string]*models.Session, error) {

	if !s.Config.IsServer() {
		return nil, fmt.Errorf("getUserSessions can only be called in server mode")
	}

	session, hasSession := c.Get(SessionContextKey)

	if !hasSession {
		return nil, fmt.Errorf("no user session found in context")
	}

	remoteSession, ok := session.(map[string]*models.Session)

	if !ok {
		return nil, fmt.Errorf("invalid session type found in context")
	}

	return remoteSession, nil
}
