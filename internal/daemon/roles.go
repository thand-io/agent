package daemon

import (
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hashicorp/go-version"
	"github.com/thand-io/agent/internal/models"
)

// EvaluateRoleRequest represents the request body for POST /roles/evaluate
type EvaluateRoleRequest struct {
	Role       string   `json:"role" binding:"required"`       // Role key (identifier) to evaluate
	Identities []string `json:"identities" binding:"required"` // Identity IDs to evaluate against
	Providers  []string `json:"providers,omitempty"`           // Optional list of providers to consider in evaluation
}

// EvaluateRoleResponse represents the response for POST /roles/evaluate
type EvaluateRoleResponse struct {
	Results map[string]*models.CompositeRole `json:"results"` // Map of identity ID to evaluated composite role
}

// getRoles handles GET /api/v1/roles
//
//	@Summary		List roles
//	@Description	Get a list of all available roles with optional provider filtering and search
//	@Tags			roles
//	@Accept			json
//	@Produce		json
//	@Param			provider	query		string					false	"Comma-separated list of providers to filter by"
//	@Param			q			query		string					false	"Search query for roles"
//	@Param			limit		query		int						false	"Maximum number of search results (default: 10)"
//	@Success		200			{object}	models.RolesResponse	"List of roles"
//	@Failure		401			{object}	map[string]any	"Unauthorized"
//	@Router			/roles [get]
//	@Security		BearerAuth
func (s *Server) getRoles(c *gin.Context) {
	// Get authenticated user if in server mode

	var foundAuthenticator string
	var foundSession *models.Session
	var err error

	if s.Config.IsServer() {

		foundAuthenticator, foundSession, err = s.getSession(c)

		if err != nil {
			s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to get user session", err)
			return
		}

	}

	// Parse query parameters
	providers := parseProviderList(c.Query("provider"))
	query := c.Query("q")
	limit := parseSearchLimit(c.Query("limit"), 10)

	// Get and filter roles
	var filteredRoles []models.RoleResponse

	if len(query) > 0 {
		filteredRoles, err = s.searchAndFilterRoles(c, query, limit, providers, foundAuthenticator, foundSession)
		if err != nil {
			s.getErrorPage(c, http.StatusInternalServerError, "Failed to search roles", err)
			return
		}
	} else {
		filteredRoles = s.filterAllRoles(providers, foundAuthenticator, foundSession)
	}

	// Build and render response
	response := models.RolesResponse{
		Version: version.Must(version.NewVersion("1.0.0")),
		Roles:   models.ReturnSearchResults(filteredRoles),
	}
	s.renderRolesResponse(c, response)
}

// parseProviderList parses comma-separated provider list from query parameter
func parseProviderList(providerParam string) []string {
	if len(providerParam) == 0 {
		return []string{}
	}
	return strings.Split(providerParam, ",")
}

// parseSearchLimit parses the limit query parameter with a default fallback
func parseSearchLimit(limitStr string, defaultLimit int) int {
	if len(limitStr) == 0 {
		return defaultLimit
	}
	if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
		return parsedLimit
	}
	return defaultLimit
}

// searchAndFilterRoles searches roles and applies filters
func (s *Server) searchAndFilterRoles(c *gin.Context, query string, limit int, providers []string, authenticatorProvider string, authenticatedUser *models.Session) ([]models.RoleResponse, error) {
	searchRequest := &models.SearchRequest{
		Limit: limit,
		Terms: []string{query},
		Query: query,
	}

	// Search roles using the index
	searchResults, err := s.Config.GetRolesConfig().ListRoles(c.Request.Context(), searchRequest)
	if err != nil {
		return nil, err
	}

	// Filter search results
	var filteredRoles []models.RoleResponse
	for _, result := range searchResults {
		role := result.Result
		if s.shouldIncludeRole(role, providers, authenticatorProvider, authenticatedUser) {
			filteredRoles = append(filteredRoles, role)
		}
	}

	// Sort alphabetically by Identifier
	sort.Slice(filteredRoles, func(i, j int) bool {
		return filteredRoles[i].Identifier < filteredRoles[j].Identifier
	})

	return filteredRoles, nil
}

