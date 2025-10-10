package daemon

import (
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/thand-io/agent/internal/models"
)

func (s *Server) setAuthCookie(c *gin.Context, authProvider string, localSession *models.LocalSession) error {

	cookie := sessions.Default(c)
	cookie.Set(authProvider, localSession.GetEncodedLocalSession())
	err := cookie.Save()

	return err

}
