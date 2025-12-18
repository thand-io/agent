package daemon

import (
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
)

// EvaluateRoleRequest represents the request body for POST /roles/evaluate
type EvaluateRoleRequest struct {
	Role     string `json:"role" binding:"required"`     // Role name to evaluate
	Identity string `json:"identity" binding:"required"` // Identity ID to evaluate against
}

// EvaluateRoleResponse represents the response for POST /roles/evaluate
type EvaluateRoleResponse struct {
	Role *models.Role `json:"role"` // The evaluated composite role
}

// getRoles handles GET /api/v1/roles
//
//	@Summary		List roles
//	@Description	Get a list of all available roles with optional provider filtering
//	@Tags			roles
//	@Accept			json
//	@Produce		json
//	@Param			provider	query		string					false	"Comma-separated list of providers to filter by"
//	@Success		200			{object}	models.RolesResponse	"List of roles"
//	@Failure		401			{object}	map[string]any	"Unauthorized"
//	@Router			/roles [get]
//	@Security		BearerAuth
func (s *Server) getRoles(c *gin.Context) {

	var authenticatedUser *models.Session

	// If we're in server mode then we need to ensure the user is authenticated
	// before we return any roles
	// This is because roles can contain sensitive information
	// and we want to ensure that only authenticated users can access them
	if s.Config.IsServer() {
		_, foundUser, err := s.getUser(c)
		if err != nil {
			s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to get user for list of available roles", err)
			return
		}
		authenticatedUser = foundUser
	}

	// Allow to filter by providers can be comma separated
	// to allow for filtering by multiple providers

	provider := c.Query("provider")
	providers := []string{}

	if len(provider) > 0 {
		foundProviders := strings.Split(provider, ",")
		providers = append(providers, foundProviders...)
	}

	// Filter out roles that are not in the requested providers
	filteredRoles := make(map[string]models.RoleResponse)
	for roleName, role := range s.Config.GetRoles().Definitions {
		if len(providers) > 0 && !hasAnyProvider(role.Providers, providers) {
			continue
		}
		if authenticatedUser != nil && !role.HasPermission(authenticatedUser.User) {
			continue
		}
		filteredRoles[roleName] = models.RoleResponse{
			Role: role,
		}
	}

	// Now if a query param is provided. We need to filter through the roles,
	// this gets more interesting as a user might be search for a partial inheritance
	// match or permission match.

	// Given we support wildcards the first thing we need to do is resolve and create a
	// composite role for the user to match against.

	response := models.RolesResponse{
		Version: "1.0",
		Roles:   filteredRoles,
	}

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
//	@Summary		Get role by name
//	@Description	Retrieve detailed information about a specific role
//	@Tags			roles
//	@Accept			json
//	@Produce		json
//	@Param			role	path		string					true	"Role name"
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
	_, session, err := s.getUser(c)
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

	// Look up the identity from available identities
	identityResult, err := s.findIdentityByID(session.User, request.Identity)
	if err != nil {
		s.getErrorPage(c, http.StatusNotFound, "Identity not found", err)
		return
	}

	// Evaluate the composite role
	compositeRole, err := s.Config.GetCompositeRole(&identityResult.Result, baseRole)
	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to evaluate composite role", err)
		return
	}

	c.JSON(http.StatusOK, EvaluateRoleResponse{
		Role: compositeRole,
	})
}

// findIdentityByID looks up an identity by its ID from available identity sources
func (s *Server) findIdentityByID(user *models.User, identityID string) (*models.SearchResult[models.Identity], error) {
	// Get identities from all providers for this user
	identities, err := s.Config.GetIdentitiesWithFilter(user, config.IdentityTypeAll, &models.SearchRequest{})
	if err != nil {
		return nil, err
	}

	for _, identity := range identities {
		if identity.ID == identityID {
			return &identity, nil
		}
	}

	// If no exact match found, create a basic identity with just the ID
	// This allows evaluation for identities that may not be in the system yet
	return &models.SearchResult[models.Identity]{
		Result: models.Identity{
			ID:    identityID,
			Label: identityID,
		},
	}, nil
}
