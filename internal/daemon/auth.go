package daemon

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
)

func (s *Server) getAuthRequest(c *gin.Context) {
	provider := c.Param("provider")

	if len(provider) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Provider is required")
		return
	}

	callback := c.Query("callback")

	config := s.GetConfig()

	if len(callback) == 0 {
		callback = config.GetLocalServerUrl()
	}

	if strings.Compare(callback, config.GetLoginServerUrl()) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Callback cannot be the login server")
		return
	}

	fmt.Println("Callback URL:", callback)
	fmt.Println("login server URL:", config.GetLoginServerUrl())

	providerConfig, err := config.GetProviderByName(provider)

	if err != nil {
		s.getErrorPage(c, http.StatusNotFound, "Provider not found", err)
		return
	}

	client := common.GetClientIdentifier()

	authResponse, err := providerConfig.GetClient().AuthorizeSession(
		context.Background(),
		&models.AuthorizeUser{
			Scopes: []string{"email", "profile"},
			State: models.EncodingWrapper{
				Type: models.ENCODED_AUTH,
				Data: models.NewAuthWrapper(callback, client, provider),
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

func (s *Server) getAuthCallback(c *gin.Context) {

	// Handle the callback to the CLI to store the users session state

	// Check if the callback is a workflow resumption or
	// a local callback response

	state := c.Query("state")

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
	config.TemplateData
	Providers map[string]models.ProviderResponse
	Callback  string
}

func (s *Server) getAuthPage(c *gin.Context) {

	foundProviders := s.getAuthProvidersAsProviderResponse(nil)

	if len(foundProviders) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "No providers",
			fmt.Errorf("no authentication providers found. That provide authentication support"))
		return
	}

	config := s.GetConfig()

	callback, foundCallback := c.GetQuery("callback")

	if !foundCallback || len(callback) == 0 {
		callback = config.GetLocalServerUrl()
	}

	data := AuthPageData{
		TemplateData: s.GetTemplateData(c),
		Providers:    foundProviders,
		Callback:     callback,
	}

	s.renderHtml(c, "auth.html", data)
}

type AuthCallbackPageData struct {
	config.TemplateData
	Auth    models.AuthWrapper
	Session *models.LocalSession
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
	code := c.Query("code")

	session, err := provider.GetClient().CreateSession(context.TODO(), &models.AuthorizeUser{
		State:       state,
		Code:        code,
		RedirectUri: s.GetConfig().GetAuthCallbackUrl(auth.Provider),
	})

	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to create session", err)
		return
	}

	// Covert our sensitive session to one we can store on the users local system
	localSession := &models.LocalSession{
		Version: 1,
		Expiry:  session.Expiry.UTC(),
		Session: session.GetEncodedSession(
			s.Config.GetServices().GetEncryption(),
		),
	}

	data := AuthCallbackPageData{
		TemplateData: s.GetTemplateData(c),
		Auth:         auth,
		Session:      localSession,
	}

	cookie := sessions.Default(c)
	cookie.Set(auth.Provider, localSession.GetEncodedLocalSession())
	err = cookie.Save()

	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to set cookie", err)
		return
	}

	s.renderHtml(c, "auth_callback.html", data)
}

func (s *Server) getLogoutPage(c *gin.Context) {

	cookie := sessions.Default(c)
	cookie.Clear()
	err := cookie.Save()

	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to clear session", err)
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, "/")
}
