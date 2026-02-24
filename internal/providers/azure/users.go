package azure

import (
	"context"
	"fmt"
	"time"

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

	if req.Pagination == nil {
		req.Pagination = &models.PaginationOptions{
			PageSize: 100,
		}
	}

	// Fetch one page of results. When a continuation token (nextLink URL) from a
	// previous call is present, resume directly from that URL so we don't
	// re-fetch the first page on every call.
	var usersResult interface {
		GetValue() []graphmodels.Userable
		GetOdataNextLink() *string
	}
	var fetchErr error

	if len(req.Pagination.Token) > 0 {
		// Resume from the nextLink URL returned by the previous page.
		usersResult, fetchErr = p.graphClient.Users().WithUrl(req.Pagination.Token).Get(ctx, nil)
	} else {
		requestConfig := &users.UsersRequestBuilderGetRequestConfiguration{
			QueryParameters: &users.UsersRequestBuilderGetQueryParameters{
				Top:    int32Ptr(int32(req.Pagination.PageSize)),
				Select: []string{"id", "displayName", "mail", "userPrincipalName", "givenName", "surname"},
			},
		}
		usersResult, fetchErr = p.graphClient.Users().Get(ctx, requestConfig)
	}

	if fetchErr != nil {
		if len(req.Pagination.Token) == 0 {
			// Initial request failure is likely a permanent permission issue.
			return nil, temporal.NewNonRetryableApplicationError(
				"Failed to list users from Azure AD",
				"GraphUsersRequest",
				fetchErr,
			)
		}
		return nil, fmt.Errorf("failed to list users: %w", fetchErr)
	}

	var identities []models.Identity
	for _, user := range usersResult.GetValue() {
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

	// Handle pagination — store the nextLink URL as the token for the next call.
	if nextLink := usersResult.GetOdataNextLink(); nextLink != nil && len(*nextLink) > 0 {
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
