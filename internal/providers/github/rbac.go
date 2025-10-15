package github

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/thand-io/agent/internal/models"
)

// Authorize grants access for a user to a role
func (p *githubProvider) AuthorizeRole(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
) (map[string]any, error) {

	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to authorize github role")
	}

	user := req.GetUser()
	role := req.GetRole()

	username := user.Name

	// Process each resource in the role
	for _, resource := range role.Resources.Allow {
		if err := p.authorizeResource(ctx, username, resource, role); err != nil {
			return nil, fmt.Errorf("failed to authorize resource %s: %w", resource, err)
		}
	}

	return nil, nil
}

// Revoke removes access for a user from a role
func (p *githubProvider) RevokeRole(
	ctx context.Context,
	user *models.User,
	role *models.Role,
	metadata map[string]any,
) (map[string]any, error) {
	username := user.Name

	// Process each resource in the role
	for _, resource := range role.Resources.Allow {
		if err := p.revokeResource(ctx, username, resource); err != nil {
			return nil, fmt.Errorf("failed to revoke resource %s: %w", resource, err)
		}
	}

	return nil, nil
}

// authorizeResource handles authorization for a single resource
func (p *githubProvider) authorizeResource(ctx context.Context, username, resource string, role *models.Role) error {
	// Parse resource format to determine what type of GitHub entity it is
	// Expected formats:
	// - "org:myorg" or "github:org:myorg" -> organization membership
	// - "team:myorg/myteam" or "github:team:myorg/myteam" -> team membership
	// - "repo:owner/repo" or "github:repo:owner/repo" -> repository collaborator

	resource = strings.TrimPrefix(resource, "github:")

	parts := strings.Split(resource, ":")
	if len(parts) < 2 {
		return fmt.Errorf("invalid resource format: %s", resource)
	}

	resourceType := parts[0]
	resourcePath := parts[1]

	switch resourceType {
	case "org":
		return p.authorizeOrgMembership(ctx, username, resourcePath, role)
	case "team":
		return p.authorizeTeamMembership(ctx, username, resourcePath)
	case "repo":
		return p.authorizeRepoCollaboration(ctx, username, resourcePath, role)
	default:
		return fmt.Errorf("unsupported resource type: %s", resourceType)
	}
}

// revokeResource handles revocation for a single resource
func (p *githubProvider) revokeResource(ctx context.Context, username, resource string) error {
	resource = strings.TrimPrefix(resource, "github:")

	parts := strings.Split(resource, ":")
	if len(parts) < 2 {
		return fmt.Errorf("invalid resource format: %s", resource)
	}

	resourceType := parts[0]
	resourcePath := parts[1]

	switch resourceType {
	case "org":
		return p.revokeOrgMembership(ctx, username, resourcePath)
	case "team":
		return p.revokeTeamMembership(ctx, username, resourcePath)
	case "repo":
		return p.revokeRepoCollaboration(ctx, username, resourcePath)
	default:
		return fmt.Errorf("unsupported resource type: %s", resourceType)
	}
}

// Organization membership methods
func (p *githubProvider) authorizeOrgMembership(ctx context.Context, username, orgName string, role *models.Role) error {
	// Determine organization role from role name
	membershipRole := "member" // default
	roleName := strings.ToLower(role.Name)

	if strings.Contains(roleName, "admin") || strings.Contains(roleName, "owner") {
		membershipRole = "admin"
	}

	// TODO: Use correct GitHub SDK API for organization membership
	log.Printf("Would add user %s to org %s with role %s", username, orgName, membershipRole)
	return fmt.Errorf("GitHub API integration not yet implemented for org membership")
}

func (p *githubProvider) revokeOrgMembership(ctx context.Context, username, orgName string) error {
	// TODO: Use correct GitHub SDK API for organization membership
	log.Printf("Would remove user %s from org %s", username, orgName)
	return fmt.Errorf("GitHub API integration not yet implemented for org membership")
}

