package azure

import (
	"context"
	"fmt"
	"time"

	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	graphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/temporal"
)

// SynchronizeUsers fetches users from Azure AD via Microsoft Graph API
func (p *azureProvider) SynchronizeUsers(ctx context.Context, req *models.SynchronizeUsersRequest) (*models.SynchronizeUsersResponse, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Refreshed Azure AD users in %s", elapsed)
	}()

	// Create Microsoft Graph client
	graphClient, err := msgraphsdk.NewGraphServiceClientWithCredentials(p.cred.Token, []string{"https://graph.microsoft.com/.default"})
	if err != nil {
		if req.Pagination == nil {
			// This is an initial request. If we've failed to get any users,
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
	requestConfig := &users.UsersRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.UsersRequestBuilderGetQueryParameters{
			Top:    int32Ptr(int32(req.Pagination.PageSize)),
			Select: []string{"id", "displayName", "mail", "userPrincipalName", "givenName", "surname"},
		},
	}

	var usersList []graphmodels.Userable
	var nextLink *string

	usersResult, err := graphClient.Users().Get(ctx, requestConfig)
	if err != nil {
		if req.Pagination == nil || len(req.Pagination.Token) == 0 {
			// This is an initial request
			return nil, temporal.NewNonRetryableApplicationError(
				"Failed to list users from Azure AD",
				"GraphUsersRequest",
				err,
			)
		}
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	usersList = usersResult.GetValue()
	nextLink = usersResult.GetOdataNextLink()

	var identities []models.Identity
	for _, user := range usersList {
		if user == nil {
			continue
		}

		var userId string
		var displayName string
		var email string
		var username string

		if user.GetId() != nil && len(*user.GetId()) > 0 {
			userId = *user.GetId()
		} else {
			continue
		}

		if user.GetDisplayName() != nil && len(*user.GetDisplayName()) > 0 {
			displayName = *user.GetDisplayName()
		}

		if user.GetMail() != nil && len(*user.GetMail()) > 0 {
			email = *user.GetMail()
		} else if user.GetUserPrincipalName() != nil && len(*user.GetUserPrincipalName()) > 0 {
			// Use UPN as email fallback
			email = *user.GetUserPrincipalName()
		}

		if user.GetUserPrincipalName() != nil && len(*user.GetUserPrincipalName()) > 0 {
			username = common.ExtractUsernameFromEmail(*user.GetUserPrincipalName())
		}

		if len(displayName) == 0 {
			displayName = common.ExtractNameFromEmail(email)
		}

		identity := models.Identity{
			ID:     userId,
			Label:  displayName,
			Tenant: req.Tenant,
			User: &models.User{
				ID:       userId,
				Username: username,
				Email:    email,
				Name:     displayName,
				Source:   "azure-ad",
			},
		}
		identities = append(identities, identity)
	}

	response := &models.SynchronizeUsersResponse{
		Identities: identities,
	}

	// Handle pagination
	if nextLink != nil && len(*nextLink) > 0 {
		// Extract skiptoken from the next link for use in subsequent requests
		response.Pagination = &models.PaginationOptions{
			Token:    *nextLink,
			PageSize: req.Pagination.PageSize,
		}
	}

	logrus.WithFields(logrus.Fields{
		"count": len(identities),
	}).Debug("Refreshed Azure AD users")

	return response, nil
}

func int32Ptr(i int32) *int32 {
	return &i
}
