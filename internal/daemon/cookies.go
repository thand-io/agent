package daemon

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/securecookie"
	gsessions "github.com/gorilla/sessions"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"golang.org/x/net/publicsuffix"
)

// Provider session cookies can exceed the practical per-cookie size limit once
// the encoded LocalSession is wrapped by securecookie, especially for OAuth2
// providers with large access or refresh tokens. The v2 cookie transport keeps
// the small "active provider" cookie on gin sessions, but stores provider
// session state in a dedicated cookie format that signs the full payload once
// and then shards that signed value across multiple cookies when needed.
//
// Constraints and rollout rules:
//   - Sharding is only a browser transport detail; the LocalSession wire format
//     stays unchanged.
//   - Each shard targets about 3500 bytes of cookie value to leave headroom for
//     cookie metadata and intermediary limits.
//   - A provider cookie may use at most 3 shards. Larger signed payloads are
//     rejected instead of allowing unbounded header growth.
//   - Sharding works around single-cookie limits, but not total request-header
//     limits enforced by browsers, proxies, or ingress layers. The shard cap is
//     the explicit budget for one provider session.
//   - The shard cap is part of the v2 transport contract. Changing the writer
//     to emit more shards without first upgrading all readers can cause older
//     nodes to reject and clear newer cookies, so incompatible shard-budget
//     changes should use a new cookie version.
//   - During rollout we write only v2, read v2 first, and fall back to v1 only
//     when v2 is absent. Partial or corrupt v2 state is treated as invalid and
//     cleared rather than silently downgraded.
const (
	ThandLegacyCookieName           = "_thand_v1"
	ThandCookieName                 = "_thand_v2"
	ThandCookieAttributeSessionName = "session"
	ThandCookieAttributeActiveName  = "active"
	providerCookieChunkPrefix       = "chunks-"
	providerCookieChunkSize         = 3500 // Leave headroom under practical per-cookie limits.
	providerCookieMaxShards         = 3    // Bound aggregate header growth; changing this is a v2 wire-contract change.
	providerCookieMaxValueSize      = providerCookieChunkSize * providerCookieMaxShards
)

type providerCookieVersion string

const (
	providerCookieVersionCurrent providerCookieVersion = "v2"
	providerCookieVersionLegacy  providerCookieVersion = "v1"
)

func (s *Server) setAuthCookie(c *gin.Context, authProvider string, localSession *models.LocalSession) error {
	if err := s.writeProviderCookie(c, authProvider, localSession); err != nil {
		return err
	}

	return s.setDefaultProviderCookie(c, authProvider)
}

func CreateCookieName(provider string) string {
	return createCookieNameWithBase(ThandCookieName, provider)
}

func CreateLegacyCookieName(provider string) string {
	return createCookieNameWithBase(ThandLegacyCookieName, provider)
}

func createCookieNameWithBase(baseName string, provider string) string {
	// base64 encode the provider name to ensure it's safe for cookie names, omitting padding
	encoded := base64.RawURLEncoding.EncodeToString([]byte(provider))
	// prepend the thand cookie name
	return fmt.Sprintf("%s_%s", baseName, encoded)
}

func (s *Server) getCookieOptions() *sessions.Options {
	hostname := s.Config.GetLoginServerHostname()
	domain := getCookieDomain(hostname)

	return &sessions.Options{
		Path:     "/",
		Domain:   domain,
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
		Secure:   true,                 // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode, // Needed for OAuth2 redirects
	}
}

// getSessionStore creates a session store with secure cookie settings
func (s *Server) getSessionStore(secret string) sessions.Store {
	store := cookie.NewStore([]byte(secret))
	store.Options(*s.getCookieOptions())
	return store
}

func (s *Server) getLegacySessionStore(secret string) *gsessions.CookieStore {
	store := gsessions.NewCookieStore([]byte(secret))
	options := s.getCookieOptions()
	store.Options = &gsessions.Options{
		Path:     options.Path,
		Domain:   options.Domain,
		MaxAge:   options.MaxAge,
		Secure:   options.Secure,
		HttpOnly: options.HttpOnly,
		SameSite: options.SameSite,
	}
	store.MaxAge(options.MaxAge)
	return store
}

