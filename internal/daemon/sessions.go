package daemon

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/sessions"
)

func (s *Server) postSession(c *gin.Context) {

	// Get the post JSON Body as a Session create request
	// which is a struct with fields for session creation
	var sessionCreateRequest models.SessionCreateRequest
	if err := c.ShouldBindJSON(&sessionCreateRequest); err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to parse request body", err)
		return
	}

	sessionToken := sessionCreateRequest.Session

	// The session token is an encoded local session
	// The payload is encrypted
	sessionData, err := models.EncodingWrapper{
		Type: models.ENCODED_SESSION_LOCAL,
		Data: sessionToken,
	}.Decode(sessionToken)

	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to decode session token", err)
		return
	}

	var session models.LocalSession
	err = common.ConvertMapToInterface(sessionData.Data.(map[string]any), &session)

	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to convert session data", err)
		return
	}

	// Now lets store the session in the users local session manager.
	sessionManager := sessions.GetSessionManager()
	err = sessionManager.AddSession(
		s.Config.GetLoginServerHostname(),
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

func (s *Server) getSessions(c *gin.Context) {

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

		sessionsList := sessions.LoginServer{
			Version:   "1",
			Timestamp: time.Now(),
			Sessions:  foundSessions,
		}

		c.JSON(http.StatusOK, sessionsList)
		return

	} else if s.Config.IsAgent() {

		loginServer := s.Config.GetLoginServerHostname()

		logrus.WithFields(logrus.Fields{
			"loginServer": loginServer,
		}).Debugln("Fetching sessions")

		sessionManager := sessions.GetSessionManager()
		sessionManager.Load(loginServer)
		sessionsList, err := sessionManager.GetLoginServer(loginServer)

		if err != nil {
			s.getErrorPage(c, http.StatusInternalServerError, "Failed to list sessions", err)
			return
		}

		c.JSON(http.StatusOK, sessionsList)
		return

	} else {

		s.getErrorPage(c, http.StatusBadRequest, "Get sessions can only be called in agent or server mode")
		return
	}
}

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

	sessionManager := sessions.GetSessionManager()
	sessionManager.Load(loginServer)
	session, err := sessionManager.GetSession(loginServer, provider)

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

func (s *Server) deleteSession(c *gin.Context) {

	provider := c.Param("provider")
	if len(provider) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Provider is required")
		return
	}

	sessionManager := sessions.GetSessionManager()
	err := sessionManager.RemoveSession(s.Config.GetLoginServerHostname(), provider)

	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to delete session", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Session deleted successfully",
	})
}
