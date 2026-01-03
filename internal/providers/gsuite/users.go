package gsuite

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// SynchronizeUsers fetches and caches user identities from GSuite
func (p *gsuiteProvider) SynchronizeUsers(ctx context.Context, req *models.SynchronizeUsersRequest) (*models.SynchronizeUsersResponse, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Refreshed GSuite user identities in %s", elapsed)
	}()

	if req.Pagination == nil {
		req.Pagination = &models.PaginationOptions{
			PageSize: 100,
		}
	}

	call := p.adminService.Users.List().
		Domain(p.domain).
		MaxResults(int64(req.Pagination.PageSize)).
		OrderBy("email")

	if len(req.Pagination.Token) != 0 {
		call = call.PageToken(req.Pagination.Token)
	}

	resp, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	var identities []models.Identity
	for _, user := range resp.Users {
		identity := models.Identity{
			ID:    user.PrimaryEmail,
			Label: user.Name.FullName,
			User: &models.User{
				ID:       user.Id,
				Username: common.ExtractNameFromEmail(user.PrimaryEmail),
				Email:    user.PrimaryEmail,
				Name:     user.Name.FullName,
				Source:   "gsuite",
			},
		}

		identities = append(identities, identity)
	}

	response := models.SynchronizeUsersResponse{
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
	}).Debug("Refreshed GSuite user identities")

	return &response, nil
}
