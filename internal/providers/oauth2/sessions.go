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
	resolved, err := p.getResolvedConfig(ctx)
	if err != nil {
		return nil, err
	}

	scopes := resolveScopes(resolved.Scopes, authRequest.Scopes)

	redirectURI := authRequest.RedirectUri
	if redirectURI == "" {
		redirectURI = resolved.RedirectURL
	}

	queryParams := url.Values{
		"scope":         {strings.Join(scopes, " ")},
		"response_type": {"code"},
		"state":         {authRequest.State},
		"redirect_uri":  {redirectURI},
		"client_id":     {resolved.ClientID},
	}

	authURL := fmt.Sprintf("%s?%s", resolved.AuthURL, queryParams.Encode())
	return &models.AuthorizeSessionResponse{Url: authURL}, nil
}

func (p *oauth2Provider) CreateSession(ctx context.Context, authRequest *models.AuthorizeUser) (*models.Session, error) {
	resolved, err := p.getResolvedConfig(ctx)
	if err != nil {
		return nil, err
	}

	scopes := resolveScopes(resolved.Scopes, authRequest.Scopes)

	redirectURI := authRequest.RedirectUri
	if redirectURI == "" {
		redirectURI = resolved.RedirectURL
	}

	conf := &oauth2.Config{
		ClientID:     resolved.ClientID,
		ClientSecret: resolved.ClientSecret,
		RedirectURL:  redirectURI,
		Scopes:       scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  resolved.AuthURL,
			TokenURL: resolved.TokenURL,
		},
	}

	token, err := conf.Exchange(ctx, authRequest.Code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %w", err)
	}

	// Try to get user info from the ID token first
	user, err := getUserInfoFromIDToken(token, resolved.UsernameClaim)
	if err != nil || user == nil {
		userInfoURL := resolved.UserInfoURL
		if userInfoURL == "" {
			userInfoURL = deriveUserInfoURL(resolved.TokenURL)
		}

		if userInfoURL == "" {
			return nil, fmt.Errorf("failed to get user info: no userinfo endpoint configured")
		}

		userFromEndpoint, endpointErr := getUserInfoFromEndpoint(ctx, userInfoURL, token.AccessToken, resolved.UsernameClaim)
		if endpointErr != nil {
			return nil, fmt.Errorf("failed to get user info: %w", endpointErr)
		}

		user = userFromEndpoint
	} else if needsUsernameFallback(user) {
		userInfoURL := resolved.UserInfoURL
		if userInfoURL == "" {
			userInfoURL = deriveUserInfoURL(resolved.TokenURL)
		}

		if userInfoURL != "" {
			userFromEndpoint, endpointErr := getUserInfoFromEndpoint(ctx, userInfoURL, token.AccessToken, resolved.UsernameClaim)
			if endpointErr == nil && userFromEndpoint != nil && user.Username == "" && userFromEndpoint.Username != "" {
				// Preserve the ID token as authoritative for the core identity fields and
				// only enrich the missing username from userinfo when available.
				user.Username = userFromEndpoint.Username
			}
		}
	}

	idToken, _ := token.Extra("id_token").(string)

	session := models.Session{
		UUID:         uuid.New(),
		User:         user,
		Token:        idToken,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry,
	}

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

	return &session, nil
}

func resolveScopes(defaultScopes, requestedScopes []string) []string {
	scopes := append([]string(nil), defaultScopes...)
	if len(requestedScopes) > 0 {
		scopes = append([]string(nil), requestedScopes...)
	}
	if len(scopes) == 0 {
		scopes = []string{"openid"}
	}

	for _, scope := range scopes {
		if scope == "openid" {
			return scopes
		}
	}

	return append([]string{"openid"}, scopes...)
}

// getUserInfoFromIDToken tries to extract user info from the ID token JWT claims.
func getUserInfoFromIDToken(token *oauth2.Token, usernameClaim string) (*models.User, error) {
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

	return userFromClaims(claims, usernameClaim, "id_token")
}

// getUserInfoFromEndpoint calls the OIDC userinfo endpoint to get user details.
func getUserInfoFromEndpoint(ctx context.Context, userInfoURL string, accessToken string, usernameClaim string) (*models.User, error) {
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

	return userFromClaims(info, usernameClaim, "userinfo response")
}

func userFromClaims(claims map[string]interface{}, usernameClaim string, subjectSource string) (*models.User, error) {
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, fmt.Errorf("%s missing sub", subjectSource)
	}

	email, _ := claims["email"].(string)
	name, _ := claims["name"].(string)
	if name == "" {
		name = email
	}

	verified := claimBool(claims, "email_verified")

	return &models.User{
		ID:       sub,
		Email:    email,
		Username: claimUsername(claims, usernameClaim),
		Name:     name,
		Verified: &verified,
		Source:   "oauth2",
	}, nil
}

func claimUsername(claims map[string]interface{}, configuredClaim string) string {
	for _, claim := range usernameClaimKeys(configuredClaim) {
		if username := claimString(claims, claim); username != "" {
			return username
		}
	}

	return ""
}

func usernameClaimKeys(configuredClaim string) []string {
	if claim := strings.TrimSpace(configuredClaim); claim != "" {
		return []string{claim}
	}

	return []string{"preferred_username", "username"}
}

func claimString(claims map[string]interface{}, key string) string {
	value, _ := claims[key].(string)
	return strings.TrimSpace(value)
}

func claimBool(claims map[string]interface{}, key string) bool {
	value, _ := claims[key].(bool)
	return value
}

func needsUsernameFallback(user *models.User) bool {
	if user == nil {
		return true
	}

	return strings.TrimSpace(user.Username) == ""
}

func deriveUserInfoURL(tokenURL string) string {
	if tokenURL == "" {
		return ""
	}

	parsedURL, err := url.Parse(tokenURL)
	if err != nil {
		return ""
	}

	path := parsedURL.Path
	if path == "" {
		return ""
	}

	hasTrailingSlash := strings.HasSuffix(path, "/")
	trimmedPath := strings.TrimSuffix(path, "/")
	lastSlash := strings.LastIndex(trimmedPath, "/")

	lastSegment := trimmedPath
	if lastSlash >= 0 {
		lastSegment = trimmedPath[lastSlash+1:]
	}
	if lastSegment != "token" {
		return ""
	}

	if lastSlash >= 0 {
		parsedURL.Path = trimmedPath[:lastSlash+1] + "userinfo"
	} else {
		parsedURL.Path = "userinfo"
	}
	if hasTrailingSlash {
		parsedURL.Path += "/"
	}

	return parsedURL.String()
}

func (p *oauth2Provider) ValidateSession(ctx context.Context, session *models.Session) error {
	return nil
}

func (p *oauth2Provider) RenewSession(ctx context.Context, session *models.Session) (*models.Session, error) {
	return session, nil
}
