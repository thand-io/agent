package daemon

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/sessions"
)

type SessionResponse struct {
	ExpiresAt time.Time `json:"expires_at"`
}

type UserPageData struct {
	config.TemplateData
	Sessions sessions.LoginServer `json:"sessions"`
	Callback string               `json:"callback"`
}

// getUserPage handles the request for the user page

func (s *Server) getUserPage(c *gin.Context) {

	config := s.GetConfig()

	callback, foundCallback := c.GetQuery("callback")

	if !foundCallback || len(callback) == 0 {
		callback = config.GetLocalServerUrl()
	}

	remoteSessions, err := s.getUserSessions(c)

	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to get user sessions", err)
		return
	}

	foundSessions := map[string]models.LocalSession{}

	// Convert to response format
	for providerName, session := range remoteSessions {
		foundSessions[providerName] = models.LocalSession{
			Expiry: session.Expiry,
		}
	}

	sessionsList := sessions.LoginServer{
		Version:   "1",
		Timestamp: time.Now(),
		Sessions:  foundSessions,
	}

	data := UserPageData{
		TemplateData: s.GetTemplateData(c),
		Sessions:     sessionsList,
		Callback:     callback,
	}

	s.renderHtml(c, "user.html", data)
}
