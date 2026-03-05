package gcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/data"
	"github.com/thand-io/agent/internal/models"
)

// SynchronizeRoles fetches roles from the GCP IAM API for the configured project
func (p *gcpProvider) SynchronizeRoles(ctx context.Context, req *models.SynchronizeRolesRequest) (*models.SynchronizeRolesResponse, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Refreshed GCP roles in %s", elapsed)
	}()

	if p.iamClient == nil {
		return nil, fmt.Errorf("GCP IAM client is not initialized")
	}

	projectId := p.GetProjectId()
	if len(projectId) == 0 {
		return nil, fmt.Errorf("GCP project ID is not configured")
	}

	if req.Pagination == nil {
		req.Pagination = &models.PaginationOptions{
			PageSize: 100,
		}
	}

	pageSize := req.Pagination.PageSize
	if pageSize == 0 {
		pageSize = 100
	}

	listCall := p.iamClient.Projects.Roles.List("projects/" + projectId).
		Context(ctx).
		PageSize(int64(pageSize)).
		View("BASIC")

	if req.Pagination.Token != "" {
		listCall = listCall.PageToken(req.Pagination.Token)
	}

	resp, err := listCall.Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list GCP project roles: %w", err)
	}

	providerRoles := make([]models.ProviderRole, 0, len(resp.Roles))
	for _, role := range resp.Roles {
		providerRoles = append(providerRoles, models.ProviderRole{
			ID:          role.Name,
			Name:        role.Name,
			Title:       role.Title,
			Description: role.Description,
			Role:        role,
		})
	}

	logrus.WithFields(logrus.Fields{
		"roles": len(providerRoles),
	}).Debug("Refreshed GCP roles")

	return &models.SynchronizeRolesResponse{
		Pagination: &models.PaginationOptions{
			PageSize: pageSize,
			Token:    resp.NextPageToken,
		},
		Roles: providerRoles,
	}, nil
}

func loadRoles(stage string) ([]models.ProviderRole, error) {

	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Parsed GCP roles in %s", elapsed)
	}()

	// Get pre-parsed GCP roles from data package
	predefinedRoles, err := data.GetParsedGcpRoles()
	if err != nil {
		return nil, fmt.Errorf("failed to get parsed GCP roles: %w", err)
	}

	var roles = make([]models.ProviderRole, 0, len(predefinedRoles))

	if len(stage) == 0 {
		stage = DefaultStage
	}

	for _, gcpRole := range predefinedRoles {

		if !strings.EqualFold(gcpRole.Stage, stage) {
			continue
		}

		role := models.ProviderRole{
			Name:        gcpRole.Name,
			Title:       gcpRole.Title,
			Description: gcpRole.Description,
		}
		roles = append(roles, role)
	}

	return roles, nil
}
