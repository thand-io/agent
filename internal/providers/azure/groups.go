package azure

import (
	"context"
	"fmt"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/groups"
	graphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/temporal"
)

// SynchronizeGroups fetches groups from Azure AD via Microsoft Graph API
func (p *azureProvider) SynchronizeGroups(ctx context.Context, req *models.SynchronizeGroupsRequest) (*models.SynchronizeGroupsResponse, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Refreshed Azure AD groups in %s", elapsed)
	}()

	if req.Pagination == nil {
		req.Pagination = &models.PaginationOptions{
			PageSize: 100,
		}
	}

	// Fetch one page of results. When a continuation token (nextLink URL) from a
	// previous call is present, resume directly from that URL so we don't
	// re-fetch the first page on every call.
	var groupsResult interface {
		GetValue() []graphmodels.Groupable
		GetOdataNextLink() *string
	}
	var fetchErr error

	if len(req.Pagination.Token) > 0 {
		// Resume from the nextLink URL returned by the previous page.
		groupsResult, fetchErr = p.graphClient.Groups().WithUrl(req.Pagination.Token).Get(ctx, nil)
	} else {
		requestConfig := &groups.GroupsRequestBuilderGetRequestConfiguration{
			QueryParameters: &groups.GroupsRequestBuilderGetQueryParameters{
				Top:    int32Ptr(int32(req.Pagination.PageSize)),
				Select: []string{"id", "displayName", "mail", "description", "groupTypes"},
			},
		}
		groupsResult, fetchErr = p.graphClient.Groups().Get(ctx, requestConfig)
	}

	if fetchErr != nil {
		if len(req.Pagination.Token) == 0 {
			// Initial request failure is likely a permanent permission issue.
			return nil, temporal.NewNonRetryableApplicationError(
				"Failed to list groups from Azure AD",
				"GraphGroupsRequest",
				fetchErr,
			)
		}
		return nil, fmt.Errorf("failed to list groups: %w", fetchErr)
	}

	var identities []models.Identity
	for _, group := range groupsResult.GetValue() {
		if group == nil {
			continue
		}

		var groupId string
		var displayName string
		var email string

		if group.GetId() != nil && len(*group.GetId()) > 0 {
			groupId = *group.GetId()
		} else {
			continue
		}

		if group.GetDisplayName() != nil && len(*group.GetDisplayName()) > 0 {
			displayName = *group.GetDisplayName()
		}

		if group.GetMail() != nil && len(*group.GetMail()) > 0 {
			email = *group.GetMail()
		}

		identity := models.Identity{
			ID:     groupId,
			Label:  displayName,
			Tenant: req.Tenant,
			Group: &models.Group{
				ID:    groupId,
				Name:  displayName,
				Email: email,
			},
		}
		identities = append(identities, identity)
	}

	response := &models.SynchronizeGroupsResponse{
		Identities: identities,
	}

	// Handle pagination — store the nextLink URL as the token for the next call.
	if nextLink := groupsResult.GetOdataNextLink(); nextLink != nil && len(*nextLink) > 0 {
		response.Pagination = &models.PaginationOptions{
			Token:    *nextLink,
			PageSize: req.Pagination.PageSize,
		}
	}

	logrus.WithFields(logrus.Fields{
		"count": len(identities),
	}).Debug("Refreshed Azure AD groups")

	return response, nil
}
