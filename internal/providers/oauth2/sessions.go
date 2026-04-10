package oauth2

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"golang.org/x/oauth2"
)

func (p *oauth2Provider) AuthorizeSession(ctx context.Context, authRequest *models.AuthorizeUser) (*models.AuthorizeSessionResponse, error) {
	schema := &ConfigSchema{}
	if err := schema.Unmarshal(p.GetConfig()); err != nil {
		return nil, fmt.Errorf("failed to unmarshal OAuth2 config: %w", err)
	}

	scopes := schema.Scopes
	if len(authRequest.Scopes) > 0 {
		scopes = authRequest.Scopes
	}
	if len(scopes) == 0 {
		scopes = []string{"openid"}
	}
	// Ensure openid scope is always included for OIDC compliance
	hasOpenID := false
	for _, s := range scopes {
		if s == "openid" {
			hasOpenID = true
			break
		}
	}
	if !hasOpenID {
		scopes = append([]string{"openid"}, scopes...)
	}

	redirectURI := authRequest.RedirectUri
	if redirectURI == "" {
		redirectURI = schema.RedirectURL
	}

	queryParams := url.Values{
		"scope":         {strings.Join(scopes, " ")},
		"response_type": {"code"},
		"state":         {authRequest.State},
		"redirect_uri":  {redirectURI},
		"client_id":     {schema.ClientID},
	}

	authURL := fmt.Sprintf("%s?%s", schema.AuthURL, queryParams.Encode())
	return &models.AuthorizeSessionResponse{Url: authURL}, nil
}

func (p *oauth2Provider) CreateSession(ctx context.Context, authRequest *models.AuthorizeUser) (*models.Session, error) {
	schema := &ConfigSchema{}
	if err := schema.Unmarshal(p.GetConfig()); err != nil {
		return nil, fmt.Errorf("failed to unmarshal OAuth2 config: %w", err)
	}

	scopes := schema.Scopes
	if len(authRequest.Scopes) > 0 {
		scopes = authRequest.Scopes
	}

	redirectURI := authRequest.RedirectUri
	if redirectURI == "" {
		redirectURI = schema.RedirectURL
	}

	conf := &oauth2.Config{
		ClientID:     schema.ClientID,
		ClientSecret: schema.ClientSecret,
		RedirectURL:  redirectURI,
		Scopes:       scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  schema.AuthURL,
			TokenURL: schema.TokenURL,
		},
	}

	token, err := conf.Exchange(ctx, authRequest.Code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %w", err)
	}

	// Try to get user info from the ID token first
	user, err := getUserInfoFromIDToken(token)
	if err != nil || user == nil {
		// Fallback: call userinfo endpoint
		user, err = getUserInfoFromEndpoint(ctx, schema.TokenURL, token.AccessToken)
		if err != nil {
			return nil, fmt.Errorf("failed to get user info: %w", err)
		}
	}

	session := buildAuthOnlySession(user, token)

	// Log the identity information being added
	// ONLY log in debug to avoid PII leakage
	logrus.WithFields(logrus.Fields{
		"user_id":    user.ID,
		"user_email": user.Email,
		"user_name":  user.Name,
	}).Debug("Adding OAuth2 user identity to provider")

	p.AddIdentities(models.Identity{
		ID:    user.ID,
		Label: user.Name,
		User:  user,
	})

	return session, nil
}

func buildAuthOnlySession(user *models.User, token *oauth2.Token) *models.Session {
	session := &models.Session{
		UUID: uuid.New(),
		User: user,
	}

	if token != nil {
		session.Expiry = token.Expiry
	}

	// Generic oauth2 providers in this repo are used for browser authentication
	// and identity discovery only. Persisting raw OAuth tokens in cookies can
	// easily exceed browser size limits, while later generic oauth2 validation
	// paths do not consume those token fields.
	return session
}

// getUserInfoFromIDToken tries to extract user info from the ID token JWT claims.
func getUserInfoFromIDToken(token *oauth2.Token) (*models.User, error) {
	idToken, ok := token.Extra("id_token").(string)
	if !ok || idToken == "" {
		return nil, fmt.Errorf("no id_token in response")
	}

	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, fmt.Errorf("no sub claim in JWT")
	}

	email, _ := claims["email"].(string)
	name, _ := claims["name"].(string)
	if name == "" {
		name = email
	}

	verifiedRaw, _ := claims["email_verified"].(bool)
	verified := verifiedRaw

	return &models.User{
		ID:       sub,
		Email:    email,
		Name:     name,
		Verified: &verified,
		Source:   "oauth2",
	}, nil
}

// getUserInfoFromEndpoint calls the OIDC userinfo endpoint to get user details.
func getUserInfoFromEndpoint(ctx context.Context, tokenURL string, accessToken string) (*models.User, error) {
	// Derive userinfo URL from token URL (OIDC standard convention)
	// e.g., .../protocol/openid-connect/token -> .../protocol/openid-connect/userinfo
	userInfoURL := strings.Replace(tokenURL, "/token", "/userinfo", 1)

	req, err := http.NewRequestWithContext(ctx, "GET", userInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo endpoint returned %d", resp.StatusCode)
	}

	var info map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	sub, _ := info["sub"].(string)
	email, _ := info["email"].(string)
	name, _ := info["name"].(string)
	if name == "" {
		name = email
	}

	verified := true

	return &models.User{
		ID:       sub,
		Email:    email,
		Name:     name,
		Verified: &verified,
		Source:   "oauth2",
	}, nil
}

func (p *oauth2Provider) ValidateSession(ctx context.Context, session *models.Session) error {
	return nil
}

func (p *oauth2Provider) RenewSession(ctx context.Context, session *models.Session) (*models.Session, error) {
	return session, nil
}
