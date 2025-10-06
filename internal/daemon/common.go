package daemon

import (
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) canAcceptHtml(c *gin.Context) bool {
	accept := c.GetHeader("Accept")
	return strings.Contains(accept, "text/html")
}
