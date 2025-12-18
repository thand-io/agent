package daemon

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
)

// getIdentities retrieves available identities
//
//	@Summary		List identities
//	@Description	Get a list of available identities from all identity providers
//	@Tags			identities
//	@Accept			json
//	@Produce		json
//	@Param			q	query		string					false	"Filter query"
//	@Success		200	{object}	map[string]any	"List of identities"
//	@Failure		401	{object}	map[string]any	"Unauthorized"
//	@Failure		403	{object}	map[string]any	"Forbidden"
//	@Failure		500	{object}	map[string]any	"Internal server error"
//	@Router			/identities [get]
//	@Security		BearerAuth
func (s *Server) getIdentities(c *gin.Context) {
	// Get user information
	if !s.Config.IsServer() {
		s.getErrorPage(c, http.StatusForbidden, "Identities endpoint is only available in server mode")
		return
	}

	_, foundUser, err := s.getUser(c)
	if err != nil {
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to get user for list of available roles", err)
		return
	}

	if foundUser == nil {
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: user not found")
		return
	}

	// Get filter parameter from query string
	query := c.Query("q")
	identityType := strings.ToLower(c.Query("t"))

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

	identityProvidersCount := s.Config.GetProvidersByCapabilityWithUser(
		foundUser.User, models.ProviderCapabilityIdentities)
	identities, err := s.Config.GetIdentitiesWithFilter(
		foundUser.User, config.IdentityType(identityType), searchRequest)

	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to get identities", err)
		return
	}

	// Return the aggregated identities
	c.JSON(http.StatusOK, gin.H{
		"identities": identities,
		"providers":  len(identityProvidersCount),
	})
}
