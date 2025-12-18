package daemon

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	sessionManager "github.com/thand-io/agent/internal/sessions"
)

const (
	// Context keys
	SessionContextKey  = "session"
	ProviderContextKey = "provider"
)

// CORSMiddleware creates a CORS middleware that supports wildcard patterns in origins
// Standard CORS only allows exact origins or "*", this middleware extends that to support
// patterns like "https://*.example.com"
func CORSMiddleware(cfg models.CORSConfig) gin.HandlerFunc {
	// Apply defaults for any unset values
	corsConfig := cfg.WithDefaults()
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		if len(origin) == 0 {
			// No origin header, skip CORS processing
			c.Next()
			return
		}

		logrus.WithFields(logrus.Fields{
			"origin":         origin,
			"method":         c.Request.Method,
			"path":           c.Request.URL.Path,
			"allowedOrigins": corsConfig.AllowedOrigins,
		}).Debugln("CORS middleware processing request")

		// Check if the origin matches any allowed pattern
		allowedOrigin := ""
		for _, pattern := range corsConfig.AllowedOrigins {
			if matchOrigin(origin, pattern) {
				allowedOrigin = origin
				logrus.WithFields(logrus.Fields{
					"origin":  origin,
					"pattern": pattern,
				}).Debugln("CORS origin matched pattern")
				break
			}
		}

		// If no match found, don't set CORS headers
		if len(allowedOrigin) == 0 {
			logrus.WithFields(logrus.Fields{
				"origin":         origin,
				"allowedOrigins": corsConfig.AllowedOrigins,
			}).Warnln("CORS origin not matched - no Access-Control-Allow-Origin header will be set")
			// For preflight requests with no matching origin, return 403
			if c.Request.Method == "OPTIONS" {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Next()
			return
		}

		// Set CORS headers
		c.Header("Access-Control-Allow-Origin", allowedOrigin)

		if corsConfig.AllowCredentials {
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		if len(corsConfig.ExposeHeaders) > 0 {
			c.Header("Access-Control-Expose-Headers", strings.Join(corsConfig.ExposeHeaders, ", "))
		}

		// Handle preflight request
		if c.Request.Method == "OPTIONS" {
			c.Header("Access-Control-Allow-Methods", strings.Join(corsConfig.AllowedMethods, ", "))
			c.Header("Access-Control-Allow-Headers", strings.Join(corsConfig.AllowedHeaders, ", "))

			if corsConfig.MaxAge > 0 {
				c.Header("Access-Control-Max-Age", fmt.Sprintf("%d", corsConfig.MaxAge))
			}

			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// matchOrigin checks if the given origin matches the pattern
// Supports exact matches and wildcard patterns like "https://*.example.com"
func matchOrigin(origin, pattern string) bool {
	if len(origin) == 0 {
		return false
	}

	// Exact match
	if origin == pattern {
		return true
	}

	// Allow all origins
	if pattern == "*" {
		return true
	}

	// Wildcard pattern matching (e.g., "https://*.example.com")
	if strings.Contains(pattern, "*") {
		return matchWildcardOrigin(origin, pattern)
	}

	return false
}

// matchWildcardOrigin matches an origin against a wildcard pattern
// Pattern format: scheme://*.domain.tld or scheme://*.subdomain.domain.tld
func matchWildcardOrigin(origin, pattern string) bool {
	// Split pattern by "*"
	parts := strings.SplitN(pattern, "*", 2)
	if len(parts) != 2 {
		return false
	}

	prefix := parts[0] // e.g., "https://"
	suffix := parts[1] // e.g., ".example.com"

	// Validate pattern format: suffix must start with a dot to prevent
	// patterns like "https://*example.com" from matching "https://evilexample.com"
	if !strings.HasPrefix(suffix, ".") {
		return false
	}

	// Origin must start with the prefix
	if !strings.HasPrefix(origin, prefix) {
		return false
	}

	// Origin must end with the suffix
	if !strings.HasSuffix(origin, suffix) {
		return false
	}

	// Extract the wildcard part (the subdomain)
	wildcardPart := origin[len(prefix) : len(origin)-len(suffix)]

	// The wildcard part should not be empty and should not contain additional dots
	// (to prevent matching nested subdomains unless explicitly allowed)
	// e.g., "https://*.example.com" should match "https://foo.example.com"
	// but not "https://foo.bar.example.com"
	if len(wildcardPart) == 0 {
		return false
	}

	// Allow nested subdomains - if you want to restrict to single level,
	// uncomment the following:
	// if strings.Contains(wildcardPart, ".") {
	// 	return false
	// }

	return true
}

// SetupMiddleware this automatically detects and updates the server hostname
// if its currently set to a default value or localhost
func (s *Server) SetupMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		if !s.Config.IsServer() {

			c.Next()
			return
		}

		// Ok so we're running in server mode, check if the hostname
		// has been configured

		notDefaultLoginEndpoint := s.Config.GetLoginServerUrl() != common.DefaultLoginServerEndpoint
		notDefaultSecret := s.Config.Secret != common.DefaultServerSecret
		hasEncryptionService := s.Config.GetServices().HasEncryption()

		// If any configuration is missing, show setup page
		// Make sure all these are true
		if notDefaultLoginEndpoint && notDefaultSecret && hasEncryptionService {

			// Server has been configured, continue

			c.Next()
			return
		}

		// The server hasn't been configured, so we need to return the
		// setup page to allow configuration

		logrus.Infoln("Server hostname or secret not configured, redirecting to setup page")

		s.setupPage(c)

		c.Abort()

	}
}

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

		// Lastly, add all the users from the found sessions to the identities pool
		// The identity, should already be in the pool. However, if the server has
		// restarted or this is an older session, we need to re-add them
		for providerId, session := range foundSessions {
			foundProvider, err := s.Config.GetProviderByName(providerId)

			if err != nil {
				logrus.WithError(err).
					WithField("provider", providerId).
					Warnln("Failed to get provider for identity addition")
				continue
			}

			providerClient := foundProvider.GetClient()

			if providerClient == nil {
				logrus.WithField("provider", providerId).
					Warnln("Provider client is nil for identity addition")
				continue
			}

			sessionUser := session.User

			if sessionUser == nil {
				logrus.WithField("provider", providerId).
					Warnln("Session user is nil for identity addition")
				continue
			}

			providerClient.AddIdentities(models.Identity{
				ID:    sessionUser.ID,
				Label: sessionUser.Name,
				User:  sessionUser,
				Providers: map[string]string{
					foundProvider.Name: providerId,
				},
			})
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
	// Get providers to check for cookies
	providersToCheck := make(map[string]bool)

	// Always check providers configured locally
	for providerName := range s.Config.GetProvidersByCapability(models.ProviderCapabilityAuthorizer) {
		providersToCheck[providerName] = true
	}

	// In agent/client mode, also check providers from the session manager
	// This ensures we find cookies that were set from login server sessions
	if s.Config.IsAgent() || s.Config.IsClient() {
		sm := sessionManager.GetSessionManager()
		loginServer, err := sm.GetLoginServer(s.Config.GetLoginServerHostname())
		if err == nil {
			for providerName := range loginServer.GetSessions() {
				providersToCheck[providerName] = true
			}
		}
	}

	for providerName := range providersToCheck {

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
		c.Next()
		return
	}

	agentSessions := loginServer.GetSessions()

	// If no sessions available, just continue without redirect
	if len(agentSessions) == 0 {
		logrus.Debugln("No agent sessions available, continuing without redirect")
		c.Next()
		return
	}

	// Track if we actually set any new cookies
	cookiesSet := false

	for providerName, remoteSession := range agentSessions {

		cookie := sessions.DefaultMany(c, CreateCookieName(providerName))

		if cookie == nil {
			continue
		}

		// Check if the cookie already has the same session data to avoid redirect loops
		existingSession, ok := cookie.Get(ThandCookieAttributeSessionName).(string)
		newSession := remoteSession.GetEncodedLocalSession()

		if ok && existingSession == newSession {
			// Session already set, no need to redirect
			continue
		}

		cookie.Set(ThandCookieAttributeSessionName, newSession)

		err = cookie.Save()

		if err != nil {
			logrus.WithError(err).Warnln("Failed to save session cookie")
			c.Next()
			return
		}

		cookiesSet = true
	}

	// Only redirect if we actually set new cookies
	if cookiesSet {
		// Redirect to reload the page with new cookies
		c.Redirect(http.StatusTemporaryRedirect, c.Request.RequestURI)
		return
	}

	// No new cookies set, continue processing the request
	c.Next()
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

		validProviders := []string{}

		// Ok first, check to see what sessions we have for the requested providers
		for _, providerName := range authProviders {
			if _, ok := remoteSessions[providerName]; ok {
				validProviders = append(validProviders, providerName)
			}
		}

		if len(validProviders) == 0 {
			return "", nil, fmt.Errorf("no user session found for the requested providers: %s", strings.Join(authProviders, ", "))
		}

		// First see if we can find any of the requested providers
		// that haven't expired
		for _, providerName := range validProviders {
			if session, ok := remoteSessions[providerName]; ok {
				if !session.IsExpired() {
					return providerName, session, nil
				}
			}
		}

		// Otherwise return the first requested provider even if expired
		firstProvider := validProviders[0]
		return firstProvider, remoteSessions[firstProvider], nil
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
