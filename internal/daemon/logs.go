package daemon

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
)

type LogPageData struct {
	TemplateData
	Logs []*models.LogEntry
}

func (s *Server) getLogsPage(c *gin.Context) {

	// Check if we have a valid user

	if s.Config.IsServer() {
		_, _, err := s.getSession(c)
		if err != nil {
			s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to get user for list of available roles", err)
			return
		}
	}

	filter := config.LogFilter{
		Limit:  500,
		Search: c.Query("search"),
	}

	// Parse optional limit override
	if limitStr := c.Query("limit"); limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			filter.Limit = n
		}
	}

	// Parse optional level filter (single value, e.g. "info", "warning", "error")
	if levelStr := c.Query("level"); levelStr != "" {
		if level, err := logrus.ParseLevel(levelStr); err == nil {
			filter.Levels = []logrus.Level{level}
		}
	}

	logs := s.Config.GetEventsWithFilter(filter)

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