// Team membership methods
func (p *githubProvider) authorizeTeamMembership(ctx context.Context, username, teamPath string) error {
	// Parse team path: "myorg/myteam"
	parts := strings.Split(teamPath, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid team path format, expected 'org/team': %s", teamPath)
	}

	orgName, teamSlug := parts[0], parts[1]

	// TODO: Use correct GitHub SDK API for team membership
	log.Printf("Would add user %s to team %s/%s", username, orgName, teamSlug)
	return fmt.Errorf("GitHub API integration not yet implemented for team membership")
}

func (p *githubProvider) revokeTeamMembership(ctx context.Context, username, teamPath string) error {
	parts := strings.Split(teamPath, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid team path format, expected 'org/team': %s", teamPath)
	}

	orgName, teamSlug := parts[0], parts[1]

	// TODO: Use correct GitHub SDK API for team membership
	log.Printf("Would remove user %s from team %s/%s", username, orgName, teamSlug)
	return fmt.Errorf("GitHub API integration not yet implemented for team membership")
}

// Repository collaboration methods
func (p *githubProvider) authorizeRepoCollaboration(ctx context.Context, username, repoPath string, role *models.Role) error {
	// Parse repo path: "owner/repository"
	parts := strings.Split(repoPath, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo path format, expected 'owner/repo': %s", repoPath)
	}

	owner, repo := parts[0], parts[1]
	permission := p.mapRoleToPermission(role.Name)

	// TODO: Use correct GitHub SDK API for repository collaboration
	log.Printf("Would add user %s to repo %s/%s with permission %s", username, owner, repo, permission)
	return fmt.Errorf("GitHub API integration not yet implemented for repo collaboration")
}

func (p *githubProvider) revokeRepoCollaboration(ctx context.Context, username, repoPath string) error {
	parts := strings.Split(repoPath, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo path format, expected 'owner/repo': %s", repoPath)
	}

	owner, repo := parts[0], parts[1]

	// TODO: Use correct GitHub SDK API for repository collaboration
	log.Printf("Would remove user %s from repo %s/%s", username, owner, repo)
	return fmt.Errorf("GitHub API integration not yet implemented for repo collaboration")
}

// Helper function to map role names to GitHub permissions
func (p *githubProvider) mapRoleToPermission(roleName string) string {
	roleName = strings.ToLower(roleName)

	if strings.Contains(roleName, "read") || strings.Contains(roleName, "reader") {
		return "pull"
	}
	if strings.Contains(roleName, "write") || strings.Contains(roleName, "writer") {
		return "push"
	}
	if strings.Contains(roleName, "admin") || strings.Contains(roleName, "administrator") {
		return "admin"
	}
	if strings.Contains(roleName, "maintain") || strings.Contains(roleName, "maintainer") {
		return "maintain"
	}
	if strings.Contains(roleName, "triage") {
		return "triage"
	}

	// Default to read-only
	return "pull"
}

// Helper functions for OAuth flow

func (p *githubProvider) exchangeCodeForToken(ctx context.Context, code, redirectURI string) (string, error) {

	oauthClient := p.oauthClient

	data := url.Values{
		"client_id":     {oauthClient.ClientID},
		"client_secret": {oauthClient.ClientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	}

	// Use Resty client with secure TLS config
	client := resty.New()
	client.SetTimeout(10 * time.Second)
	// client.SetTLSClientConfig(&tls.Config{
	// 	InsecureSkipVerify: false,
	// })

	var tokenResp GitHubTokenResponse
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		SetBody(data.Encode()).
		SetResult(&tokenResp).
		Post(p.oauthClient.Endpoint.TokenURL)

	if err != nil {
		return "", err
	}

	if resp.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("GitHub OAuth error: %s", resp.Status())
	}

	return tokenResp.AccessToken, nil
}

func (p *githubProvider) getUserInfo(ctx context.Context, accessToken string) (*GitHubUser, error) {
	// Use Resty client with secure TLS config
	client := resty.New()
	client.SetTimeout(10 * time.Second)
	// client.SetTLSClientConfig(&tls.Config{
	// 	InsecureSkipVerify: false,
	// })

	var user GitHubUser
	resp, err := client.R().
		SetContext(ctx).
		SetAuthToken(accessToken).
		SetHeader("Accept", "application/vnd.github.v3+json").
		SetResult(&user).
		Get("https://api.github.com/user")

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("GitHub API error: %s", resp.Status())
	}

	return &user, nil
}