// getProviderCookieCodecs returns securecookie codecs for the v2 provider-cookie
// transport. MaxLength is disabled here because size enforcement is handled by
// the explicit shard budget in writeProviderCookie.
func (s *Server) getProviderCookieCodecs(secret string) []securecookie.Codec {
	codecs := securecookie.CodecsFromPairs([]byte(secret))
	options := s.getCookieOptions()
	for _, codec := range codecs {
		if sc, ok := codec.(*securecookie.SecureCookie); ok {
			sc.MaxLength(0)
			sc.MaxAge(options.MaxAge)
		}
	}
	return codecs
}

func getSessionValueAsString(cookie sessions.Session, key string) string {
	if cookie == nil {
		return ""
	}

	value := cookie.Get(key)
	if value == nil {
		return ""
	}

	if provider, ok := value.(string); ok {
		return provider
	}

	return ""
}

// getDefaultProviderCookieValue returns the active provider name from the small
// default-provider cookie. It prefers the current v2 cookie and falls back to
// the legacy v1 cookie during rollout.
func (s *Server) getDefaultProviderCookieValue(c *gin.Context) string {
	if provider := getSessionValueAsString(sessions.DefaultMany(c, ThandCookieName), ThandCookieAttributeActiveName); len(provider) > 0 {
		return provider
	}

	return getSessionValueAsString(sessions.DefaultMany(c, ThandLegacyCookieName), ThandCookieAttributeActiveName)
}

// setDefaultProviderCookie records the active provider name in the current v2
// default-provider cookie and clears the legacy v1 default cookie so subsequent
// reads converge on the new format.
func (s *Server) setDefaultProviderCookie(c *gin.Context, provider string) error {
	defaultCookie := sessions.DefaultMany(c, ThandCookieName)
	if defaultCookie == nil {
		return fmt.Errorf("default provider cookie session not initialized")
	}

	defaultCookie.Set(ThandCookieAttributeActiveName, provider)
	if err := defaultCookie.Save(); err != nil {
		return err
	}

	s.clearCookieByName(c, ThandLegacyCookieName)
	return nil
}

func (s *Server) clearDefaultProviderCookies(c *gin.Context) {
	s.clearCookieByName(c, ThandCookieName)
	s.clearCookieByName(c, ThandLegacyCookieName)
}

// writeProviderCookie writes the provider-specific LocalSession using the v2
// transport format. The LocalSession payload is signed once, then stored either
// directly in the base cookie or split across numbered shard cookies when the
// signed value exceeds providerCookieChunkSize.
func (s *Server) writeProviderCookie(c *gin.Context, provider string, localSession *models.LocalSession) error {
	if localSession == nil {
		return fmt.Errorf("local session is nil")
	}

	// Copy the session and remove the endpoint data as we don't want to store that in the cookie.
	// This keeps the browser transport smaller without changing the LocalSession wire format.
	copiedLocalSession := localSession.CopyWithoutEndpoint()
	encodedLocalSession := copiedLocalSession.GetEncodedLocalSession()

	if len(encodedLocalSession) == 0 {
		logrus.WithFields(logrus.Fields{
			"provider": provider,
		}).Errorln("Failed to encode local session for cookie")
		return fmt.Errorf("failed to encode local session for cookie")
	}

	cookieName := CreateCookieName(provider)
	signedValue, err := securecookie.EncodeMulti(
		cookieName,
		encodedLocalSession,
		s.getProviderCookieCodecs(s.Config.GetSecret())...,
	)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"provider": provider,
		}).WithError(err).Errorln("Failed to sign provider session cookie")
		return fmt.Errorf("failed to encode provider session cookie: %w", err)
	}

	chunkCount := (len(signedValue) + providerCookieChunkSize - 1) / providerCookieChunkSize
	if chunkCount > providerCookieMaxShards {
		logrus.WithFields(logrus.Fields{
			"provider":   provider,
			"cookieName": cookieName,
			"encoded":    len(signedValue),
			"shards":     chunkCount,
		}).Errorln("Signed session size exceeds bounded cookie/header budget")
		return fmt.Errorf("signed provider session exceeds bounded cookie/header budget")
	}

	writtenCookieNames := map[string]struct{}{
		cookieName: {},
	}

	if chunkCount <= 1 {
		s.setCookieValue(c, cookieName, signedValue)
	} else {
		s.setCookieValue(c, cookieName, fmt.Sprintf("%s%d", providerCookieChunkPrefix, chunkCount))

		for shard := 0; shard < chunkCount; shard++ {
			start := shard * providerCookieChunkSize
			end := start + providerCookieChunkSize
			if end > len(signedValue) {
				end = len(signedValue)
			}

			s.setCookieValue(
				c,
				fmt.Sprintf("%sC%d", cookieName, shard+1),
				signedValue[start:end],
			)
			writtenCookieNames[fmt.Sprintf("%sC%d", cookieName, shard+1)] = struct{}{}
		}
	}

	s.clearStaleCookieSetFromRequest(c, cookieName, writtenCookieNames)
	s.clearCookieByName(c, CreateLegacyCookieName(provider))
	return nil
}

