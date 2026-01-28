package cloudflare

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

// SynchronizeUsers fetches and caches user identities from Cloudflare
func (p *cloudflareProvider) SynchronizeUsers(ctx context.Context, req *models.SynchronizeUsersRequest) (*models.SynchronizeUsersResponse, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Refreshed Cloudflare identities in %s", elapsed)
	}()

	accountID := p.GetAccountID()

	if req.Pagination == nil {
		req.Pagination = &models.PaginationOptions{
			Page:     1,
			PageSize: 100,
		}
	}

	// List all account members
	members, resultInfo, err := p.client.AccountMembers(ctx, accountID, cloudflare.PaginationOptions{
		PerPage: req.Pagination.PageSize,
		Page:    req.Pagination.Page,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list account members: %w", err)
	}

	var identities []models.Identity

	for _, member := range members {
		identity := models.Identity{
			ID:    member.ID,
			Label: member.User.Email,
			User: &models.User{
				ID:     member.User.ID,
				Email:  member.User.Email,
				Name:   fmt.Sprintf("%s %s", member.User.FirstName, member.User.LastName),
				Source: "cloudflare",
			},
		}

		identities = append(identities, identity)
	}

	logrus.WithFields(logrus.Fields{
		"identities": len(identities),
	}).Debug("Refreshed Cloudflare identities")

	return &models.SynchronizeUsersResponse{
		Pagination: &models.PaginationOptions{
			Page:     resultInfo.Page + 1,
			PageSize: resultInfo.PerPage,
		},
		Identities: identities,
	}, nil
}
