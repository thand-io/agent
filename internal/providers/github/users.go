package github

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/v57/github"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

// Sync fetches and caches user and team identities from GitHub
func (p *githubProvider) SynchronizeUsers(ctx context.Context, req *models.SynchronizeUsersRequest) (*models.SynchronizeUsersResponse, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Refreshed GitHub identities in %s", elapsed)
	}()

	if p.client == nil {
		return nil, fmt.Errorf("github client is not initialized")
	}

	if len(p.organizationName) == 0 {
		logrus.Warn("GitHub provider configuration missing 'organization', cannot fetch identities")
		return nil, fmt.Errorf("missing required GitHub configuration: organization")
	}

	if req.Pagination == nil {
		req.Pagination = &models.PaginationOptions{
			Page:     1,
			PageSize: 100,
		}
	}

	var identities []models.Identity
	opts := &github.ListMembersOptions{
		ListOptions: github.ListOptions{
			Page:    req.Pagination.Page,
			PerPage: req.Pagination.PageSize,
		},
	}

	members, resp, err := p.client.Organizations.ListMembers(ctx, p.organizationName, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch organization members: %w", err)
	}

	for _, member := range members {
		if member == nil {
			continue
		}

		// Extract user details
		login := member.GetLogin()
		id := member.GetID()

		if len(login) == 0 || id == 0 {
			continue
		}

		// Fetch full user details to get email
		// Note: This might hit rate limits for large organizations
		// fullUser, _, err := client.Users.Get(ctx, login)
		// if err != nil {
		//	logrus.Warnf("Failed to fetch user details for %s: %v", login, err)
		// } else {
		//	member = fullUser
		//}

		userId := fmt.Sprintf("%d", id)

		user := &models.User{
			ID:       userId,
			Username: login,
			Email:    member.GetEmail(),
			Source:   "github",
		}

		identity := models.Identity{
			ID:    userId,
			Label: login,
			User:  user,
		}
		identities = append(identities, identity)
	}

	response := models.SynchronizeUsersResponse{
		Identities: identities,
	}

	if resp.NextPage != 0 {
		response.Pagination = &models.PaginationOptions{
			Page:     resp.NextPage,
			PageSize: req.Pagination.PageSize,
		}
	}

	logrus.WithFields(logrus.Fields{
		"count": len(identities),
		"org":   p.organizationName,
	}).Debug("Refreshed GitHub identities")

	return &response, nil
}