// readProviderLocalSession loads a provider cookie during the v1->v2 rollout.
// It returns the decoded LocalSession, the cookie version used to satisfy the
// read, whether any cookie state was present for that provider, and an error if
// the discovered cookie state was invalid. A corrupt or partial v2 cookie set is
// treated as found=true with an error so callers can fail closed and avoid
// silently falling back to legacy state.
func (s *Server) readProviderLocalSession(
	c *gin.Context,
	provider string,
) (*models.LocalSession, providerCookieVersion, bool, error) {
	currentCookieName := CreateCookieName(provider)
	signedValue, found, err := s.readCurrentProviderCookieValue(c.Request, currentCookieName)
	if found {
		if err != nil {
			s.clearCookieSetFromRequest(c, currentCookieName)
			return nil, providerCookieVersionCurrent, true, err
		}

		localSession, err := s.decodeCurrentProviderCookieValue(currentCookieName, signedValue)
		if err != nil {
			s.clearCookieSetFromRequest(c, currentCookieName)
			return nil, providerCookieVersionCurrent, true, err
		}

		return localSession, providerCookieVersionCurrent, true, nil
	}

	legacySession, found, err := s.readLegacyProviderLocalSession(c.Request, provider)
	if err != nil {
		return nil, providerCookieVersionLegacy, true, err
	}

	return legacySession, providerCookieVersionLegacy, found, nil
}

// readCurrentProviderCookieValue returns the signed v2 provider cookie payload.
// For unsharded cookies it returns the base cookie value directly. For sharded
// cookies it reassembles the numbered shard cookies named <cookieName>C1..CN.
// The bool result reports whether any v2 cookie state was present at all.
func (s *Server) readCurrentProviderCookieValue(
	r *http.Request,
	cookieName string,
) (string, bool, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie == nil || len(cookie.Value) == 0 {
		return "", false, nil
	}

	if len(cookie.Value) > providerCookieChunkSize {
		return "", true, fmt.Errorf("provider cookie base value exceeds shard limit")
	}

	if !strings.HasPrefix(cookie.Value, providerCookieChunkPrefix) {
		return cookie.Value, true, nil
	}

	shardCount, err := strconv.Atoi(strings.TrimPrefix(cookie.Value, providerCookieChunkPrefix))
	if err != nil {
		return cookie.Value, true, nil
	}
	if shardCount <= 0 || shardCount > providerCookieMaxShards {
		return "", true, fmt.Errorf("invalid provider cookie shard count")
	}

	var builder strings.Builder
	for shard := 1; shard <= shardCount; shard++ {
		chunkCookie, chunkErr := r.Cookie(fmt.Sprintf("%sC%d", cookieName, shard))
		if chunkErr != nil || chunkCookie == nil || len(chunkCookie.Value) == 0 {
			return "", true, fmt.Errorf("provider cookie shard %d missing", shard)
		}
		if len(chunkCookie.Value) > providerCookieChunkSize {
			return "", true, fmt.Errorf("provider cookie shard %d exceeds shard limit", shard)
		}

		builder.WriteString(chunkCookie.Value)
		if builder.Len() > providerCookieMaxValueSize {
			return "", true, fmt.Errorf("provider cookie value exceeds bounded shard budget")
		}
	}

	return builder.String(), true, nil
}

// decodeCurrentProviderCookieValue verifies and decodes a signed v2 provider
// cookie value back into a LocalSession. The signedValue input must be the full
// reassembled securecookie payload for cookieName.
func (s *Server) decodeCurrentProviderCookieValue(cookieName string, signedValue string) (*models.LocalSession, error) {
	var encodedLocalSession string
	if err := securecookie.DecodeMulti(
		cookieName,
		signedValue,
		&encodedLocalSession,
		s.getProviderCookieCodecs(s.Config.GetSecret())...,
	); err != nil {
		return nil, fmt.Errorf("failed to decode signed provider cookie: %w", err)
	}

	localSession, err := models.DecodedLocalSession(encodedLocalSession)
	if err != nil {
		return nil, fmt.Errorf("failed to decode provider local session: %w", err)
	}

	return localSession, nil
}

