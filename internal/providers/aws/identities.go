package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/temporal"
)

func (p *awsProvider) SynchronizeIdentities(ctx context.Context, req *models.SynchronizeIdentitiesRequest) (*models.SynchronizeIdentitiesResponse, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Refreshed AWS IAM users in %s", elapsed)
	}()

	input := &iam.ListUsersInput{}

	if req.Pagination == nil {

		// No page request. This is the start of our sync. Check we have access first.
		// If we don't then cancel the request.

		maxItems := int32(100)
		input.MaxItems = &maxItems

	} else {

		if len(req.Pagination.Token) != 0 {
			input.Marker = &req.Pagination.Token
		}
		if req.Pagination.PageSize > 0 {
			maxItems := int32(req.Pagination.PageSize)
			input.MaxItems = &maxItems
		}
	}

	output, err := p.service.ListUsers(ctx, input)

	if err != nil {

		if req.Pagination == nil {

			// This is an initial request. If we've failed to get any identities,
			// this is probably a permission error.

			return nil, temporal.NewNonRetryableApplicationError(
				"Failed to list IAM users",
				"IamUsersRequest",
				err,
			)
		}

		return nil, fmt.Errorf("failed to list IAM users: %w", err)
	}

	var identities []models.Identity
	for _, user := range output.Users {
		var userId string
		var userName string

		if user.Arn != nil && len(*user.Arn) > 0 {
			userId = *user.Arn
		} else if user.UserId != nil && len(*user.UserId) > 0 {
			userId = *user.UserId
		} else {
			continue
		}

		if user.UserName != nil && len(*user.UserName) > 0 {
			userName = *user.UserName
		}

		// IAM users don't have emails
		identity := models.Identity{
			ID:     userId,
			Label:  userName,
			Tenant: req.Tenant,
			User: &models.User{
				ID:       userId,
				Username: userName,
				Name:     userName,
				Source:   "iam",
			},
		}
		identities = append(identities, identity)
	}

	response := &models.SynchronizeIdentitiesResponse{
		Identities: identities,
	}

	if output.IsTruncated && output.Marker != nil {
		response.Pagination = &models.PaginationOptions{
			Token:    *output.Marker,
			PageSize: req.Pagination.PageSize,
		}
	}

	return response, nil
}
