package thand

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
)

const DefaultThandAuthEndpoint = "https://auth.thand.io"

const DefaultThandUserInfoPath = "/userinfo"

var ThandProviderName = "thand"

// thandProvider implements the ProviderImpl interface for Thand federated OIDC authentication
type thandProvider struct {
	*models.BaseProvider
	authEndpoint string
}

// UserInfoResponse represents the user information returned from thand.io
type UserInfoResponse struct {
	Sub               string   `json:"sub"`            // Subject - unique user ID
	Email             string   `json:"email"`          // User's email
	EmailVerified     bool     `json:"email_verified"` // Whether email is verified
	Name              string   `json:"name"`           // Full name
	PreferredUsername string   `json:"preferred_username,omitempty"`
	Groups            []string `json:"groups,omitempty"` // User groups/roles
}

func (p *thandProvider) Initialize(identifier string, provider models.ProviderConfig) error {
	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		ThandCapabilities,
	)

	thandConfig := p.GetConfig()

	// Get endpoints from config with defaults
	p.authEndpoint = thandConfig.GetStringWithDefault("endpoint", DefaultThandAuthEndpoint)

	return nil
}

func (p *thandProvider) AuthorizeSession(ctx context.Context, authRequest *models.AuthorizeUser) (*models.AuthorizeSessionResponse, error) {
	// Build the OAuth2 authorization URL

	queryParams := url.Values{
		"state": {authRequest.State},
	}

	returnUrl, err := url.Parse(authRequest.RedirectUri)

	if err != nil {
		return nil, fmt.Errorf("invalid redirect URI: %w", err)
	}

	returnUrl.RawQuery = queryParams.Encode()

	authUrl, err := url.Parse(p.authEndpoint)

	if err != nil {
		return nil, fmt.Errorf("invalid auth endpoint URL: %w", err)
	}

	returnParams := url.Values{
		"redirect_uri": {returnUrl.String()},
	}

	authUrl.RawQuery = returnParams.Encode()

	return &models.AuthorizeSessionResponse{Url: authUrl.String()}, nil
}

func (p *thandProvider) CreateSession(ctx context.Context, authRequest *models.AuthorizeUser) (*models.Session, error) {

	if len(authRequest.Code) == 0 {
		return nil, fmt.Errorf("authorization code is required")
	}

	userInfo, err := p.getUserInfo(ctx, authRequest.Code)

	if err != nil {
		logrus.WithError(err).Errorln("failed to get user info")
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	user := &models.User{
		ID:       userInfo.Sub,
		Email:    userInfo.Email,
		Username: userInfo.PreferredUsername,
		Name:     userInfo.Name,
		Source:   ThandProviderName,
	}

	session := models.Session{
		UUID:   uuid.New(),
		User:   user,
		Expiry: time.Now().Add(1 * time.Hour), // Set session expiry to 1 hour
	}

	// Add session to identities pool
	p.AddIdentities(models.Identity{
		ID:    user.ID,
		Label: user.Name,
		User:  user,
	})

	return &session, nil
}

// getUserInfo fetches user information from the userinfo endpoint
func (p *thandProvider) getUserInfo(ctx context.Context, accessToken string) (*UserInfoResponse, error) {
	// Create HTTP request using common.InvokeHttpRequest with Bearer token authentication
	resp, err := common.InvokeHttpRequest(&model.HTTPArguments{
		Method: http.MethodGet,
		Endpoint: &model.Endpoint{
			EndpointConfig: &model.EndpointConfiguration{
				URI: &model.LiteralUri{Value: fmt.Sprintf(
					"%s/%s",
					strings.TrimSuffix(p.authEndpoint, "/"),
					strings.TrimPrefix(DefaultThandUserInfoPath, "/"))},
				Authentication: &model.ReferenceableAuthenticationPolicy{
					AuthenticationPolicy: &model.AuthenticationPolicy{
						Bearer: &model.BearerAuthenticationPolicy{
							Token: accessToken,
						},
					},
				},
			},
		},
		Headers: map[string]string{
			"Accept": "application/json",
		},
	})

	if err != nil {
		return nil, fmt.Errorf("userinfo request failed: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("userinfo endpoint returned status %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	var userInfo UserInfoResponse
	if err := json.Unmarshal(resp.Body(), &userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode userinfo response: %w", err)
	}

	// Validate required fields
	if len(userInfo.Sub) == 0 {
		return nil, fmt.Errorf("invalid userinfo: missing subject (sub)")
	}

	return &userInfo, nil
}

func (p *thandProvider) ValidateSession(ctx context.Context, session *models.Session) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	// Check if session is expired
	if session.IsExpired() {
		return fmt.Errorf("session has expired")
	}

	// Optionally validate the access token by making a test call to userinfo
	if len(session.AccessToken) != 0 {
		_, err := p.getUserInfo(ctx, session.AccessToken)
		if err != nil {
			return fmt.Errorf("token validation failed: %w", err)
		}
	}

	return nil
}

func (p *thandProvider) RenewSession(ctx context.Context, session *models.Session) (*models.Session, error) {
	if session == nil {
		return nil, fmt.Errorf("session is nil")
	}

	return nil, fmt.Errorf("session renewal not implemented")
}

func init() {
	providers.Register(ThandProviderName, &thandProvider{})
}
