package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"

	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"
)

var GithubProviderName = "github"

// githubProvider implements the ProviderImpl interface for GitHub
type githubProvider struct {
	*models.BaseProvider
	client           *github.Client
	oauthClient      *oauth2.Config
	organizationName string
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

func (p *githubProvider) Initialize(identifier string, provider models.ProviderConfig) error {

	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		GithubCapabilities,
	)

	// Right lets figure out how to initialize the GitHub SDK
	githubConfig := p.GetConfig()

	githubToken, foundToken := githubConfig.GetString("token")

	if foundToken && len(strings.TrimSpace(githubToken)) > 0 {

		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: githubToken},
		)

		tc := oauth2.NewClient(context.Background(), ts)
		p.client = github.NewClient(tc)

	} else {

		logrus.Debugln("GitHub token not provided; skipping client setup")
		p.DisableCapability(models.ProviderCapabilityProvisioning)
		p.DisableCapability(models.ProviderCapabilityIdentities)
	}

	orgName, found := githubConfig.GetString("organization")

	if found && len(orgName) > 0 {
		p.organizationName = orgName
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

	} else {

		logrus.Debugln("GitHub OAuth client_id or client_secret not provided; skipping OAuth setup")
		p.DisableCapability(models.ProviderCapabilityAuthorizer)
	}

	return nil
}

// ValidateConfig validates the GitHub configuration without initialization
func (p *githubProvider) ValidateConfig(config *models.BasicConfig) error {
	schema := &ConfigSchema{}
	if err := schema.Unmarshal(config); err != nil {
		return fmt.Errorf("failed to unmarshal GitHub config: %w", err)
	}
	return schema.Validate()
}

// Validate validates the GitHub provider configuration
func (p *githubProvider) Validate() error {
	return p.ValidateConfig(p.GetConfig())
}

func init() {
	providers.Register(GithubProviderName, &githubProvider{})
}
