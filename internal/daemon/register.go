package daemon

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/thand-io/agent/internal/config"
)

// postRegister handles agent registration
//
//	@Summary		Register agent
//	@Description	Register a new agent with the server
//	@Tags			registration
//	@Accept			json
//	@Produce		json
//	@Param			registration	body		config.RegistrationRequest	true	"Registration request"
//	@Success		200				{object}	config.RegistrationResponse	"Registration successful"
//	@Failure		400				{object}	map[string]any		"Bad request"
//	@Router			/register [post]
func (s *Server) postRegister(c *gin.Context) {

	var registrationRequest config.RegistrationRequest
	if err := c.ShouldBindJSON(&registrationRequest); err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid request payload", err)
		return
	}

	if registrationRequest.Environment == nil {
		s.getErrorPage(c, http.StatusBadRequest, "Environment configuration is required")
		return
	}

	// This is pretty much a stub for now and to provide
	// a config for the cli

	cfg := s.GetConfig()

	c.JSON(http.StatusOK, config.RegistrationResponse{
		Success:   true,
		Services:  &cfg.Services,
		Roles:     &cfg.Roles,
		Providers: &cfg.Providers,
		Workflows: &cfg.Workflows,
	})

}