// filterAllRoles returns all roles with filters applied
func (s *Server) filterAllRoles(providers []string, authenticatorProvider string, authenticatedUser *models.Session) []models.RoleResponse {
	var filteredRoles []models.RoleResponse
	for _, role := range s.Config.GetRolesConfig().Definitions {
		if s.shouldIncludeRole(role, providers, authenticatorProvider, authenticatedUser) {
			filteredRoles = append(filteredRoles, role)
		}
	}

	// Sort alphabetically by Identifier
	sort.Slice(filteredRoles, func(i, j int) bool {
		return filteredRoles[i].Identifier < filteredRoles[j].Identifier
	})

	return filteredRoles
}

// shouldIncludeRole checks if a role should be included based on filters
func (s *Server) shouldIncludeRole(role models.Role, providers []string, authenticatorProvider string, authenticatedUser *models.Session) bool {
	// Filter by provider
	if len(providers) > 0 && !hasAnyProvider(role.Providers, providers) {
		return false
	}

	// Filter by authenticator
	if len(authenticatorProvider) > 0 && len(role.Authenticators) > 0 && !slices.Contains(role.Authenticators, authenticatorProvider) {
		return false
	}

	// Filter by user permissions
	if authenticatedUser != nil && !role.HasPermission(authenticatedUser.User) {
		return false
	}

	return true
}

// renderRolesResponse renders the roles response as HTML or JSON
func (s *Server) renderRolesResponse(c *gin.Context, response models.RolesResponse) {
	if s.canAcceptHtml(c) {
		data := struct {
			TemplateData TemplateData
			Response     models.RolesResponse
		}{
			TemplateData: s.GetTemplateData(c),
			Response:     response,
		}
		s.renderHtml(c, "roles.html", data)
	} else {
		c.JSON(http.StatusOK, response)
	}
}

// hasAnyProvider checks if any provider in roleProviders exists in requestedProviders
func hasAnyProvider(roleProviders []string, requestedProviders []string) bool {
	for _, rp := range roleProviders {
		if slices.Contains(requestedProviders, rp) {
			return true
		}
	}
	return false
}

// getRoleByName handles GET /api/v1/role/:role
//
//	@Summary		Get role by key
//	@Description	Retrieve detailed information about a specific role by its key (identifier)
//	@Tags			roles
//	@Accept			json
//	@Produce		json
//	@Param			role	path		string					true	"Role key (identifier)"
//	@Success		200		{object}	models.RoleResponse		"Role details"
//	@Failure		400		{object}	map[string]any	"Bad request"
//	@Failure		404		{object}	map[string]any	"Role not found"
//	@Router			/role/{role} [get]
//	@Security		BearerAuth
func (s *Server) getRoleByName(c *gin.Context) {
	roleName := c.Param("role")

	if len(roleName) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Role name is required")
		return
	}

	role, exists := s.Config.Roles.Definitions[roleName]
	if !exists {
		s.getErrorPage(c, http.StatusNotFound, "Role not found")
		return
	}

	c.JSON(http.StatusOK, role)
}

func (s *Server) getRolesPage(c *gin.Context) {
	s.getRoles(c)
}

// RolePageData represents the data passed to the role.html template
type RolePageData struct {
	TemplateData
	RoleKey       string
	Role          *models.Role
	CompositeRole *models.CompositeRole
}