// readLegacyProviderLocalSession loads the legacy v1 gorilla session cookie for
// provider. It returns found=false when the legacy cookie is absent, and found=true
// with an error when the cookie exists but cannot be decoded.
func (s *Server) readLegacyProviderLocalSession(
	r *http.Request,
	provider string,
) (*models.LocalSession, bool, error) {
	cookieName := CreateLegacyCookieName(provider)
	if _, err := r.Cookie(cookieName); err != nil {
		return nil, false, nil
	}

	legacyStore := s.getLegacySessionStore(s.Config.GetSecret())
	legacySession, err := legacyStore.Get(r, cookieName)
	if err != nil {
		return nil, true, fmt.Errorf("failed to decode legacy provider cookie: %w", err)
	}

	sessionData := legacySession.Values[ThandCookieAttributeSessionName]
	if sessionData == nil {
		return nil, true, fmt.Errorf("legacy provider cookie missing session data")
	}

	localSession, err := getLocalSessionFromCookieData(sessionData)
	if err != nil {
		return nil, true, err
	}

	return localSession, true, nil
}

// getLocalSessionFromCookieData normalizes legacy session payloads that may be
// stored as either a string or raw bytes before decoding them into LocalSession.
func getLocalSessionFromCookieData(sessionData any) (*models.LocalSession, error) {
	switch v := sessionData.(type) {
	case string:
		return models.DecodedLocalSession(v)
	case []byte:
		return models.DecodedLocalSessionBytes(v)
	default:
		return nil, fmt.Errorf("invalid session data type: %T", sessionData)
	}
}

func (s *Server) clearProviderCookies(c *gin.Context, provider string) {
	s.clearCookieSetFromRequest(c, CreateCookieName(provider))
	s.clearCookieByName(c, CreateLegacyCookieName(provider))
}

// clearStaleCookieSetFromRequest expires request cookies in the provider cookie
// set that are not being rewritten in the current response. This avoids sending
// redundant expire+rewrite headers for the same cookie name.
func (s *Server) clearStaleCookieSetFromRequest(c *gin.Context, baseCookieName string, keepNames map[string]struct{}) {
	names := map[string]struct{}{}

	for _, cookie := range c.Request.Cookies() {
		if cookie == nil {
			continue
		}

		if cookie.Name != baseCookieName && !strings.HasPrefix(cookie.Name, baseCookieName+"C") {
			continue
		}

		if _, keep := keepNames[cookie.Name]; keep {
			continue
		}

		names[cookie.Name] = struct{}{}
	}

	for name := range names {
		s.clearCookieByName(c, name)
	}
}

// clearCookieSetFromRequest expires the named base cookie plus any shard cookies
// observed on the incoming request. This lets rewrites shrink a sharded cookie
// set without leaving stale shard cookies behind.
func (s *Server) clearCookieSetFromRequest(c *gin.Context, baseCookieName string) {
	names := map[string]struct{}{
		baseCookieName: {},
	}

	for _, cookie := range c.Request.Cookies() {
		if cookie == nil {
			continue
		}

		if cookie.Name == baseCookieName || strings.HasPrefix(cookie.Name, baseCookieName+"C") {
			names[cookie.Name] = struct{}{}
		}
	}

	for name := range names {
		s.clearCookieByName(c, name)
	}
}

func (s *Server) setCookieValue(c *gin.Context, name string, value string) {
	options := s.getCookieOptions()
	cookie := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     options.Path,
		Domain:   options.Domain,
		MaxAge:   options.MaxAge,
		Expires:  time.Now().Add(time.Duration(options.MaxAge) * time.Second),
		HttpOnly: options.HttpOnly,
		Secure:   options.Secure,
		SameSite: options.SameSite,
	}
	http.SetCookie(c.Writer, cookie)
}

func (s *Server) clearCookieByName(c *gin.Context, name string) {
	options := s.getCookieOptions()
	cookie := &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     options.Path,
		Domain:   options.Domain,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: options.HttpOnly,
		Secure:   options.Secure,
		SameSite: options.SameSite,
	}
	http.SetCookie(c.Writer, cookie)
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
