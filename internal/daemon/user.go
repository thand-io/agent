package daemon

import (
	"github.com/gin-gonic/gin"
	"github.com/thand-io/agent/internal/config"
)

type UserPageData struct {
	config.TemplateData
	Callback string
}

// getUserPage handles the request for the user page

func (s *Server) getUserPage(c *gin.Context) {

	config := s.GetConfig()

	callback, foundCallback := c.GetQuery("callback")

	if !foundCallback || len(callback) == 0 {
		callback = config.GetLocalServerUrl()
	}

	data := UserPageData{
		TemplateData: s.GetTemplateData(c),
		Callback:     callback,
	}

	s.renderHtml(c, "user.html", data)
}
