package github

import (
	"fmt"
	"time"

	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"

	"github.com/octokit/go-sdk/pkg"
	"golang.org/x/oauth2"
)

var ProviderName = "github"

// githubProvider implements the ProviderImpl interface for GitHub
type githubProvider struct {
	*models.BaseProvider
	client      *pkg.Client
	oauthClient *oauth2.Config
	permissions []models.ProviderPermission
	roles       []models.ProviderRole
}

// GitHubUser represents the GitHub user response
type GitHubUser struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// GitHubTokenResponse represents the OAuth token response
type GitHubTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

func (p *githubProvider) Initialize(provider models.Provider) error {

	p.BaseProvider = models.NewBaseProvider(
		provider,
		models.ProviderCapabilityAuthorizor,
		models.ProviderCapabilityRBAC,
	)

	// Right lets figure out how to initialize the GitHub SDK
	githubConfig := p.GetConfig()

	githubEndpoint, foundEndpoint := githubConfig.GetString("endpoint")
	githubToken, foundToken := githubConfig.GetString("token")

	if !foundToken {
		return fmt.Errorf("missing required GitHub configuration: either token or OAuth credentials required")
	}

	if !foundEndpoint || len(githubEndpoint) == 0 {
		githubEndpoint = "https://api.github.com"
	}

	if foundToken {
		githubClient, err := pkg.NewApiClient(
			pkg.WithUserAgent("thand"),
			pkg.WithRequestTimeout(5*time.Second),
			pkg.WithBaseUrl(githubEndpoint),
			pkg.WithTokenAuthentication(githubToken),
		)

		if err != nil {
			return fmt.Errorf("error creating GitHub client: %v", err)
		}

		p.client = githubClient
	}

	// OAuth configuration
	clientID, foundClientId := githubConfig.GetString("client_id")
	clientSecret, foundClientSecret := githubConfig.GetString("client_secret")

	// Create a client config
	if foundClientId && foundClientSecret {
		conf := &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Scopes:       []string{"user"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://github.com/login/oauth/authorize",
				TokenURL: "https://github.com/login/oauth/access_token",
			},
		}

		p.oauthClient = conf
	}

	p.permissions = GitHubPermissions
	p.roles = GitHubRoles

	return nil
}

func init() {
	providers.Register(ProviderName, &githubProvider{})
}
