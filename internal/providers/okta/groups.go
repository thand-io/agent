package okta

import (
	"context"
	"fmt"
	"time"

	"github.com/okta/okta-sdk-golang/v2/okta/query"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

// SynchronizeGroups fetches and caches group identities from Okta
func (p *oktaProvider) SynchronizeGroups(ctx context.Context, req *models.SynchronizeGroupsRequest) (*models.SynchronizeGroupsResponse, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Refreshed Okta group identities in %s", elapsed)
	}()

	if req.Pagination == nil {
		req.Pagination = &models.PaginationOptions{
			PageSize: 100,
		}
	}

	queryParams := &query.Params{
		Limit: int64(req.Pagination.PageSize),
	}

	if len(req.Pagination.Token) != 0 {
		queryParams.After = req.Pagination.Token
	}

	groups, resp, err := p.client.Group.ListGroups(ctx, queryParams)
	if err != nil {
		return nil, fmt.Errorf("failed to list groups: %w", err)
	}

	var identities []models.Identity
	for _, group := range groups {
		identity := models.Identity{
			ID:    group.Id,
			Label: group.Profile.Name,
			Group: &models.Group{
				ID:   group.Id,
				Name: group.Profile.Name,
			},
		}

		identities = append(identities, identity)
	}

	response := models.SynchronizeGroupsResponse{
		Identities: identities,
	}

	// Handle pagination
	if len(resp.NextPage) != 0 {
		token := p.GetNextTokenFromResponse(resp)

		if len(token) > 0 {
			response.Pagination = &models.PaginationOptions{
				Token:    token,
				PageSize: req.Pagination.PageSize,
			}
		}
	}

	logrus.WithFields(logrus.Fields{
		"count": len(identities),
	}).Debug("Refreshed Okta group identities")

	return &response, nil
}
