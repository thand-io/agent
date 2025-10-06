package salesforce

import (
	"fmt"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/simpleforce/simpleforce"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
)

// salesForceProvider implements the ProviderImpl interface for Salesforce
type salesForceProvider struct {
	*models.BaseProvider
	client     *simpleforce.Client
	roles      []models.ProviderRole
	rolesIndex bleve.Index
}

func (p *salesForceProvider) Initialize(provider models.Provider) error {
	p.BaseProvider = models.NewBaseProvider(
		provider,
		models.ProviderCapabilityRBAC,
		models.ProviderCapabilityAuthorizor,
	)

	salesForceConfig := p.GetConfig()

	sdkConfig, err := CreateSalesforceClient(salesForceConfig)

	if err != nil {
		return fmt.Errorf("failed to create Salesforce config: %w", err)
	}

	p.client = sdkConfig

	// Cool lets query the avalible roles
	foundRoles, err := p.LoadRoles()

	if err != nil {
		return fmt.Errorf("failed to load roles: %w", err)
	}

	p.roles = foundRoles

	return nil
}

/*
https://developer.salesforce.com/docs/atlas.en-us.api_rest.meta/api_rest/dome_query.htm
*/
func (p *salesForceProvider) queryWithParams(query string, args ...any) (*simpleforce.QueryResult, error) {

	rawQuery, err := common.QueryWithParams(query, args...)

	if err != nil {
		return nil, fmt.Errorf("failed to construct query with params: %w", err)
	}

	// Before we run our rawQuery salesforce uses SOQL not SQL so we need to
	// replace the double quotes with single quotes

	rawQuery = strings.ReplaceAll(rawQuery, "\"", "'")

	// Execute the Salesforce query (assuming query is SOQL, not SQL)
	result, err := p.client.Query(rawQuery) // Note: for Tooling API, use client.Tooling().Query(q)

	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
			"query": query,
		}).Error("Failed to execute Salesforce query")
		return nil, err
	}

	return result, nil
}

func CreateSalesforceClient(salesForceConfig *models.BasicConfig) (*simpleforce.Client, error) {

	endpoint, foundEndpoint := salesForceConfig.GetString("endpoint")

	if !foundEndpoint {
		endpoint = "https://login.salesforce.com"
	}

	clientId, foundClientId := salesForceConfig.GetString("client_id")

	if !foundClientId {
		// handle the error
		clientId = simpleforce.DefaultClientID
	}

	username, foundUsername := salesForceConfig.GetString("username")

	if !foundUsername {
		// handle the error
		return nil, fmt.Errorf("username not found in config")
	}

	password, foundPassword := salesForceConfig.GetString("password")

	if !foundPassword {
		// handle the error
		return nil, fmt.Errorf("password not found in config")
	}

	token, foundToken := salesForceConfig.GetString("token")

	if !foundToken {
		// handle the error
		return nil, fmt.Errorf("token not found in config")
	}

	client := simpleforce.NewClient(
		endpoint, clientId, simpleforce.DefaultAPIVersion)
	if client == nil {
		// handle the error

		return nil, fmt.Errorf("failed to create Salesforce client")
	}

	err := client.LoginPassword(username, password, token)
	if err != nil {
		// handle the error

		return nil, fmt.Errorf("failed to login to Salesforce: %w", err)
	}

	// Do some other stuff with the client instance if needed.

	return client, nil
}

func init() {
	providers.Register("salesforce", &salesForceProvider{})
}
