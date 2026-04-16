package daemon

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	sessionManager "github.com/thand-io/agent/internal/sessions"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

// postSession creates a new session
//
//	@Summary		Create a new session
//	@Description	Create a new session with the provided session token
//	@Tags			sessions
//	@Accept			json
//	@Produce		json
//	@Param			session	body		models.SessionCreateRequest	true	"Session creation request"
//	@Success		200		{object}	map[string]any		"Session created successfully"
//	@Failure		400		{object}	map[string]any		"Bad request"
//	@Failure		500		{object}	map[string]any		"Internal server error"
//	@Router			/sessions [post]
func (s *Server) postSession(c *gin.Context) {

	// This is an un-authenticated endpoint to create a session
	// but only allowed in agent mode. The code provided must match
	// the code we issued when starting the agent. This will prevent
	// unauthorised session creation.

	if !s.Config.IsAgent() && !s.Config.IsClient() {
		s.getErrorPage(c, http.StatusBadRequest, "Session creation can only be called in agent mode")
		return
	}

	// Get the post JSON Body as a Session create request
	// which is a struct with fields for session creation
	var sessionCreateRequest models.SessionCreateRequest
	if err := c.ShouldBindJSON(&sessionCreateRequest); err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to parse request body", err)
		return
	}

	// Validate the code we sent matches the expected code
	if len(sessionCreateRequest.Code) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Session code is required")
		return
	}

	// We need to decrypt the code to check we issued it.
	if !s.Config.GetServices().HasEncryption() {
		s.getErrorPage(c, http.StatusInternalServerError, "Encryption service is not configured")
		return
	}

	sessionCode := sessionCreateRequest.Code

	// If the code decrypts then we're all good.
	codeResponse, err := models.EncodingWrapper{
		Type: sdkConstants.ENCODED_SESSION_CODE,
	}.DecodeAndDecrypt(
		sessionCode,
		s.Config.GetServices().GetEncryption(),
	)

	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to decrypt session code", err)
		return
	}

	codeWrapper := models.CodeWrapper{}
	err = common.ConvertInterfaceToInterface(codeResponse.Data, &codeWrapper)

	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid session code data")
		return
	}

	// Validate the code is still valid
	if !codeWrapper.IsValid(s.Config.GetLoginServerUrl()) {
		s.getErrorPage(c, http.StatusBadRequest, "Session code is invalid or expired")
		return
	}

	sessionToken := sessionCreateRequest.Session

	// The session token is an encoded local session
	// The payload is encrypted - however, the decode
	// call does not require decryption as the data is
	// already encrypted within the session token.
	sessionData, err := models.EncodingWrapper{
		Type: sdkConstants.ENCODED_SESSION_LOCAL,
	}.Decode(sessionToken)

	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to decode session token", err)
		return
	}

	decodedSessionData, ok := sessionData.Data.(map[string]any)

	if !ok {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid session token data")
		return
	}

	var session models.LocalSession
	err = common.ConvertMapToInterface(decodedSessionData, &session)

	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to convert session data", err)
		return
	}
	loginServer := s.Config.GetLoginServerHostname()

	logrus.WithFields(logrus.Fields{
		"loginServer": loginServer,
		"provider":    sessionCreateRequest.Provider,
	}).Debugln("Creating session")

	// Now lets store the session in the users local session manager.
	sessionMgr := sessionManager.GetSessionManager()
	err = sessionMgr.AddSession(
		loginServer,
		sessionCreateRequest.Provider,
		session,
	)

	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to store session", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Session created successfully",
		"expiry":  session.Expiry.UTC(),
	})
}

