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

// Authentication Callback Handlers
//
// This file implements two separate callback handlers:
//
// 1. getAuthCallback() - OAuth2 GET callbacks
//    - Expects state and code in query parameters
//    - Used by OAuth2 providers (GitHub, Google, etc.)
//
// 2. postAuthCallback() - SAML POST callbacks
//    - Expects RelayState and SAMLResponse in form parameters
//    - Supports SP-initiated (with RelayState) and IdP-initiated (no RelayState)
//    - Used by SAML providers (Okta, Azure AD, etc.)
//
// Both handlers delegate to getAuthCallbackPage() for session creation.

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

	// Validate callback URL to prevent infinite loops
	// Only block callbacks that would loop back to the auth request endpoint
	if len(callback) > 0 {
		callbackURL, callbackErr := url.Parse(callback)
		loginServerURL, loginServerErr := url.Parse(config.GetLoginServerUrl())

		if callbackErr == nil && loginServerErr == nil {
			// Block only if it's the same host and the callback would loop back to /api/v1/auth/request
			if callbackURL.Host == loginServerURL.Host &&
				strings.HasPrefix(callbackURL.Path, "/api/v1/auth/request") {
				s.getErrorPage(c, http.StatusBadRequest, "Callback cannot be the auth request endpoint - this would create an infinite loop")
				return
			}
		} else {
			// If we can't parse the URLs, log the error but allow the request to proceed
			logrus.WithFields(logrus.Fields{
				"callback":       callback,
				"callbackErr":    callbackErr,
				"loginServerErr": loginServerErr,
			}).Warnln("Failed to parse callback or login server URL for validation")
		}
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

	encodedState := models.EncodingWrapper{
		Type: models.ENCODED_AUTH,
		Data: models.NewAuthWrapper(
			callback,        // where are we returning to
			client.String(), // server identifier
			provider,        // provider name
			code,            // the code sent by the client
		),
	}.EncodeAndEncrypt(
		s.Config.GetServices().GetEncryption(),
	)

	logrus.WithFields(logrus.Fields{
		"encodedState": encodedState,
		"stateLength":  len(encodedState),
	}).Debugln("Encoded state for auth request")

	authResponse, err := providerConfig.GetClient().AuthorizeSession(
		context.Background(),
		// This creates the state payload for the auth request
		&models.AuthorizeUser{
			Scopes:      []string{"email", "profile"},
			State:       encodedState,
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

// getAuthCallback handles OAuth2 GET callback requests
//
//	@Summary		OAuth2 authentication callback
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
	// OAuth2 flow: state and code come in query parameters (GET)
	state := c.Query("state")

	// Debug logging
	logrus.WithFields(logrus.Fields{
		"method": c.Request.Method,
		"state":  state,
	}).Debugln("OAuth2 callback parameters")

	// Validate state parameter is required for OAuth2
	if len(state) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "State is required for OAuth2 flow")
		return
	}

	// Decode and decrypt state
	decoded, err := s.decodeState(state)
	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid state", err)
		return
	}

	// Process decoded state
	s.processDecodedState(c, decoded)
}

// postAuthCallback handles SAML POST callback requests
//
//	@Summary		SAML authentication callback
//	@Description	Handle the SAML POST callback from the provider
//	@Tags			auth
//	@Accept			x-www-form-urlencoded
//	@Produce		json
//	@Param			provider		path		string	true	"Provider name"
//	@Param			RelayState		formData	string	false	"SAML RelayState (SP-initiated)"
//	@Param			SAMLResponse	formData	string	true	"SAML Response"
//	@Success		200				"Authentication successful"
//	@Failure		400				{object}	map[string]any	"Bad request"
//	@Router			/auth/callback/{provider} [post]
func (s *Server) postAuthCallback(c *gin.Context) {
	// SAML flow: RelayState and SAMLResponse come in form parameters (POST)
	relayState := c.PostForm("RelayState")
	samlResponse := c.PostForm("SAMLResponse")

	// Debug logging
	logrus.WithFields(logrus.Fields{
		"method":       c.Request.Method,
		"relay_state":  relayState,
		"has_response": len(samlResponse) > 0,
	}).Debugln("SAML callback parameters")

	// Handle IdP-initiated SAML flow (no RelayState parameter)
	if len(relayState) == 0 {
		// Check if this is a SAML callback with SAMLResponse
		if len(samlResponse) > 0 {
			// IdP-initiated flow: create a default auth wrapper
			providerName := c.Param("provider")
			logrus.WithFields(logrus.Fields{
				"provider": providerName,
			}).Info("Handling IdP-initiated SAML flow")

			authWrapper := models.AuthWrapper{
				Callback: "", // No callback for IdP-initiated
				Provider: providerName,
				Code:     "", // No client code
				Client:   "", // No client identifier
			}
			s.getAuthCallbackPage(c, authWrapper)
			return
		}

		// Not a SAML IdP-initiated flow, RelayState is required
		s.getErrorPage(c, http.StatusBadRequest, "RelayState is required for SP-initiated SAML flow")
		return
	}

	// SP-initiated flow: decode and decrypt RelayState
	decoded, err := s.decodeState(relayState)
	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid RelayState", err)
		return
	}

	// Process decoded state
	s.processDecodedState(c, decoded)
}

// decodeState decodes and decrypts the state parameter
func (s *Server) decodeState(state string) (models.EncodingWrapper, error) {
	decoded, err := models.EncodingWrapper{}.DecodeAndDecrypt(
		state,
		s.Config.GetServices().GetEncryption(),
	)
	if err != nil {
		return models.EncodingWrapper{}, fmt.Errorf("failed to decode state: %w", err)
	}
	return *decoded, nil
}

// processDecodedState routes based on decoded state type
func (s *Server) processDecodedState(c *gin.Context, decoded models.EncodingWrapper) {
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

	// For OAuth2: state and code come in query parameters (GET)
	// For SAML: RelayState and SAMLResponse come in form parameters (POST)
	state := c.Query("state")
	if len(state) == 0 {
		state = c.PostForm("RelayState")
	}

	code := c.Query("code") // This is the code from the provider - not the client
	if len(code) == 0 {
		code = c.PostForm("SAMLResponse")
	}

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
		// Use 303 See Other to force a GET redirect from the POST callback
		c.Redirect(http.StatusSeeOther, "/")
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
