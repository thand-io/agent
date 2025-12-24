package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/identitystore"
	"github.com/aws/aws-sdk-go-v2/service/ssoadmin"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/temporal"
)

func (p *awsProvider) SynchronizeGroups(ctx context.Context, req *models.SynchronizeGroupsRequest) (*models.SynchronizeGroupsResponse, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Refreshed AWS group identities in %s", elapsed)
	}()

	// 1. Get Identity Store ID
	resp, err := p.ssoAdminService.ListInstances(ctx, &ssoadmin.ListInstancesInput{})
	if err != nil {

		if req.Pagination == nil {

			// This is an initial request. If we've failed to get any groups,
			// this is probably a permission error.

			return nil, temporal.NewNonRetryableApplicationError(
				"Failed to list identity center instances",
				"IdentityCenterRequest",
				err,
			)
		}

		return nil, fmt.Errorf("failed to list SSO instances: %w", err)
	}

	if len(resp.Instances) == 0 {
		logrus.Warn("No SSO instances found, skipping group synchronization")
		return &models.SynchronizeGroupsResponse{}, nil
	}

	identityStoreId := resp.Instances[0].IdentityStoreId
	if identityStoreId == nil {
		return nil, temporal.NewNonRetryableApplicationError(
			"identity store ID not found in SSO instance",
			"IdentityCenterRequest",
			fmt.Errorf("identity store ID is nil"),
		)
	}

	if req.Pagination == nil {
		req.Pagination = &models.PaginationOptions{
			PageSize: 100,
		}
	}

	input := &identitystore.ListGroupsInput{
		IdentityStoreId: identityStoreId,
		MaxResults:      aws.Int32(int32(req.Pagination.PageSize)),
	}

	if len(req.Pagination.Token) != 0 {
		input.NextToken = aws.String(req.Pagination.Token)
	}

	groupsResp, err := p.identityStoreClient.ListGroups(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list groups: %w", err)
	}

	var identities []models.Identity
	for _, group := range groupsResp.Groups {

		var groupId string
		var displayName string

		if group.GroupId != nil && len(*group.GroupId) > 0 {
			groupId = *group.GroupId
		} else {
			continue
		}

		if group.DisplayName != nil && len(*group.DisplayName) > 0 {
			displayName = *group.DisplayName
		}

		identity := models.Identity{
			ID:     groupId,
			Label:  displayName,
			Tenant: req.Tenant,
			Group: &models.Group{
				ID:   groupId,
				Name: displayName,
			},
		}
		identities = append(identities, identity)
	}

	response := &models.SynchronizeGroupsResponse{
		Identities: identities,
	}

	if groupsResp.NextToken != nil {
		response.Pagination = &models.PaginationOptions{
			Token:    *groupsResp.NextToken,
			PageSize: req.Pagination.PageSize,
		}
	}

	logrus.WithFields(logrus.Fields{
		"count": len(identities),
	}).Debug("Refreshed AWS group identities")

	return response, nil
}
