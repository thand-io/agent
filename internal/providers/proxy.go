package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

const ProviderProxySessionKey = "session"

type remoteProviderProxy struct {
	*models.BaseProvider
	providerKey string
	endpoint    string
}

func NewRemoteProviderProxy(providerKey, endpoint string) models.Provider {

	logrus.Debugf("Creating new remote provider proxy: %s/provider/%s", endpoint, providerKey)

	return &remoteProviderProxy{
		providerKey: providerKey,
		endpoint:    endpoint,
	}
}

func (p *remoteProviderProxy) Initialize(identifier string, provider models.ProviderConfig) error {

	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		models.NewProviderCapabilities().
			WithRolesConfiguration(models.RolesConfiguration{
				Enabled:        true,
				Synchronizable: false,
			}).
			WithPermissionsConfiguration(models.PermissionsConfiguration{
				Enabled:        true,
				Synchronizable: false,
			}),
	)

	return nil

}

func (p *remoteProviderProxy) AuthorizeSession(ctx context.Context, user *models.AuthorizeUser) (*models.AuthorizeSessionResponse, error) {

	var resp models.AuthorizeSessionResponse

	err := p.proxyRequest(
		ctx,
		fmt.Sprintf("%s/provider/%s/authorizeSession", p.endpoint, p.providerKey),
		http.MethodPost,
		user,
		&resp,
	)

	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func (p *remoteProviderProxy) ListIdentities(ctx context.Context, searchRequest *models.SearchRequest) ([]models.SearchResult[models.Identity], error) {

	var resp []models.SearchResult[models.Identity]

	err := p.proxyRequest(
		ctx,
		fmt.Sprintf("%s/provider/%s/listIdentities", p.endpoint, p.providerKey),
		http.MethodPost,
		searchRequest,
		&resp,
	)

	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (p *remoteProviderProxy) ListTenants(
	ctx context.Context,
	searchRequest *models.SearchRequest,
) ([]models.SearchResult[models.ProviderTenant], error) {

	var resp []models.SearchResult[models.ProviderTenant]

	err := p.proxyRequest(
		ctx,
		fmt.Sprintf("%s/provider/%s/listTenants", p.endpoint, p.providerKey),
		http.MethodPost,
		searchRequest,
		&resp,
	)

	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (p *remoteProviderProxy) proxyRequest(
	ctx context.Context,
	url string,
	method string,
	body any,
	response any) error {

	bodyBytes, err := json.Marshal(body)

	if err != nil {
		return err
	}

	if ctx == nil {
		return fmt.Errorf("context is nil")
	}

	sessionKey, ok := ctx.Value(ProviderProxySessionKey).(*models.LocalSession)

	if !ok || sessionKey == nil {
		return fmt.Errorf("no session key found in context for provider proxy")
	}

	endpoint := &model.Endpoint{
		EndpointConfig: &model.EndpointConfiguration{
			URI: &model.LiteralUri{
				Value: url,
			},
			Authentication: &model.ReferenceableAuthenticationPolicy{
				AuthenticationPolicy: &model.AuthenticationPolicy{
					Bearer: &model.BearerAuthenticationPolicy{
						Token: sessionKey.GetEncodedLocalSession(),
					},
				},
			},
		},
	}

	req := &model.HTTPArguments{
		Method:   method,
		Endpoint: endpoint,
		Body:     bodyBytes,
		Headers:  map[string]string{"Content-Type": "application/json"},
	}

	// Make post request with the user to the providers api
	resp, err := common.InvokeHttpRequest(req)

	logrus.WithFields(logrus.Fields{
		"provider": p.providerKey,
		"url":      req.Endpoint.String(),
	}).Debugln("Sending authorization request")

	if err != nil {
		return err
	}

	if resp.StatusCode() == http.StatusNotFound {
		return fmt.Errorf("provider %s does not exist", p.providerKey)
	}

	if resp.IsError() {
		logrus.WithFields(logrus.Fields{
			"provider": p.GetName(),
		}).Error("Failed to authorize session")
		return fmt.Errorf("failed to authorize session: %s", resp.Error())
	}

	if err := json.Unmarshal(resp.Body(), &response); err != nil {
		return err
	}

	return nil
}
