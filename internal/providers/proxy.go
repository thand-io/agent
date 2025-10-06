package providers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

type remoteProviderProxy struct {
	*models.BaseProvider
	providerKey string
	client      *resty.Client
}

func NewRemoteProviderProxy(providerKey, endpoint string) models.ProviderImpl {

	logrus.Debugf("Creating new remote provider proxy: %s", endpoint)

	return &remoteProviderProxy{
		providerKey: providerKey,
		client:      resty.New().SetBaseURL(endpoint),
	}
}

func (p *remoteProviderProxy) Initialize(provider models.Provider) error {

	p.BaseProvider = models.NewBaseProvider(
		provider,
		models.ProviderCapabilityRBAC,
	)

	return nil

}

func (p *remoteProviderProxy) AuthorizeSession(ctx context.Context, user *models.AuthorizeUser) (*models.AuthorizeSessionResponse, error) {

	url := fmt.Sprintf("/provider/%s/authorizeSession", p.providerKey)

	// Make post request with the user to the providers api
	resp, err := p.client.R().
		SetContext(ctx).
		SetBody(user).
		Post(url)

	logrus.WithFields(logrus.Fields{
		"provider": p.providerKey,
		"url":      url,
	}).Debugln("Sending authorization request")

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() == 404 {
		return nil, fmt.Errorf("provider %s does not exist", p.providerKey)
	}

	if resp.IsError() {
		logrus.WithFields(logrus.Fields{
			"provider": p.GetName(),
			"body":     string(resp.Body()),
		}).Error("Failed to authorize session")
		return nil, fmt.Errorf("failed to authorize session: %s", resp.Error())
	}

	var authResponse models.AuthorizeSessionResponse
	if err := json.Unmarshal(resp.Body(), &authResponse); err != nil {
		return nil, err
	}

	return &authResponse, nil
}
