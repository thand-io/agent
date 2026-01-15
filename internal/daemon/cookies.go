package daemon

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"golang.org/x/net/publicsuffix"
)

var ThandCookieName = "_thand_v1"
var ThandCookieAttributeSessionName = "session"
var ThandCookieAttributeActiveName = "active"

func (s *Server) setAuthCookie(c *gin.Context, authProvider string, localSession *models.LocalSession) error {

	// Copy the session and remove the endpoint data as we don't want to store that in the cookie
	// This reduces the size of the cookie significantly
	copiedLocalSession := localSession.CopyWithoutEndpoint()

	// Strip the local session to clear endpoint data for cookie
	getEncodedCookie := copiedLocalSession.GetEncodedLocalSessionBytes()

	if len(getEncodedCookie) == 0 {
		logrus.WithFields(logrus.Fields{
			"provider": authProvider,
		}).Errorln("Failed to encode local session for cookie")
		return fmt.Errorf("failed to encode local session for cookie")
	}

	// Check cookie size limit (generally 4096 bytes)
	// getEncodedCookie is raw bytes. Session store will base64 encode it (x1.33).
	// 2800 * 1.33 = 3733 bytes. Plus overhead (HMAC etc), it should fit in 4096.
	if len(getEncodedCookie) > 2800 {
		logrus.WithFields(logrus.Fields{
			"provider": authProvider,
			"encoded":  len(getEncodedCookie),
		}).Errorln("Encoded session size exceeds cookie limit")
		return fmt.Errorf("encoded session size exceeds cookie limit")
	}

	providerCookie := sessions.DefaultMany(c, CreateCookieName(authProvider))
	providerCookie.Set(ThandCookieAttributeSessionName, getEncodedCookie)
	err := providerCookie.Save()

	if err != nil {
		logrus.WithFields(logrus.Fields{
			"provider": authProvider,
			"encoded":  len(getEncodedCookie),
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

// getSessionStore creates a session store with secure cookie settings
func (s *Server) getSessionStore(secret string) sessions.Store {

	hostname := s.Config.GetLoginServerHostname()
	domain := getCookieDomain(hostname)

	store := cookie.NewStore([]byte(secret))
	store.Options(sessions.Options{
		Path:     "/",
		Domain:   domain,
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
		Secure:   true,                 // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode, // Needed for OAuth2 redirects
	})
	return store
}

// getCookieDomain determines the appropriate cookie domain based on the hostname.
// Uses publicsuffix library to properly handle multi-level TLDs like azurecontainerapps.io
func getCookieDomain(hostname string) string {
	// For localhost or empty hostname, use host-only cookies (most secure)
	if hostname == "" || hostname == "localhost" || strings.HasPrefix(hostname, "127.") {
		return ""
	}

	// Get the effective TLD+1 (registrable domain)
	// This handles complex TLDs like azurecontainerapps.io properly
	eTLDPlusOne, err := publicsuffix.EffectiveTLDPlusOne(hostname)
	if err != nil {
		// If we can't determine the public suffix, fall back to host-only cookies for safety
		logrus.WithFields(logrus.Fields{
			"hostname": hostname,
			"error":    err,
		}).Debugln("Failed to determine public suffix for cookie domain, using host-only cookies")
		return ""
	}

	// For multi-subdomain setups (e.g., thand.livelysand-xxx.eastus.azurecontainerapps.io),
	// the eTLD+1 would be eastus.azurecontainerapps.io, which is what we want.
	// However, if hostname == eTLD+1, it means we're at the base domain already,
	// so we should use host-only cookies instead of adding a dot prefix
	if hostname == eTLDPlusOne {
		// At base domain, use host-only cookies (no Domain attribute)
		return ""
	}

	// For subdomains, set cookie domain with leading dot to share across subdomains
	return "." + eTLDPlusOne
}
