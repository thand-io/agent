package daemon

import (
	"encoding/base64"
	"fmt"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/thand-io/agent/internal/models"
)

var ThandCookieName = "_thand_v1"
var ThandCookieAttributeSessionName = "session"
var ThandCookieAttributeActiveName = "active"

func (s *Server) setAuthCookie(c *gin.Context, authProvider string, localSession *models.LocalSession) error {

	providerCookie := sessions.DefaultMany(c, CreateCookieName(authProvider))
	providerCookie.Set(ThandCookieAttributeSessionName, localSession.GetEncodedLocalSession())
	err := providerCookie.Save()

	if err != nil {
		return fmt.Errorf("failed to save auth cookie: %v", err)
	}

	// Set the active provider in the main thand cookie
	defaultCookie := sessions.DefaultMany(c, ThandCookieName)
	defaultCookie.Set(ThandCookieAttributeActiveName, authProvider)
	err = defaultCookie.Save()

	return err

}

func CreateCookieName(provider string) string {
	// base64 encode the provider name to ensure it's safe for cookie names, omitting padding
	encoded := base64.RawURLEncoding.EncodeToString([]byte(provider))
	// prepend the thand cookie name
	return fmt.Sprintf("%s_%s", ThandCookieName, encoded)
}