// getSessions retrieves all sessions
//
//	@Summary		Get all sessions
//	@Description	Retrieve all active sessions for the user
//	@Tags			sessions
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	models.SessionsResponse		"List of sessions with default provider"
//	@Failure		400	{object}	map[string]any	"Bad request"
//	@Failure		500	{object}	map[string]any	"Internal server error"
//	@Router			/sessions [get]
func (s *Server) getSessions(c *gin.Context) {

	// Get the default provider from cookie
	defaultProvider := s.getDefaultProviderCookieValue(c)

	if s.Config.IsServer() {

		remoteSessions, err := s.getUserSessions(c)

		if err != nil {
			s.getErrorPage(c, http.StatusBadRequest, "Failed to get user sessions", err)
			return
		}

		foundSessions := map[string]models.LocalSession{}

		// Convert to response format
		for providerName, session := range remoteSessions {
			foundSessions[providerName] = models.LocalSession{
				Version: 1,
				Expiry:  session.Expiry,
			}
		}

		sessionsResponse := models.SessionsResponse{
			Version:         "1",
			Timestamp:       time.Now(),
			Sessions:        foundSessions,
			DefaultProvider: defaultProvider,
		}

		c.JSON(http.StatusOK, sessionsResponse)
		return

	} else if s.Config.IsAgent() {

		loginServer := s.Config.GetLoginServerHostname()

		logrus.WithFields(logrus.Fields{
			"loginServer": loginServer,
		}).Debugln("Fetching sessions")

		sessionMgr := sessionManager.GetSessionManager()
		sessionMgr.Load(loginServer)
		sessionsList, err := sessionMgr.GetLoginServer(loginServer)

		if err != nil {
			s.getErrorPage(c, http.StatusInternalServerError, "Failed to list sessions", err)
			return
		}

		sessionsResponse := models.SessionsResponse{
			Version:         sessionsList.Version,
			Timestamp:       sessionsList.Timestamp,
			Sessions:        sessionsList.Sessions,
			DefaultProvider: defaultProvider,
		}

		c.JSON(http.StatusOK, sessionsResponse)
		return

	} else {

		s.getErrorPage(c, http.StatusBadRequest, "Get sessions can only be called in agent or server mode")
		return
	}
}

// getWhoami retrieves the current authenticated user
//
//	@Summary		Get current user
//	@Description	Get information about the currently authenticated user
//	@Tags			sessions
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	models.WhoamiResponse	"Current user information"
//	@Failure		401	{object}	map[string]any	"Unauthorized"
//	@Router			/whoami [get]
//	@Security		BearerAuth
func (s *Server) getWhoami(c *gin.Context) {
	// Get authenticated session
	provider, session, err := s.getSession(c)
	if err != nil {
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	if session == nil || session.User == nil {
		s.getErrorPage(c, http.StatusUnauthorized, "No user session found")
		return
	}

	// Return user information
	response := models.WhoamiResponse{
		User:     session.User,
		Provider: provider,
		Session: models.SessionInfo{
			UUID:   session.UUID,
			Expiry: session.Expiry,
		},
	}

	c.JSON(http.StatusOK, response)
}

// getSessionByProvider retrieves a session for a specific provider
//
//	@Summary		Get session by provider
//	@Description	Retrieve session information for a specific provider
//	@Tags			sessions
//	@Accept			json
//	@Produce		json
//	@Param			provider	path		string					true	"Provider name"
//	@Success		200			{object}	map[string]any	"Session information"
//	@Failure		400			{object}	map[string]any	"Bad request"
//	@Failure		404			{object}	map[string]any	"Session not found"
//	@Failure		500			{object}	map[string]any	"Internal server error"
//	@Router			/session/{provider} [get]
func (s *Server) getSessionByProvider(c *gin.Context) {

	provider := c.Param("provider")
	if len(provider) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Provider is required")
		return
	}

	loginServer := s.Config.GetLoginServerHostname()

	logrus.WithFields(logrus.Fields{
		"loginServer": loginServer,
		"provider":    provider,
	}).Debugln("Fetching session for provider")

	sessionMgr := sessionManager.GetSessionManager()
	sessionMgr.Load(loginServer)
	session, err := sessionMgr.GetSession(loginServer, provider)

	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to get session", err)
		return
	}

	if session == nil {
		s.getErrorPage(c, http.StatusNotFound, "Session not found for provider")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"session": session,
	})
}

// putSession sets the default session provider
//
//	@Summary		Set default session provider
//	@Description	Update the default session provider for the user
//	@Tags			sessions
//	@Accept			json
//	@Produce		json
//	@Param		request	body		models.SessionSetDefaultRequest	true	"Provider selection request"
//	@Success		200		{object}	map[string]any		"Default provider updated successfully"
//	@Failure		400		{object}	map[string]any		"Bad request"
//	@Failure		404		{object}	map[string]any		"Provider session not found"
//	@Failure		500		{object}	map[string]any		"Internal server error"
//	@Router			/sessions [put]
func (s *Server) putSession(c *gin.Context) {

	// This endpoint can only be called in server mode
	if !s.Config.IsServer() {
		s.getErrorPage(c, http.StatusBadRequest, "Setting default session can only be called in server mode")
		return
	}

	// Parse the request body to get the provider name
	var requestBody models.SessionSetDefaultRequest
	if err := c.ShouldBindJSON(&requestBody); err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to parse request body", err)
		return
	}

	provider := requestBody.Provider

	logrus.WithFields(logrus.Fields{
		"provider": provider,
	}).Debugln("Setting default session provider")

	// Verify that the user has an active session for this provider
	remoteSessions, err := s.getUserSessions(c)
	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to verify session", err)
		return
	}

	session, exists := remoteSessions[provider]
	if !exists {
		s.getErrorPage(c, http.StatusNotFound, "No active session found for provider")
		return
	}

	// Validate that the session is not expired
	if session.Expiry.Before(time.Now()) {
		s.getErrorPage(c, http.StatusBadRequest, "Session for this provider has expired")
		return
	}

	// Update the default provider cookie
	if err := s.setDefaultProviderCookie(c, provider); err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to save default provider", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Default provider updated successfully",
		"provider": provider,
	})
}

