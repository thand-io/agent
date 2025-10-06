package googleoauth2

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	oauth2api "google.golang.org/api/oauth2/v2"
	"google.golang.org/api/option"
)

var ProviderName = "oauth2.google"

// oauth2Provider implements the ProviderImpl interface for OAuth2
type oauth2Provider struct {
	*models.BaseProvider
	OauthConfig *oauth2.Config
}

func (p *oauth2Provider) Initialize(provider models.Provider) error {
	p.BaseProvider = models.NewBaseProvider(
		provider,
		models.ProviderCapabilityAuthorizor,
	)

	// Get client id and secret from the config
	googleConfig := p.GetConfig()

	clientID, foundClientID := googleConfig.GetString("client_id")
	clientSecret, foundClientSecret := googleConfig.GetString("client_secret")

	if !foundClientID || !foundClientSecret {
		return fmt.Errorf("client_id and client_secret must be set in the config")
	}

	scopes, foundScopes := googleConfig.GetStringSlice("scopes")

	if !foundScopes || len(scopes) == 0 {
		scopes = []string{
			"email",
			"profile",
		}
	}

	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       scopes,
		Endpoint:     google.Endpoint,
	}

	p.OauthConfig = conf

	return nil
}

func (p *oauth2Provider) AuthorizeSession(ctx context.Context, authRequest *models.AuthorizeUser) (*models.AuthorizeSessionResponse, error) {

	googleConfig := p.OauthConfig

	scopes := googleConfig.Scopes

	if len(authRequest.Scopes) > 0 {
		scopes = authRequest.Scopes
	}

	queryParams := url.Values{
		"scope":         {strings.Join(scopes, " ")},
		"response_type": {"code"},
		"state":         {authRequest.State},
		"redirect_uri":  {authRequest.RedirectUri},
		"client_id":     {googleConfig.ClientID},
		"access_type":   {"offline"}, // Request refresh token
		"prompt":        {"consent"}, // Request consent screen
		// "hd":          {"example.com"}, // Request domain-wide delegation
	}

	url := fmt.Sprintf(
		"%s?%s",
		googleConfig.Endpoint.AuthURL,
		queryParams.Encode(),
	)

	return &models.AuthorizeSessionResponse{Url: url}, nil
}

func (p *oauth2Provider) CreateSession(ctx context.Context, authRequest *models.AuthorizeUser) (*models.Session, error) {

	googleConfig := p.OauthConfig

	scopes := googleConfig.Scopes

	if len(authRequest.Scopes) > 0 {
		scopes = authRequest.Scopes
	}

	// TODO: make this a helper function
	conf := &oauth2.Config{
		ClientID:     googleConfig.ClientID,
		ClientSecret: googleConfig.ClientSecret,
		RedirectURL:  authRequest.RedirectUri,
		Scopes:       scopes,
		Endpoint:     google.Endpoint,
	}

	// Use a new context with a secure http client
	secureContext := context.WithValue(context.TODO(), oauth2.HTTPClient, &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false, // Ensure this is false for production
			},
		},
	})

	token, err := conf.Exchange(secureContext, authRequest.Code)
	if err != nil {
		return nil, err
	}

	oauth2Service, err := oauth2api.NewService(
		ctx,
		option.WithTokenSource(conf.TokenSource(ctx, token)),
	)
	if err != nil {
		return nil, err
	}

	userInfo, err := oauth2Service.Userinfo.Get().Do()
	if err != nil {
		return nil, err
	}

	session := models.Session{
		UUID: uuid.New(),
		User: &models.User{
			ID:       userInfo.Id,
			Email:    userInfo.Email,
			Name:     userInfo.Name,
			Verified: userInfo.VerifiedEmail,
			Provider: "google",
		},
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry,
	}

	return &session, nil
}

func (p *oauth2Provider) ValidateSession(ctx context.Context, session *models.Session) error {
	// TODO: Implement OAuth2 session validation logic
	return nil
}

func (p *oauth2Provider) RenewSession(ctx context.Context, session *models.Session) (*models.Session, error) {
	// TODO: Implement OAuth2 session renewal logic
	return session, nil
}

func init() {
	providers.Register(ProviderName, &oauth2Provider{})
}
