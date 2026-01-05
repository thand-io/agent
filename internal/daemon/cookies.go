package daemon

import (
	"encoding/base64"
	"fmt"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

var ThandCookieName = "_thand_v1"
var ThandCookieAttributeSessionName = "session"
var ThandCookieAttributeActiveName = "active"

func (s *Server) setAuthCookie(c *gin.Context, authProvider string, localSession *models.LocalSession) error {

	// Copy the session and remove the endpoint data as we don't want to store that in the cookie
	// This reduces the size of the cookie significantly
	copiedLocalSession := localSession.CopyWithoutEndpoint()

	// Strip the local session to clear endpoint data for cookie
	getEncodedCookie := copiedLocalSession.GetEncodedLocalSession()

	if len(getEncodedCookie) == 0 {
		logrus.WithFields(logrus.Fields{
			"provider": authProvider,
		}).Errorln("Failed to encode local session for cookie")
		return fmt.Errorf("failed to encode local session for cookie")
	}

	// Check cookie size limit (generally 4096 bytes)
	// Leaving some buffer for cookie name and other attributes
	if len(getEncodedCookie) > 4000 {
		logrus.WithFields(logrus.Fields{
			"provider": authProvider,
		}).Errorln("Encoded session size exceeds cookie limit")
		return fmt.Errorf("encoded session size exceeds cookie limit")
	}

	providerCookie := sessions.DefaultMany(c, CreateCookieName(authProvider))
	providerCookie.Set(ThandCookieAttributeSessionName, getEncodedCookie)
	err := providerCookie.Save()

	if err != nil {
		logrus.WithFields(logrus.Fields{
			"provider": authProvider,
		}).WithError(err).Errorln("Failed to save auth cookie")
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
