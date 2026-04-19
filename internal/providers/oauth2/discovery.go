package oauth2

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const oidcDiscoveryPath = "/.well-known/openid-configuration"

type oidcDiscoveryDocument struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
}

type resolvedOAuth2Config struct {
	ClientID      string
	ClientSecret  string
	RedirectURL   string
	Scopes        []string
	UsernameClaim string
	Authority     string
	AuthURL       string
	TokenURL      string
	UserInfoURL   string
}

func (p *oauth2Provider) getResolvedConfig(ctx context.Context) (*resolvedOAuth2Config, error) {
	p.resolvedConfigMu.RLock()
	if p.resolvedConfig != nil {
		defer p.resolvedConfigMu.RUnlock()
		return p.resolvedConfig, nil
	}
	p.resolvedConfigMu.RUnlock()
	p.resolvedConfigMu.Lock()
	defer p.resolvedConfigMu.Unlock()

	if p.resolvedConfig != nil {
		return p.resolvedConfig, nil
	}
	schema := &ConfigSchema{}
	if err := schema.Unmarshal(p.GetConfig()); err != nil {
		return nil, fmt.Errorf("failed to unmarshal OAuth2 config: %w", err)
	}

	resolved := &resolvedOAuth2Config{
		ClientID:      schema.ClientID,
		ClientSecret:  schema.ClientSecret,
		RedirectURL:   schema.RedirectURL,
		Scopes:        append([]string(nil), schema.Scopes...),
		UsernameClaim: schema.UsernameClaim,
		Authority:     schema.Authority,
		AuthURL:       schema.AuthURL,
		TokenURL:      schema.TokenURL,
		UserInfoURL:   schema.UserInfoURL,
	}
	needsDiscovery := strings.TrimSpace(resolved.AuthURL) == "" ||
		strings.TrimSpace(resolved.TokenURL) == "" ||
		strings.TrimSpace(resolved.UserInfoURL) == ""

	if strings.TrimSpace(schema.Authority) != "" && needsDiscovery {
		discoveryURL := normalizeDiscoveryURL(schema.Authority)
		document, err := fetchOIDCDiscoveryDocument(ctx, p.getHTTPClient(), discoveryURL)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(resolved.AuthURL) == "" {
			resolved.AuthURL = document.AuthorizationEndpoint
		}
		if strings.TrimSpace(resolved.TokenURL) == "" {
			resolved.TokenURL = document.TokenEndpoint
		}
		if strings.TrimSpace(resolved.UserInfoURL) == "" {
			resolved.UserInfoURL = document.UserInfoEndpoint
		}
	}

	if strings.TrimSpace(resolved.AuthURL) == "" {
		return nil, fmt.Errorf("OAuth2 config resolution failed: authorization endpoint is empty")
	}
	if strings.TrimSpace(resolved.TokenURL) == "" {
		return nil, fmt.Errorf("OAuth2 config resolution failed: token endpoint is empty")
	}
	p.resolvedConfig = resolved
	return p.resolvedConfig, nil
}

func normalizeDiscoveryURL(authority string) string {
	trimmed := strings.TrimSpace(authority)
	parsedURL, err := url.Parse(trimmed)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		normalized := strings.TrimRight(trimmed, "/")
		if strings.HasSuffix(normalized, oidcDiscoveryPath) {
			return normalized
		}
		return normalized + oidcDiscoveryPath
	}

	normalizedPath := strings.TrimRight(parsedURL.Path, "/")
	if strings.HasSuffix(normalizedPath, oidcDiscoveryPath) {
		parsedURL.Path = normalizedPath
		return parsedURL.String()
	}
	if normalizedPath == "" {
		parsedURL.Path = oidcDiscoveryPath
	} else {
		parsedURL.Path = normalizedPath + oidcDiscoveryPath
	}

	return parsedURL.String()
}

func fetchOIDCDiscoveryDocument(ctx context.Context, httpClient *http.Client, discoveryURL string) (*oidcDiscoveryDocument, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC discovery request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OIDC discovery endpoint returned %d", resp.StatusCode)
	}

	var document oidcDiscoveryDocument
	if err := json.NewDecoder(resp.Body).Decode(&document); err != nil {
		return nil, fmt.Errorf("failed to decode OIDC discovery response: %w", err)
	}

	return &document, nil
}
