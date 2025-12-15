package daemon

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// getAuthRequest initiates the authentication flow
//
//	@Summary		Initiate authentication
//	@Description	Start the OAuth2 authentication flow for a provider
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			provider	path	string	true	"Provider name"
//	@Param			callback	query	string	false	"Callback URL"
//	@Success		307			"Redirect to provider authentication"
//	@Failure		400			{object}	map[string]any	"Bad request"
//	@Failure		404			{object}	map[string]any	"Provider not found"
//	@Failure		500			{object}	map[string]any	"Internal server error"
//	@Router			/auth/request/{provider} [get]
func (s *Server) getAuthRequest(c *gin.Context) {
	provider := c.Param("provider")

	if len(provider) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Provider is required")
		return
	}

	callback := c.Query("callback")
	code := c.Query("code") // Optional

	config := s.GetConfig()

	if len(callback) > 0 && strings.Compare(callback, config.GetLoginServerUrl()) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Callback cannot be the login server")
		return
	}

	logrus.WithFields(logrus.Fields{
		"provider":    provider,
		"callback":    callback,
		"code":        code,
		"loginServer": config.GetLoginServerUrl(),
	}).Debugln("Initiating authentication request")

	providerConfig, err := config.GetProviderByName(provider)

	if err != nil {
		s.getErrorPage(c, http.StatusNotFound, "Provider not found", err)
		return
	}

	client := common.GetClientIdentifier()

	authResponse, err := providerConfig.GetClient().AuthorizeSession(
		context.Background(),
		// This creates the state payload for the auth request
		&models.AuthorizeUser{
			Scopes: []string{"email", "profile"},
			State: models.EncodingWrapper{
				Type: models.ENCODED_AUTH,
				Data: models.NewAuthWrapper(
					callback,        // where are we returning to
					client.String(), // server identifier
					provider,        // provider name
					code,            // the code sent by the client
				),
			}.EncodeAndEncrypt(
				s.Config.GetServices().GetEncryption(),
			),
			RedirectUri: s.GetConfig().GetAuthCallbackUrl(provider),
		},
	)

	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to authorize user", err)
		return
	}

	c.Redirect(
		http.StatusTemporaryRedirect,
		authResponse.Url,
	)
}

// getAuthCallback handles the OAuth2 callback
//
//	@Summary		Authentication callback
//	@Description	Handle the OAuth2 callback from the provider
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			provider	path	string	true	"Provider name"
//	@Param			state		query	string	true	"OAuth state"
//	@Param			code		query	string	true	"OAuth code"
//	@Success		200			"Authentication successful"
//	@Failure		400			{object}	map[string]any	"Bad request"
//	@Router			/auth/callback/{provider} [get]
func (s *Server) getAuthCallback(c *gin.Context) {

	// Handle the callback to the CLI to store the users session state

	// Check if the callback is a workflow resumption or
	// a local callback response

	state := c.Query("state")

	if len(state) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "State is required")
		return
	}

	decoded, err := models.EncodingWrapper{}.DecodeAndDecrypt(
		state,
		s.Config.GetServices().GetEncryption(),
	)

	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid state", err)
		return
	}

	switch decoded.Type {
	case models.ENCODED_WORKFLOW_TASK:
		s.getElevateAuthOAuth2(c)
	case models.ENCODED_AUTH:

		authWrapper := models.AuthWrapper{}
		err := common.ConvertMapToInterface(
			decoded.Data.(map[string]any), &authWrapper)

		if err != nil {
			s.getErrorPage(c, http.StatusBadRequest, "Invalid state data", err)
			return
		}

		s.getAuthCallbackPage(c, authWrapper)

	default:
		s.getErrorPage(c, http.StatusBadRequest, "Invalid state type")
	}
}

type AuthPageData struct {
	TemplateData
	Providers map[string]models.ProviderResponse
	Callback  string
	Code      string
}

