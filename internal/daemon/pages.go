package daemon

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
)

type ErrorPageData struct {
	config.TemplateData
	Error models.ErrorResponse
}

// getErrorPage handles the request for the error page
func (s *Server) getErrorPage(c *gin.Context, code int, message string, err ...error) {

	var messages []string

	if len(err) == 0 {

		logrus.WithField("code", code).Errorln(message)

	} else {

		// Log all errors
		for _, e := range err {
			logrus.WithError(e).Errorln(message)
			messages = append(messages, e.Error())
		}
	}

	// Don't show error details for 500 status codes
	showDetails := code != http.StatusInternalServerError
	errorMessage := fmt.Sprintf("An internal error occurred. Details are available in the logs at: %s.", time.Now().UTC().Format("2006-01-02 15:04:05"))

	if showDetails {
		errorMessage = strings.Join(messages, ". ")
	}

	errReponse := models.ErrorResponse{
		Code:    code,
		Title:   message,
		Message: errorMessage,
	}

	if s.canAcceptHtml(c) {

		data := ErrorPageData{
			TemplateData: s.GetTemplateData(c),
			Error:        errReponse,
		}

		s.renderHtml(c, "error.html", data)

	} else {
		c.JSON(code, errReponse)
	}
}

func (s *Server) renderHtml(c *gin.Context, template string, data any) {

	c.Header("Content-Type", "text/html; charset=utf-8")

	err := s.GetTemplateEngine().ExecuteTemplate(c.Writer, template, data)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error rendering page: %v", err)
		return
	}
}

func (s *Server) getIndexPage(c *gin.Context) {
	data := s.GetTemplateData(c)
	s.renderHtml(c, "index.html", data)
}
