package daemon

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
)

type LogPageData struct {
	config.TemplateData
	Logs []*models.LogEntry
}

func (s *Server) getLogsPage(c *gin.Context) {

	// Check if we have a valid user

	if s.Config.IsServer() {
		_, _, err := s.getUser(c)
		if err != nil {
			s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to get user for list of available roles", err)
			return
		}
	}

	logs := s.Config.GetEventsWithFilter(config.LogFilter{
		Limit: 500,
	})

	if s.canAcceptHtml(c) {

		c.HTML(http.StatusOK, "logs.html", LogPageData{
			TemplateData: s.GetTemplateData(c),
			Logs:         logs,
		})

	} else {

		c.JSON(http.StatusOK, gin.H{
			"logs": logs,
		})
	}

}
