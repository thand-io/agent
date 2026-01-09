package azure

import (
	"context"
	"fmt"
	"time"

	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
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

	// Create Microsoft Graph client
	graphClient, err := msgraphsdk.NewGraphServiceClientWithCredentials(p.cred.Token, []string{"https://graph.microsoft.com/.default"})
	if err != nil {
		if req.Pagination == nil {
			// This is an initial request. If we've failed to get any groups,
			// this is probably a permission error.
			return nil, temporal.NewNonRetryableApplicationError(
				"Failed to create Microsoft Graph client",
				"GraphClientRequest",
				err,
			)
		}
		return nil, fmt.Errorf("failed to create Microsoft Graph client: %w", err)
	}

	if req.Pagination == nil {
		req.Pagination = &models.PaginationOptions{
			PageSize: 100,
		}
	}

	// Build request configuration
	requestConfig := &groups.GroupsRequestBuilderGetRequestConfiguration{
		QueryParameters: &groups.GroupsRequestBuilderGetQueryParameters{
			Top:    int32Ptr(int32(req.Pagination.PageSize)),
			Select: []string{"id", "displayName", "mail", "description", "groupTypes"},
		},
	}

	var groupsList []graphmodels.Groupable
	var nextLink *string

	result, err := graphClient.Groups().Get(ctx, requestConfig)
	if err != nil {
		if req.Pagination == nil || len(req.Pagination.Token) == 0 {
			// This is an initial request
			return nil, temporal.NewNonRetryableApplicationError(
				"Failed to list groups from Azure AD",
				"GraphGroupsRequest",
				err,
			)
		}
		return nil, fmt.Errorf("failed to list groups: %w", err)
	}
	groupsList = result.GetValue()
	nextLink = result.GetOdataNextLink()

	var identities []models.Identity
	for _, group := range groupsList {
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

	// Handle pagination
	if nextLink != nil && len(*nextLink) > 0 {
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