// getRolePage handles GET /role/:role
//
//	@Summary		Get role page
//	@Description	Display a page showing the composite role with all inherited permissions resolved
//	@Tags			roles
//	@Accept			html
//	@Produce		html
//	@Param			role	path		string	true	"Role key"
//	@Success		200		{object}	nil		"Role page HTML"
//	@Failure		400		{object}	map[string]any	"Bad request"
//	@Failure		401		{object}	map[string]any	"Unauthorized"
//	@Failure		404		{object}	map[string]any	"Role not found"
//	@Failure		500		{object}	map[string]any	"Internal server error"
//	@Router			/role/{role} [get]
func (s *Server) getRolePage(c *gin.Context) {
	roleKey := c.Param("role")

	if len(roleKey) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Role name is required")
		return
	}

	// Get the authenticated user's session
	_, foundSession, err := s.getSession(c)
	if err != nil {
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to get user", err)
		return
	}

	// Get the base role by name
	baseRole, err := s.Config.GetRoleByName(roleKey)
	if err != nil {
		s.getErrorPage(c, http.StatusNotFound, "Role not found", err)
		return
	}

	// Create identity from the authenticated user
	identity := &models.Identity{
		ID:    foundSession.User.GetIdentity(),
		Label: foundSession.User.GetName(),
		User:  foundSession.User,
	}

	// Evaluate the composite role with all inherited permissions resolved
	compositeRole, err := s.Config.GetCompositeRole(identity, baseRole)
	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to evaluate composite role", err)
		return
	}

	if s.canAcceptHtml(c) {
		data := RolePageData{
			TemplateData:  s.GetTemplateData(c),
			RoleKey:       roleKey,
			Role:          baseRole,
			CompositeRole: compositeRole,
		}
		s.renderHtml(c, "role.html", data)
	} else {
		c.JSON(http.StatusOK, compositeRole)
	}
}

// postEvaluateRole handles POST /api/v1/roles/evaluate
//
//	@Summary		Evaluate composite role
//	@Description	Evaluate a role against an identity to get the composite role with all inherited permissions resolved
//	@Tags			roles
//	@Accept			json
//	@Produce		json
//	@Param			request	body		EvaluateRoleRequest		true	"Role evaluation request"
//	@Success		200		{object}	EvaluateRoleResponse	"Evaluated composite role"
//	@Failure		400		{object}	map[string]any			"Bad request"
//	@Failure		401		{object}	map[string]any			"Unauthorized"
//	@Failure		404		{object}	map[string]any			"Role or identity not found"
//	@Failure		500		{object}	map[string]any			"Internal server error"
//	@Router			/roles/evaluate [post]
//	@Security		BearerAuth
func (s *Server) postEvaluateRole(c *gin.Context) {
	var request EvaluateRoleRequest

	if err := c.ShouldBindJSON(&request); err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid request payload", err)
		return
	}

	// Get the authenticated user
	_, _, err := s.getSession(c)
	if err != nil {
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	// Get the base role by name
	baseRole, err := s.Config.GetRoleByName(request.Role)
	if err != nil {
		s.getErrorPage(c, http.StatusNotFound, "Role not found", err)
		return
	}

	// Resolve provider objects from the requested provider names
	var resolvedProviders []models.Provider
	for _, providerName := range request.Providers {
		provider, err := s.Config.GetProviderByName(providerName)
		if err != nil {
			s.getErrorPage(c, http.StatusNotFound, "Provider not found", err)
			return
		}
		resolvedProviders = append(resolvedProviders, provider)
	}

	results := make(map[string]*models.CompositeRole)

	for _, identityID := range request.Identities {

		// Look up the identity from available identities
		identityResult, err := s.Config.GetIdentity(identityID)
		if err != nil {
			s.getErrorPage(c, http.StatusNotFound, "Identity not found", err)
			return
		}

		// Evaluate the composite role, scoped to the requested providers
		compositeRole, err := s.Config.GetCompositeRoleForIdentity(identityResult, baseRole, resolvedProviders...)
		if err != nil {
			s.getErrorPage(c, http.StatusInternalServerError, "Failed to evaluate composite role", err)
			return
		}

		results[identityID] = compositeRole
	}

	c.JSON(http.StatusOK, EvaluateRoleResponse{
		Results: results,
	})
}
