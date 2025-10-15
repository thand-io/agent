package daemon

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
)

func (s *Server) getProviderRoles(c *gin.Context) {

	providerName := c.Param("provider")
	provider, foundProvider := s.Config.Providers.Definitions[providerName]

	if !foundProvider {
		s.getErrorPage(c, http.StatusNotFound, "Provider not found")
		return
	}

	if provider.GetClient() == nil {
		s.getErrorPage(c, http.StatusNotFound, "Provider has no client defined")
		return
	}

	filter := c.Query("q")

	roles, err := provider.GetClient().ListRoles(context.Background(), filter)
	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to list roles")
		return
	}

	c.JSON(http.StatusOK, models.ProviderRolesResponse{
		Version:  "1.0",
		Provider: providerName,
		Roles:    roles,
	})
}

func (s *Server) getProviderByName(c *gin.Context) {

	providerName := c.Param("provider")
	provider := s.Config.Providers.Definitions[providerName]

	if provider.GetClient() == nil {
		s.getErrorPage(c, http.StatusNotFound, "Provider not found")
		return
	}

	c.JSON(http.StatusOK, models.ProviderResponse{
		Name:        provider.Name,
		Description: provider.Description,
		Provider:    provider.Provider,
		Enabled:     true,
	})
}

func (s *Server) getProviderPermissions(c *gin.Context) {

	providerName := c.Param("provider")

	provider, foundProvider := s.Config.Providers.Definitions[providerName]

	if !foundProvider {
		s.getErrorPage(c, http.StatusNotFound, "Provider not found")
		return
	}

	if provider.GetClient() == nil {
		s.getErrorPage(c, http.StatusNotFound, "Provider has no client defined")
		return
	}

	filter := c.Query("q")

	permissions, err := provider.GetClient().ListPermissions(context.Background(), filter)
	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to list permissions", err)
		return
	}

	c.JSON(http.StatusOK, models.ProviderPermissionsResponse{
		Version:     "1.0",
		Provider:    providerName,
		Permissions: permissions,
	})
}

func (s *Server) getAuthProvidersAsProviderResponse(authenticatedUser *models.Session) map[string]models.ProviderResponse {
	return s.getProvidersAsProviderResponse(
		authenticatedUser,
		models.ProviderCapabilityAuthorizor)
}

func (s *Server) getProvidersAsProviderResponse(
	authenticatedUser *models.Session,
	capabilities ...models.ProviderCapability,
) map[string]models.ProviderResponse {

	providerResponse := map[string]models.ProviderResponse{}

	for providerKey, provider := range s.Config.Providers.Definitions {

		providerName := providerKey

		if len(provider.Name) > 0 {
			providerName = provider.Name
		}

		// Skip providers that don't have a client initialized
		if provider.GetClient() == nil {
			continue
		}

		if len(capabilities) > 0 && !provider.GetClient().HasAnyCapability(capabilities...) {
			continue
		}

		if authenticatedUser != nil && !provider.HasPermission(authenticatedUser.User) {
			continue
		}

		providerResponse[providerKey] = models.ProviderResponse{
			Name:        providerName,
			Description: provider.Description,
			Provider:    provider.Provider,
			Enabled:     true,
		}
	}
	return providerResponse
}

// getProviders handles GET /api/v1/providers
func (s *Server) getProviders(c *gin.Context) {

	var authenticatedUser *models.Session

	// If we're in server mode then we need to ensure the user is authenticated
	// before we return any roles
	// This is because roles can contain sensitive information
	// and we want to ensure that only authenticated users can access them
	if s.Config.IsServer() {
		foundUser, err := s.getUser(c)
		if err != nil {
			s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to get user for list of available providers", err)
			return
		}
		authenticatedUser = foundUser
	}

	// Add query filters for filtering by capability
	// these are comma separated
	capability := c.Query("capability")
	capabilities := []models.ProviderCapability{}

	if len(capability) > 0 {
		for cap := range strings.SplitSeq(capability, ",") {
			if parsedCap, err := models.GetCapabilityFromString(cap); err == nil {
				capabilities = append(capabilities, parsedCap)
			}
		}
	}

	response := models.ProvidersResponse{
		Version:   "1.0",
		Providers: s.getProvidersAsProviderResponse(authenticatedUser, capabilities...),
	}

	if s.canAcceptHtml(c) {

		data := struct {
			TemplateData config.TemplateData
			Response     models.ProvidersResponse
		}{
			TemplateData: s.GetTemplateData(c),
			Response:     response,
		}
		s.renderHtml(c, "providers.html", data)

	} else {

		c.JSON(http.StatusOK, response)
	}
}

func (s *Server) postProviderAuthorizeSession(c *gin.Context) {

	// User in body
	var user models.AuthorizeUser
	if err := c.ShouldBindJSON(&user); err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid request payload")
		return
	}

	// Call provider to authorize session
	providerName := c.Param("provider")
	provider, foundProvider := s.Config.Providers.Definitions[providerName]

	if !foundProvider {
		s.getErrorPage(c, http.StatusNotFound, "Provider not found")
		return
	}

	if provider.GetClient() == nil {
		s.getErrorPage(c, http.StatusNotFound, "Provider has no client defined")
		return
	}

	authResponse, err := provider.GetClient().AuthorizeSession(context.Background(), &user)

	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to authorize session")
		return
	}

	c.JSON(http.StatusOK, authResponse)
}

func (s *Server) getProvidersPage(c *gin.Context) {
	s.getProviders(c)
}
