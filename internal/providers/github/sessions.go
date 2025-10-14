package github

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/thand-io/agent/internal/models"
)

func (p *githubProvider) CreateSession(ctx context.Context, authRequest *models.AuthorizeUser) (*models.Session, error) {
	// Exchange authorization code for access token
	accessToken, err := p.exchangeCodeForToken(ctx, authRequest.Code, authRequest.RedirectUri)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %w", err)
	}

	// Get user information using the access token
	user, err := p.getUserInfo(ctx, accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	// Create session
	session := &models.Session{
		UUID: uuid.New(),
		User: &models.User{
			ID:     fmt.Sprintf("%d", user.ID),
			Email:  user.Email,
			Name:   user.Name,
			Source: ProviderName,
		},
		AccessToken: accessToken,
		Expiry:      time.Now().Add(24 * time.Hour), // GitHub tokens don't expire, but we set session expiry
	}

	return session, nil
}

func (p *githubProvider) ValidateSession(ctx context.Context, session *models.Session) error {
	if session.Expiry.UTC().Before(time.Now().UTC()) {
		return fmt.Errorf("session expired")
	}

	// Validate the access token by making a test API call
	_, err := p.getUserInfo(ctx, session.AccessToken)
	if err != nil {
		return fmt.Errorf("invalid session: %w", err)
	}

	return nil
}

func (p *githubProvider) RenewSession(ctx context.Context, session *models.Session) (*models.Session, error) {
	// GitHub access tokens don't expire, so we just update the session expiry
	session.Expiry = time.Now().UTC().Add(24 * time.Hour)
	return session, nil
}

func (p *githubProvider) AuthorizeSession(ctx context.Context, authRequest *models.AuthorizeUser) (*models.AuthorizeSessionResponse, error) {
	scopes := []string{"user:email", "read:org"}

	if len(authRequest.Scopes) > 0 {
		scopes = authRequest.Scopes
	}

	oauthClient := p.oauthClient

	queryParams := url.Values{
		"client_id":     {oauthClient.ClientID},
		"redirect_uri":  {authRequest.RedirectUri},
		"scope":         {strings.Join(scopes, " ")},
		"state":         {authRequest.State},
		"response_type": {"code"},
	}

	authURL := fmt.Sprintf(
		"%s?%s",
		oauthClient.Endpoint.AuthURL,
		queryParams.Encode(),
	)

	return &models.AuthorizeSessionResponse{Url: authURL}, nil
}
