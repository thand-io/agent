package daemon

import (
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
)

// getRoles handles GET /api/v1/roles
func (s *Server) getRoles(c *gin.Context) {

	var authenticatedUser *models.Session

	// If we're in server mode then we need to ensure the user is authenticated
	// before we return any roles
	// This is because roles can contain sensitive information
	// and we want to ensure that only authenticated users can access them
	if s.Config.IsServer() {
		foundUser, err := s.getUser(c)
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
	for roleName, role := range s.Config.Roles.Definitions {
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

	response := models.RolesResponse{
		Version: "1.0",
		Roles:   filteredRoles,
	}

	if s.canAcceptHtml(c) {

		data := struct {
			TemplateData config.TemplateData
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
