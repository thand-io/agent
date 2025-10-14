package daemon

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/thand-io/agent/internal/models"
)

func (s *Server) getIdentities(c *gin.Context) {

	// Get user information

	if !s.Config.IsServer() {
		s.getErrorPage(c, http.StatusForbidden, "Identities endpoint is only available in server mode")
		return
	}
	foundUser, err := s.getUser(c)
	if err != nil {
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to get user for list of available roles", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"identities": []models.User{
			*foundUser.User,
		},
	})
}
