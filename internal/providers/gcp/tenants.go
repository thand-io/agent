package gcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"google.golang.org/api/cloudresourcemanager/v3"
)

// gcpPaginationToken holds pagination state for both projects and folders
type gcpPaginationToken struct {
	ProjectToken string `json:"project_token,omitempty"`
	FolderToken  string `json:"folder_token,omitempty"`
}

// encodeGCPToken serializes the composite token to a base64 string
func encodeGCPToken(token gcpPaginationToken) string {
	if token.ProjectToken == "" && token.FolderToken == "" {
		return ""
	}
	data, err := json.Marshal(token)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}

// decodeGCPToken deserializes a base64 token string back to the composite token
func decodeGCPToken(tokenStr string) gcpPaginationToken {
	if tokenStr == "" {
		return gcpPaginationToken{}
	}
	data, err := base64.StdEncoding.DecodeString(tokenStr)
	if err != nil {
		return gcpPaginationToken{}
	}
	var token gcpPaginationToken
	if err := json.Unmarshal(data, &token); err != nil {
		return gcpPaginationToken{}
	}
	return token
}

func (p *gcpProvider) SynchronizeTenants(ctx context.Context, req *models.SynchronizeTenantsRequest) (*models.SynchronizeTenantsResponse, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Refreshed GCP tenants (projects and folders) in %s", elapsed)
	}()

	// Check if crmClient is available
	if p.crmClient == nil {
		logrus.Warn("Cloud Resource Manager client not initialized, skipping tenant synchronization")
		return &models.SynchronizeTenantsResponse{}, nil
	}

	// Create a v3 client for listing projects and folders
	crmV3Service, err := cloudresourcemanager.NewService(ctx, p.client.ClientOptions...)
	if err != nil {
		logrus.WithError(err).Warn("Failed to create Cloud Resource Manager v3 client")
		return &models.SynchronizeTenantsResponse{}, nil
	}

	// Set pagination options
	if req.Pagination == nil {
		req.Pagination = &models.PaginationOptions{
			PageSize: 10,
		}
	}

	// Decode the composite token to get separate project and folder tokens
	compositeToken := decodeGCPToken(req.Pagination.Token)

	var tenants []models.ProviderTenant

	// Create pagination options with the project-specific token
	projectPagination := &models.PaginationOptions{
		PageSize: req.Pagination.PageSize,
		Token:    compositeToken.ProjectToken,
	}

	// First, try to list projects
	projects, nextToken, err := p.listProjects(ctx, crmV3Service, projectPagination)
	if err != nil {
		logrus.WithError(err).Warn("Failed to list projects, continuing with folders if available")
	} else {
		tenants = append(tenants, projects...)
	}

	// Create pagination options with the folder-specific token
	folderPagination := &models.PaginationOptions{
		PageSize: req.Pagination.PageSize,
		Token:    compositeToken.FolderToken,
	}

	// If we have permission, also try to list folders
	// This is useful for organizations that use folder hierarchy
	folders, folderToken, err := p.listFolders(ctx, crmV3Service, folderPagination)
	if err != nil {
		logrus.WithError(err).Debug("Failed to list folders (this is expected if not in an organization)")
	} else {
		tenants = append(tenants, folders...)
	}

	response := &models.SynchronizeTenantsResponse{
		Tenants: tenants,
	}

	// Create a composite token if either projects or folders have more pages
	if nextToken != "" || folderToken != "" {
		newCompositeToken := gcpPaginationToken{
			ProjectToken: nextToken,
			FolderToken:  folderToken,
		}
		response.Pagination = &models.PaginationOptions{
			Token:    encodeGCPToken(newCompositeToken),
			PageSize: req.Pagination.PageSize,
		}
	}

	logrus.WithFields(logrus.Fields{
		"count": len(tenants),
	}).Debug("Refreshed GCP tenants (projects and folders)")

	return response, nil
}

func (p *gcpProvider) listProjects(ctx context.Context, crmV3Service *cloudresourcemanager.Service, pagination *models.PaginationOptions) ([]models.ProviderTenant, string, error) {
	var tenants []models.ProviderTenant

	projectsListCall := crmV3Service.Projects.List()
	projectsListCall.PageSize(int64(pagination.PageSize))

	if len(pagination.Token) != 0 {
		projectsListCall.PageToken(pagination.Token)
	}

	// Search for projects in the ACTIVE state
	projectsListCall.ShowDeleted(false)

	projectsResp, err := projectsListCall.Do()
	if err != nil {
		return nil, "", fmt.Errorf("failed to list projects: %w", err)
	}

	for _, project := range projectsResp.Projects {
		// Skip projects that are not ACTIVE
		if project.State != "ACTIVE" {
			continue
		}

		// Use project ID as the identifier
		projectId := project.ProjectId
		projectName := project.DisplayName

		if projectName == "" {
			projectName = projectId
		}

		tenant := models.ProviderTenant{
			ID:     projectId,
			Type:   "project",
			Parent: project.Parent,
			Name:   projectName,
			Tenant: project,
		}
		tenants = append(tenants, tenant)
	}

	return tenants, projectsResp.NextPageToken, nil
}

func (p *gcpProvider) listFolders(ctx context.Context, crmV3Service *cloudresourcemanager.Service, pagination *models.PaginationOptions) ([]models.ProviderTenant, string, error) {
	var tenants []models.ProviderTenant

	// List folders - we need to search under a parent (organization or folder)
	// If no parent is specified, we'll try to list all accessible folders
	foldersSearchCall := crmV3Service.Folders.Search()
	foldersSearchCall.PageSize(int64(pagination.PageSize))

	if len(pagination.Token) != 0 {
		foldersSearchCall.PageToken(pagination.Token)
	}

	// Search for folders in the ACTIVE state
	foldersSearchCall.Query("state:ACTIVE")

	foldersResp, err := foldersSearchCall.Do()
	if err != nil {
		return nil, "", fmt.Errorf("failed to list folders: %w", err)
	}

	for _, folder := range foldersResp.Folders {
		// Skip folders that are not ACTIVE
		if folder.State != "ACTIVE" {
			continue
		}

		// Extract folder ID from the name (format: folders/123456789)
		folderId := folder.Name
		folderDisplayName := folder.DisplayName

		if folderDisplayName == "" {
			folderDisplayName = folderId
		}

		tenant := models.ProviderTenant{
			ID:     folderId,
			Type:   "folder",
			Parent: folder.Parent,
			Name:   folderDisplayName,
			Tenant: folder,
		}
		tenants = append(tenants, tenant)
	}

	return tenants, foldersResp.NextPageToken, nil
}
