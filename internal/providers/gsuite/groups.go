package gsuite

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

// SynchronizeGroups fetches and caches group identities from GSuite
func (p *gsuiteProvider) SynchronizeGroups(ctx context.Context, req *models.SynchronizeGroupsRequest) (*models.SynchronizeGroupsResponse, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Refreshed GSuite group identities in %s", elapsed)
	}()

	if req.Pagination == nil {
		req.Pagination = &models.PaginationOptions{
			PageSize: 100,
		}
	}

	call := p.adminService.Groups.List().
		Domain(p.domain).
		MaxResults(int64(req.Pagination.PageSize)).
		OrderBy("email")

	if len(req.Pagination.Token) != 0 {
		call = call.PageToken(req.Pagination.Token)
	}

	resp, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list groups: %w", err)
	}

	var identities []models.Identity
	for _, group := range resp.Groups {
		identity := models.Identity{
			ID:    group.Email,
			Label: group.Name,
			Group: &models.Group{
				ID:    group.Id,
				Name:  group.Name,
				Email: group.Email,
			},
		}

		identities = append(identities, identity)
	}

	response := models.SynchronizeGroupsResponse{
		Identities: identities,
	}

	if len(resp.NextPageToken) != 0 {
		response.Pagination = &models.PaginationOptions{
			Token:    resp.NextPageToken,
			PageSize: req.Pagination.PageSize,
		}
	}

	logrus.WithFields(logrus.Fields{
		"count": len(identities),
	}).Debug("Refreshed GSuite group identities")

	return &response, nil
}