// getAuthPage displays the authentication page
//
//	@Summary		Authentication page
//	@Description	Display the authentication page with available providers
//	@Tags			auth
//	@Accept			json
//	@Produce		html
//	@Param			callback	query	string	false	"Callback URL"
//	@Param			provider	query	string	false	"Provider name for direct authentication"
//	@Success		200			"Authentication page"
//	@Failure		400			{object}	map[string]any	"Bad request"
//	@Router			/auth [get]
func (s *Server) getAuthPage(c *gin.Context) {

	foundProviders := s.getAuthProvidersAsProviderResponse(nil)

	if len(foundProviders) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "No providers",
			fmt.Errorf("no authentication providers found. That provide authentication support"))
		return
	}

	callback, foundCallback := c.GetQuery("callback")

	if !foundCallback || len(callback) == 0 {
		logrus.Debug("Using local server URL as callback")
	}

	// Has a provider been specified
	provider, foundProvider := c.GetQuery("provider")
	code := c.Query("code")

	if foundProvider && len(provider) > 0 {

		// If a provider has been specified then redirect to the
		// auth endpoint for that provider

		if _, exists := foundProviders[provider]; !exists {
			s.getErrorPage(c, http.StatusBadRequest, "Invalid provider",
				fmt.Errorf("provider %s not found", provider))
			return
		}

		// {{$.Config.GetApiBasePath}}/auth/request/{{$key}}?callback={{$.Callback}}

		params := url.Values{
			"code":     {code},
			"callback": {callback},
		}

		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/auth/request/%s?callback=%s",
			s.Config.GetApiBasePath(),
			provider,
			params.Encode(),
		))

		return

	} else {

		data := AuthPageData{
			TemplateData: s.GetTemplateData(c),
			Providers:    foundProviders,
			Callback:     callback,
			Code:         code,
		}

		s.renderHtml(c, "auth.html", data)
		return

	}
}

type AuthCallbackPageData struct {
	TemplateData
	Auth        models.AuthWrapper
	Session     *models.LocalSession
	LoginServer string
}

func (s *Server) getAuthCallbackPage(c *gin.Context, auth models.AuthWrapper) {

	// Get the provider and pull back the user session into
	// the context

	provider, err := s.Config.GetProviderByName(auth.Provider)

	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid provider", err)
		return
	}

	state := c.Query("state")
	code := c.Query("code") // This is the code from the provider - not the client

	session, err := provider.GetClient().CreateSession(c, &models.AuthorizeUser{
		State:       state,
		Code:        code,
		RedirectUri: s.GetConfig().GetAuthCallbackUrl(auth.Provider),
	})

	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to create session", err)
		return
	}

	if session == nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Session is nil")
		return
	}

	exportableSession := &models.ExportableSession{
		Session:  session,
		Provider: auth.Provider,
	}

	// Covert our sensitive session to one we can store on the users local system
	localSession := exportableSession.ToLocalSession(
		s.Config.GetServices().GetEncryption())

	data := AuthCallbackPageData{
		TemplateData: s.GetTemplateData(c),
		Auth:         auth,
		Session:      localSession,
		LoginServer:  s.Config.GetLoginServerUrl(),
	}

	if err := s.setAuthCookie(c, auth.Provider, localSession); err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to set auth cookie", err)
		return
	}

	if len(auth.Callback) == 0 {
		c.Redirect(http.StatusTemporaryRedirect, "/")
	} else {
		s.renderHtml(c, "auth_callback.html", data)
	}
}

// getLogoutPage handles user logout
//
//	@Summary		Logout
//	@Description	Clear the user session and logout
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			provider	path	string	false	"Provider name"
//	@Success		200			{object}	map[string]any	"Logged out successfully"
//	@Success		307			"Redirect to home page"
//	@Failure		500			{object}	map[string]any	"Internal server error"
//	@Router			/auth/logout [get]
//	@Router			/auth/logout/{provider} [get]
func (s *Server) getLogoutPage(c *gin.Context) {

	cookie := sessions.DefaultMany(c, ThandCookieName)
	cookie.Clear()

	// Need to loop over all cookies and clear them
	provider := c.Param("provider")

	if len(provider) > 0 {

		_, err := s.Config.GetProviderByName(provider)

		if err != nil {
			s.getErrorPage(c, http.StatusBadRequest, "Invalid provider", err)
			return
		}

		providerCookie := sessions.DefaultMany(c, CreateCookieName(provider))
		providerCookie.Clear()

		err = providerCookie.Save()

		if err != nil {
			s.getErrorPage(c, http.StatusInternalServerError, "Failed to clear provider session", err)
			return
		}

	} else {

		allProviders := s.Config.GetProvidersByCapability(models.ProviderCapabilityAuthorizer)

		for providerName := range allProviders {

			cookie := sessions.DefaultMany(c, CreateCookieName(providerName))

			if cookie == nil {
				continue
			}

			cookie.Clear()
			err := cookie.Save()

			if err != nil {
				s.getErrorPage(c, http.StatusInternalServerError, "Failed to clear provider session", err)
				return
			}

		}

	}

	err := cookie.Save()

	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to clear session", err)
		return
	}

	if s.canAcceptHtml(c) {
		c.Redirect(http.StatusTemporaryRedirect, "/")
	} else {
		c.JSON(http.StatusOK, gin.H{
			"message": "Logged out successfully",
		})
	}

}
