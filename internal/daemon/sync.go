package daemon

import (
	"time"

	"github.com/gin-gonic/gin"
)

// TODO: create. a true sync endpoint

func (s *Server) getSync(c *gin.Context) {
	c.JSON(200, gin.H{
		"version":   s.GetVersion(),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
