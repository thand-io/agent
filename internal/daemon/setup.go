package daemon

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/thand-io/agent/internal/common"
)

// show a setup page - should only be accessible if no configuration is provided

type SetupPageData struct {
	TemplateData
	Configuration map[string]bool
}

func (s *Server) setupPage(c *gin.Context) {

	// If we can't accept HTML, return 421
	if !s.canAcceptHtml(c) {
		s.getErrorPage(c,
			http.StatusMisdirectedRequest,
			"Request cannot be processed as the server has not been, or is missing configuration state.")
	}

	if s.Config == nil {
		s.getErrorPage(c,
			http.StatusInternalServerError,
			"Server configuration is missing.")
		return
	}

	// Check if the login server URL is still the default
	defaultLoginEndpoint := s.Config.GetLoginServerUrl() == common.DefaultLoginServerEndpoint
	defaultSecret := s.Config.Secret == common.DefaultServerSecret

	defaultServicesTemporalHost := false
	defaultServicestemporalPort := false
	defaultServicesTemporalNamespace := false

	if s.Config.Services.Temporal != nil {

		// Check optional temporal is configured, correctly
		defaultServicesTemporalHost = len(s.Config.Services.Temporal.Host) == 0
		defaultServicestemporalPort = s.Config.Services.Temporal.Port <= 0
		defaultServicesTemporalNamespace = len(s.Config.Services.Temporal.Namespace) == 0

	}

	getServices := s.Config.GetServices()

	data := SetupPageData{
		TemplateData: s.GetTemplateData(c),
		// true = configured, false = default
		Configuration: map[string]bool{
			"login.endpoint": !defaultLoginEndpoint,
			"secret":         !defaultSecret,

			"services.temporal":           getServices.HasTemporal(), // A client has been configured
			"services.temporal.host":      !defaultServicesTemporalHost,
			"services.temporal.port":      !defaultServicestemporalPort,
			"services.temporal.namespace": !defaultServicesTemporalNamespace,

			"services.vault":     getServices.HasVault(),
			"services.encrypt":   getServices.HasEncryption(),
			"services.scheduler": getServices.HasScheduler(),
			"services.llm":       getServices.HasLargeLanguageModel(),
		},
	}

	s.renderHtml(c, "setup.html", data)

}
