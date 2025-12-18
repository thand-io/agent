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
func (p *githubProvider) SynchronizeGroups(ctx context.Context, req *models.SynchronizeGroupsRequest) (*models.SynchronizeGroupsResponse, error) {
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
	opts := &github.ListOptions{
		Page:    req.Pagination.Page,
		PerPage: req.Pagination.PageSize,
	}

	teams, resp, err := p.client.Teams.ListTeams(ctx, p.organizationName, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch organization teams: %w", err)
	}

	for _, team := range teams {
		if team == nil {
			continue
		}

		name := team.GetName()
		slug := team.GetSlug()
		id := team.GetID()

		if len(name) == 0 || id == 0 {
			continue
		}

		teamId := fmt.Sprintf("%d", id)

		group := &models.Group{
			ID:   teamId,
			Name: name,
		}

		// Use slug as label if available, else name
		label := name
		if len(slug) != 0 {
			label = slug
		}

		identity := models.Identity{
			ID:    teamId,
			Label: label,
			Group: group,
		}
		identities = append(identities, identity)
	}

	response := models.SynchronizeGroupsResponse{
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