// deleteSession removes a session
//
//	@Summary		Delete session
//	@Description	Remove a session for a specific provider
//	@Tags			sessions
//	@Accept			json
//	@Produce		json
//	@Param			provider	path		string					true	"Provider name"
//	@Success		200			{object}	map[string]any	"Session deleted successfully"
//	@Failure		400			{object}	map[string]any	"Bad request"
//	@Failure		500			{object}	map[string]any	"Internal server error"
//	@Router			/session/{provider} [delete]
func (s *Server) deleteSession(c *gin.Context) {

	provider := c.Param("provider")
	if len(provider) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Provider is required")
		return
	}

	logrus.WithFields(logrus.Fields{
		"provider": provider,
	}).Debugln("Deleting session for provider")

	if s.Config.IsServer() {
		// Server mode: Clear all cookies for this provider

		// Clear the provider-specific cookie
		s.clearProviderCookies(c, provider)

		// If this was the default provider, clear the default setting
		if s.getDefaultProviderCookieValue(c) == provider {
			s.clearDefaultProviderCookies(c)
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Session deleted successfully",
		})
		return

	} else if s.Config.IsAgent() {
		// Agent mode: Delete local session via session manager
		sessionMgr := sessionManager.GetSessionManager()
		err := sessionMgr.RemoveSession(s.Config.GetLoginServerHostname(), provider)

		if err != nil {
			s.getErrorPage(c, http.StatusInternalServerError, "Failed to delete session", err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Session deleted successfully",
		})
		return

	} else {
		s.getErrorPage(c, http.StatusBadRequest, "Delete session can only be called in agent or server mode")
		return
	}
}

// deleteSessions removes all sessions
//
//	@Summary		Delete all sessions
//	@Description	Remove all sessions for the user
//	@Tags			sessions
//	@Accept			json
//	@Produce		json
//	@Success		200			{object}	map[string]any	"All sessions deleted successfully"
//	@Failure		400			{object}	map[string]any	"Bad request"
//	@Failure		500			{object}	map[string]any	"Internal server error"
//	@Router			/sessions [delete]
func (s *Server) deleteSessions(c *gin.Context) {

	logrus.Debugln("Deleting all sessions")

	if s.Config.IsServer() {
		// Server mode: Get all user sessions and clear all cookies

		remoteSessions, err := s.getUserSessions(c)
		if err != nil {
			s.getErrorPage(c, http.StatusInternalServerError, "Failed to get user sessions", err)
			return
		}

		// Clear provider-specific cookies for each session
		for providerName := range remoteSessions {
			s.clearProviderCookies(c, providerName)
		}

		// Clear the default provider setting
		s.clearDefaultProviderCookies(c)

		c.JSON(http.StatusOK, gin.H{
			"message": "All sessions deleted successfully",
		})
		return

	} else if s.Config.IsAgent() {
		// Agent mode: Delete all local sessions via session manager
		loginServer := s.Config.GetLoginServerHostname()
		sessionMgr := sessionManager.GetSessionManager()
		sessionMgr.Load(loginServer)

		// Get all sessions
		loginServerData, err := sessionMgr.GetLoginServer(loginServer)
		if err != nil {
			s.getErrorPage(c, http.StatusInternalServerError, "Failed to get sessions", err)
			return
		}

		// Delete each session
		for providerName := range loginServerData.Sessions {
			err := sessionMgr.RemoveSession(loginServer, providerName)
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"provider": providerName,
					"error":    err,
				}).Warnln("Failed to delete session")
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "All sessions deleted successfully",
		})
		return

	} else {
		s.getErrorPage(c, http.StatusBadRequest, "Delete sessions can only be called in agent or server mode")
		return
	}
}
