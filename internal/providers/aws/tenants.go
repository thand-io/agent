package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	"github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

func (p *awsProvider) SynchronizeTenants(ctx context.Context, req *models.SynchronizeTenantsRequest) (*models.SynchronizeTenantsResponse, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Refreshed AWS tenants (accounts) in %s", elapsed)
	}()

	// Check if organizationsClient is available
	if p.organizationsClient == nil {
		logrus.Warn("Organizations client not initialized, skipping tenant synchronization")
		return &models.SynchronizeTenantsResponse{}, nil
	}

	// First, check if we have access to AWS Organizations by attempting to describe the organization
	_, err := p.organizationsClient.DescribeOrganization(ctx, &organizations.DescribeOrganizationInput{})
	if err != nil {
		// If this is the initial request and we get an error, it's likely a permission issue
		if req.Pagination == nil {
			logrus.WithError(err).Warn("Unable to access AWS Organizations - provider may not have necessary permissions or not be part of an organization")
			// Return empty response rather than error to allow provider to initialize
			return &models.SynchronizeTenantsResponse{}, nil
		}
		return nil, fmt.Errorf("failed to describe organization: %w", err)
	}

	if req.Pagination == nil {
		req.Pagination = &models.PaginationOptions{
			PageSize: 10,
		}
	}

	input := &organizations.ListAccountsInput{
		MaxResults: aws.Int32(int32(req.Pagination.PageSize)),
	}

	if len(req.Pagination.Token) != 0 {
		input.NextToken = aws.String(req.Pagination.Token)
	}

	accountsResp, err := p.organizationsClient.ListAccounts(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}

	var tenants []models.ProviderTenant
	for _, account := range accountsResp.Accounts {
		// Skip accounts that are not ACTIVE
		if account.Status != types.AccountStatusActive {
			continue
		}

		var accountId string
		var accountName string

		if account.Id != nil && len(*account.Id) > 0 {
			accountId = *account.Id
		} else {
			continue
		}

		if account.Name != nil && len(*account.Name) > 0 {
			accountName = *account.Name
		} else {
			// Use account ID as name if name is not available
			accountName = accountId
		}

		tenant := models.ProviderTenant{
			ID:   accountId,
			Name: accountName,
		}
		tenants = append(tenants, tenant)
	}

	response := &models.SynchronizeTenantsResponse{
		Tenants: tenants,
	}

	if accountsResp.NextToken != nil {
		response.Pagination = &models.PaginationOptions{
			Token:    *accountsResp.NextToken,
			PageSize: req.Pagination.PageSize,
		}
	}

	logrus.WithFields(logrus.Fields{
		"count": len(tenants),
	}).Debug("Refreshed AWS tenants (accounts)")

	return response, nil
}
