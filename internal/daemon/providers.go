package daemon

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/thand-io/agent/internal/models"
)

// getProviderIdentities lists identities available in a provider
//
//	@Summary		List provider identities
//	@Description	Get a list of identities available in a specific provider
//	@Tags			providers
//	@Accept			json
//	@Produce		json
//	@Param			provider	path		string								true	"Provider name"
//	@Param			q			query		string								false	"Filter query"
//	@Success		200			{object}	models.ProviderIdentitiesResponse		"Provider identities"
//	@Failure		404			{object}	map[string]any				"Provider not found"
//	@Failure		500			{object}	map[string]any				"Internal server error"
//	@Router			/provider/{provider}/identities [get]
//	@Security		BearerAuth
func (s *Server) getProviderIdentities(c *gin.Context) {

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

	if !provider.GetClient().HasCapability(models.ProviderCapabilityIdentities) {
		s.getErrorPage(c, http.StatusNotImplemented, "The provider does not implement identities")
		return
	}

	filter := c.Query("q")

	identities, err := provider.GetClient().ListIdentities(
		context.Background(), &models.SearchRequest{
			Terms: []string{filter},
		})

	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to list identities", err)
		return
	}

	c.JSON(http.StatusOK, models.ProviderIdentitiesResponse{
		Version:    "1.0",
		Provider:   providerName,
		Identities: identities,
	})
}

// getProviderRoles lists roles available in a provider
//
//	@Summary		List provider roles
//	@Description	Get a list of roles available in a specific provider
//	@Tags			providers
//	@Accept			json
//	@Produce		json
//	@Param			provider	path		string								true	"Provider name"
//	@Param			q			query		string								false	"Filter query"
//	@Success		200			{object}	models.ProviderRolesResponse		"Provider roles"
//	@Failure		404			{object}	map[string]any				"Provider not found"
//	@Failure		500			{object}	map[string]any				"Internal server error"
//	@Router			/provider/{provider}/roles [get]
//	@Security		BearerAuth
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

	if !provider.GetClient().HasCapability(models.ProviderCapabilityRBAC) {
		s.getErrorPage(c, http.StatusNotImplemented, "The provider does not implement rbac")
		return
	}

	query := c.Query("q")

	searchRequest := &models.SearchRequest{
		Limit: 10,
	}

	if len(query) > 0 {
		searchRequest.Terms = []string{query}
		if !strings.HasSuffix(query, "*") {
			searchRequest.Query = query + "*"
		} else {
			searchRequest.Query = query
		}
	}

	roles, err := provider.GetClient().ListRoles(context.Background(), searchRequest)

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

// getProviderByName retrieves a provider by name
//
//	@Summary		Get provider by name
//	@Description	Retrieve detailed information about a specific provider
//	@Tags			providers
//	@Accept			json
//	@Produce		json
//	@Param			provider	path		string					true	"Provider name"
//	@Success		200			{object}	models.ProviderResponse	"Provider details"
//	@Failure		404			{object}	map[string]any	"Provider not found"
//	@Router			/provider/{provider} [get]
//	@Security		BearerAuth
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

// getProviderPermissions lists permissions available in a provider
//
//	@Summary		List provider permissions
//	@Description	Get a list of permissions available in a specific provider
//	@Tags			providers
//	@Accept			json
//	@Produce		json
//	@Param			provider	path		string									true	"Provider name"
//	@Param			q			query		string									false	"Filter query"
//	@Success		200			{object}	models.ProviderPermissionsResponse		"Provider permissions"
//	@Failure		404			{object}	map[string]any					"Provider not found"
//	@Failure		500			{object}	map[string]any					"Internal server error"
//	@Router			/provider/{provider}/permissions [get]
//	@Security		BearerAuth
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

	if !provider.GetClient().HasCapability(models.ProviderCapabilityRBAC) {
		s.getErrorPage(c, http.StatusNotImplemented, "The provider does not implement rbac")
		return
	}

	query := c.Query("q")

	searchRequest := &models.SearchRequest{
		Limit: 10,
	}

	if len(query) > 0 {
		searchRequest.Terms = []string{query}
		if !strings.HasSuffix(query, "*") {
			searchRequest.Query = query + "*"
		} else {
			searchRequest.Query = query
		}
	}

	permissions, err := provider.GetClient().ListPermissions(context.Background(), searchRequest)

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
		models.ProviderCapabilityAuthorizer)
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
			ID:          providerKey,
			Name:        providerName,
			Description: provider.Description,
			Provider:    provider.Provider,
			Enabled:     true,
		}
	}
	return providerResponse
}

// getProviders handles GET /api/v1/providers
//
//	@Summary		List providers
//	@Description	Get a list of all available providers with optional capability filtering
//	@Tags			providers
//	@Accept			json
//	@Produce		json
//	@Param			capability	query		string						false	"Comma-separated list of capabilities to filter by"
//	@Success		200			{object}	models.ProvidersResponse	"List of providers"
//	@Failure		401			{object}	map[string]any		"Unauthorized"
//	@Router			/providers [get]
//	@Security		BearerAuth
func (s *Server) getProviders(c *gin.Context) {

	var authenticatedUser *models.Session

	// If we're in server mode then we need to ensure the user is authenticated
	// before we return any roles
	// This is because roles can contain sensitive information
	// and we want to ensure that only authenticated users can access them
	if s.Config.IsServer() {
		_, foundUser, err := s.getUser(c)
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
			TemplateData TemplateData
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

// postProviderAuthorizeSession authorizes a session with a provider
//
//	@Summary		Authorize provider session
//	@Description	Authorize a session with a specific provider
//	@Tags			providers
//	@Accept			json
//	@Produce		json
//	@Param			provider	path		string						true	"Provider name"
//	@Param			user		body		models.AuthorizeUser		true	"Authorization request"
//	@Success		200			{object}	map[string]any		"Authorization response"
//	@Failure		400			{object}	map[string]any		"Bad request"
//	@Failure		404			{object}	map[string]any		"Provider not found"
//	@Failure		500			{object}	map[string]any		"Internal server error"
//	@Router			/provider/{provider}/authorizeSession [post]
//	@Security		BearerAuth
func (s *Server) postProviderAuthorizeSession(c *gin.Context) {

	// User in body
	var user models.AuthorizeUser
	if err := c.ShouldBindJSON(&user); err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid request payload")
		return
	}

	provider, err := s.getProvider(c.Param("provider"))

	if err != nil {
		s.getErrorPage(c, http.StatusNotFound, "Provider not found", err)
		return
	}

	authResponse, err := provider.GetClient().AuthorizeSession(context.Background(), &user)

	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to authorize session", err)
		return
	}

	c.JSON(http.StatusOK, authResponse)
}

func (s *Server) getProvider(providerName string) (*models.Provider, error) {

	provider, err := s.Config.GetProviderByName(providerName)

	if err != nil {
		return nil, fmt.Errorf("provider '%s' not found", providerName)
	}

	if provider.GetClient() == nil {
		return nil, fmt.Errorf("provider has no client defined")
	}

	return provider, nil
}

func (s *Server) getProvidersPage(c *gin.Context) {
	s.getProviders(c)
}
