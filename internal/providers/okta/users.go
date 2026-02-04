package okta

import (
	"context"
	"fmt"
	"time"

	"github.com/okta/okta-sdk-golang/v2/okta/query"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

// SynchronizeUsers fetches and caches user identities from Okta
func (p *oktaProvider) SynchronizeUsers(ctx context.Context, req *models.SynchronizeUsersRequest) (*models.SynchronizeUsersResponse, error) {

	startTime := time.Now()

	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Refreshed Okta user identities in %s", elapsed)
	}()

	if req.Pagination == nil {
		req.Pagination = &models.PaginationOptions{
			// Moderately low pagination size as we have to get groups for each user later
			// and if we fail we have to re-query all the users again
			PageSize: 25,
		}
	}

	queryParams := &query.Params{
		Limit: int64(req.Pagination.PageSize),
	}

	if len(req.Pagination.Token) != 0 {
		queryParams.After = req.Pagination.Token
	}

	users, resp, err := p.client.User.ListUsers(ctx, queryParams)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	var identities []models.Identity

	for _, user := range users {
		email := ""
		name := ""
		if user.Profile != nil {
			if emailVal, ok := (*user.Profile)["email"].(string); ok {
				email = emailVal
			}
			if nameVal, ok := (*user.Profile)["firstName"].(string); ok {
				name = nameVal
			}
			if lastNameVal, ok := (*user.Profile)["lastName"].(string); ok {
				if len(name) != 0 {
					name = name + " " + lastNameVal
				} else {
					name = lastNameVal
				}
			}
		}

		// Fetch groups for this user
		groups := []string{}
		userGroups, _, err := p.client.User.ListUserGroups(ctx, user.Id)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"user_id":    user.Id,
				"user_email": email,
			}).WithError(err).Warn("Failed to fetch groups for user")
		} else {
			for _, group := range userGroups {
				groups = append(groups, group.Profile.Name)
			}
		}

		identity := models.Identity{
			ID:    email,
			Label: name,
			User: &models.User{
				ID:     user.Id,
				Email:  email,
				Name:   name,
				Source: "okta",
				Groups: groups,
			},
		}

		identities = append(identities, identity)
	}

	response := models.SynchronizeUsersResponse{
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
	}).Debug("Refreshed Okta user identities")

	return &response, nil
}
