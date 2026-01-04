package gcpiap

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
)

// Calling ?gcp-iap-mode=CLEAR_LOGIN_COOKIE to the webhook will clear the login cookie

const GcpIAPProviderName = "gcp.iap"
const WebhookResourceIAP = "iap"

const (
	// IAPJWTHeader is the header that IAP adds to requests
	IAPJWTHeader = "X-Goog-IAP-JWT-Assertion"

	// IAPIssuer is the expected issuer for IAP JWTs
	IAPIssuer = "https://cloud.google.com/iap"
)

// IAPClaims represents the claims in an IAP JWT
type IAPClaims struct {
	Email        string `json:"email"`
	Subject      string `json:"sub"`
	HostedDomain string `json:"hd,omitempty"`
	GoogleClaims string `json:"google,omitempty"`
	GCIP         string `json:"gcip,omitempty"` // For external identities
}

// IAPGoogleClaims represents the nested Google claims in an IAP JWT
type IAPGoogleClaims struct {
	AccessLevels []string `json:"access_levels,omitempty"`
	DeviceID     string   `json:"device_id,omitempty"`
}

// gcpIAPProvider implements the ProviderImpl interface for GCP IAP
type gcpIAPProvider struct {
	*models.BaseProvider
	audience          string // Audience for validating incoming JWTs (e.g., /projects/NUM/apps/ID)
	oauthClientId     string // OAuth 2.0 client ID for programmatic access (e.g., XXX.apps.googleusercontent.com)
	oauthClientSecret string // OAuth client secret for token exchange
}

func (p *gcpIAPProvider) Initialize(identifier string, provider models.Provider) error {
	logrus.WithFields(logrus.Fields{
		"identifier": identifier,
		"type":       GcpIAPProviderName,
	}).Debugln("Starting provider initialization")

	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		GcpIAPCapabilities,
	)

	// Get the IAP audience from config
	gcpIAPConfig := p.GetConfig()

	// Log available config keys for debugging
	configMap := gcpIAPConfig.AsMap()
	configKeys := make([]string, 0, len(configMap))
	for k := range configMap {
		configKeys = append(configKeys, k)
	}
	logrus.WithField("config_keys", configKeys).Debugln("Loaded configuration")

	audience, foundAudience := gcpIAPConfig.GetString("audience")

	if !foundAudience || len(audience) == 0 {
		logrus.WithField("identifier", identifier).Errorln("Missing audience in configuration")
		return fmt.Errorf("audience must be set in the config for GCP IAP provider")
	}

	p.audience = audience

	// OAuth client ID for programmatic access (optional)
	oauthClientId, _ := gcpIAPConfig.GetString("oauth_client_id")
	p.oauthClientId = oauthClientId

	oauthClientSecret, _ := gcpIAPConfig.GetString("oauth_client_secret")
	p.oauthClientSecret = oauthClientSecret

	logrus.WithFields(logrus.Fields{
		"provider":         identifier,
		"audience":         audience,
		"oauth_client_id":  oauthClientId,
		"has_oauth_secret": len(oauthClientSecret) > 0,
	}).Infoln("Initialized GCP IAP provider")

	return nil
}

// AuthorizeSession is not applicable for IAP as authorization happens at the IAP layer
func (p *gcpIAPProvider) AuthorizeSession(ctx context.Context, auth *models.AuthorizeUser) (*models.AuthorizeSessionResponse, error) {

	// We don't yet have our user information as this is handled by IAP.
	// and we're not really supposed to have the user context at this stage.
	// so we need to hijack the redirect url. To our own webhook handler that
	// can extract the JWT and convert it into a session.

	logrus.Debugln("AuthorizeSession called - generating redirect URL to IAP webhook")

	newRedirectUri := strings.TrimSuffix(auth.RedirectUri, "/auth/callback/"+p.GetIdentifier())
	newRedirectUri += fmt.Sprintf(
		"/provider/%s/%s",
		url.PathEscape(p.GetIdentifier()),
		WebhookResourceIAP,
	)

	foundUrl, err := url.Parse(newRedirectUri)

	if err != nil {
		logrus.WithError(err).Errorln("Failed to parse redirect URI")
		return nil, fmt.Errorf("failed to parse redirect URI: %w", err)
	}

	// Override the path with our webhook endpoint
	query := foundUrl.Query()
	query.Add("state", auth.State)
	foundUrl.RawQuery = query.Encode()

	return &models.AuthorizeSessionResponse{
		Url: foundUrl.String(),
	}, nil
}

// HandleWebhook
func (p *gcpIAPProvider) HandleWebhook(ctx context.Context, req *models.WebhookRequest) error {

	if req == nil {
		logrus.Errorln("Missing webhook request")
		return fmt.Errorf("missing webhook request")
	}

	if req.Context == nil {
		logrus.Errorln("Missing request context in webhook")
		return fmt.Errorf("missing request context in webhook")
	}

	function := req.Context.Param("function")

	if len(function) == 0 {
		logrus.Errorln("Missing webhook function")
		return fmt.Errorf("missing webhook function")
	}

	if !strings.EqualFold(function, WebhookResourceIAP) {
		logrus.WithField("function", function).Errorln("Unsupported webhook function")
		return fmt.Errorf("unsupported webhook function: %s", function)
	}

	context := req.Context

	/*
		We implement a single webhook handler here to that is able to take in IAP JWTs
		and convert them into the request needed for the CreateSession handler after the
		callback
	*/

	sessionState := context.Query("state")

	if len(sessionState) == 0 {
		logrus.Errorln("Missing state parameter in webhook request")
		return fmt.Errorf("missing state parameter in webhook request")
	}

	// Extract the JWT token from the header
	jwtToken := context.GetHeader(IAPJWTHeader)

	if len(jwtToken) == 0 {
		logrus.WithField("header", IAPJWTHeader).Errorln("Missing IAP JWT token in request")
		return fmt.Errorf("missing IAP JWT token in header: %s", IAPJWTHeader)
	}

	// Create our new URL redirect request with the JWT as the code
	redirectUrl, err := url.Parse(fmt.Sprintf(
		"%s/%s",
		strings.TrimSuffix(req.Endpoint, "/"),
		strings.TrimPrefix(fmt.Sprintf("/auth/callback/%s", url.PathEscape(p.GetIdentifier())), "/"),
	))

	if err != nil {
		logrus.WithError(err).Errorln("Failed to parse redirect URL")
		return fmt.Errorf("failed to parse redirect URL: %w", err)
	}

	// State is already captured in the query parameters
	// Just need to add the JWT token. Taken from the header
	query := redirectUrl.Query()
	query.Add("code", jwtToken)
	query.Add("state", sessionState)
	redirectUrl.RawQuery = query.Encode()

	context.Redirect(
		http.StatusFound,
		redirectUrl.String(),
	)

	return nil

}

// CreateSession creates a session from an IAP JWT token
func (p *gcpIAPProvider) CreateSession(ctx context.Context, auth *models.AuthorizeUser) (*models.Session, error) {
	logrus.WithFields(logrus.Fields{
		"code_length": len(auth.Code),
	}).Debugln("CreateSession called")

	// Extract JWT from the authorization code (in this case, the JWT token itself)
	jwtToken := auth.Code
	if len(jwtToken) == 0 {
		logrus.Errorln("Missing JWT token in authorization code")
		return nil, fmt.Errorf("no JWT token provided in authorization code")
	}

	// Verify and parse the JWT
	claims, expiryTime, err := p.VerifyIAPJWT(ctx, jwtToken)
	if err != nil {
		logrus.WithError(err).Errorln("JWT verification failed")
		return nil, fmt.Errorf("failed to verify IAP JWT: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"email":   claims.Email,
		"subject": claims.Subject,
	}).Debugln("JWT verified, converting to session")

	// Convert claims to session
	session := p.ConvertIAPClaimsToSession(ctx, claims, expiryTime)

	// If OAuth is configured, exchange for a user ID token for programmatic access
	if len(auth.State) > 0 && len(p.oauthClientId) > 0 {
		logrus.Debugln("OAuth configured - generating user ID token for programmatic IAP access")

		// For programmatic IAP access, generate an ID token with the OAuth client ID as audience
		// This token can be used by the client to make authenticated requests to IAP
		identityToken, err := p.GetIdentityToken(ctx, p.oauthClientId)
		if err != nil {
			logrus.WithError(err).Errorln("Failed to generate OAuth ID token")
			return nil, fmt.Errorf("failed to generate OAuth ID token: %w", err)
		}

		// This token has the OAuth client ID as the audience
		// The client can use this to authenticate to IAP-protected resources
		session.AccessToken = identityToken.AccessToken
		session.RefreshToken = identityToken.RefreshToken
		session.Expiry = identityToken.Expiry

		logrus.WithFields(logrus.Fields{
			"user_email":     session.User.Email,
			"token_expiry":   session.Expiry.Format(time.RFC3339),
			"oauth_audience": p.oauthClientId,
		}).Infoln("Created session with OAuth ID token for programmatic access")
	} else if len(auth.State) > 0 {
		// No OAuth configured - store the IAP JWT (server-side only)
		logrus.Warnln("No OAuth client configured - using IAP JWT (server-side validation only)")
		session.AccessToken = jwtToken
		session.Expiry = expiryTime
	}

	logrus.WithFields(logrus.Fields{
		"user_email":   session.User.Email,
		"user_id":      session.User.ID,
		"session_id":   session.UUID,
		"expiry":       session.Expiry.Format(time.RFC3339),
		"access_token": len(session.AccessToken),
	}).Debugln("Session created from claims")

	// Add user to identities pool
	p.AddIdentities(models.Identity{
		ID:    session.User.ID,
		Label: session.User.Name,
		User:  session.User,
	})

	logrus.WithField("user_id", session.User.ID).Debugln("User added to identity pool")

	return session, nil
}

// GetIdentityToken generates an identity token for authenticating to IAP
// https://docs.cloud.google.com/iap/docs/authentication-howto#iap_make_request-go
func (p *gcpIAPProvider) GetIdentityToken(ctx context.Context, oauthAudience string) (*oauth2.Token, error) {

	logrus.WithField("audience", oauthAudience).Debugln("Generating identity token")

	if len(oauthAudience) == 0 {
		logrus.Errorln("Token audience is not configured")
		return nil, fmt.Errorf("token audience is not configured")
	}

	// Generate an identity token with the specified audience
	// For Cloud Run: use the Cloud Run service URL
	// For IAP: use the OAuth 2.0 client ID
	// This uses Application Default Credentials
	tokenSource, err := idtoken.NewTokenSource(
		ctx,
		// This needs to be the login server URL: https://docs.cloud.google.com/iap/docs/authentication-howto
		oauthAudience,
	)
	if err != nil {
		logrus.WithError(err).Errorln("Failed to create token source")
		return nil, fmt.Errorf("failed to create token source: %w", err)
	}

	idToken, err := tokenSource.Token()
	if err != nil {
		logrus.WithError(err).Errorln("Failed to get identity token")
		return nil, fmt.Errorf("failed to get identity token: %w", err)
	}

	payload, err := idtoken.Validate(ctx, idToken.AccessToken, oauthAudience)

	if err != nil {
		logrus.WithError(err).Errorln("Failed to validate generated identity token")
		return nil, fmt.Errorf("failed to validate generated identity token: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"issuer":  payload.Issuer,
		"subject": payload.Subject,
		"expires": payload.Expires,
		"issued":  payload.IssuedAt,
	}).Debugln("Generated identity token validated successfully")

	return idToken, nil
}

// ValidateSession validates an existing IAP session
func (p *gcpIAPProvider) ValidateSession(ctx context.Context, session *models.Session) error {
	// Check if session has expired
	if time.Now().After(session.Expiry) {
		return fmt.Errorf("session has expired")
	}

	// TODO(hugh): VerifyIAPJWT can be called here if we have the JWT token stored
	return nil
}

// RenewSession attempts to renew an IAP session
func (p *gcpIAPProvider) RenewSession(ctx context.Context, session *models.Session) (*models.Session, error) {
	// IAP sessions cannot be renewed programmatically
	// The user must go through IAP again to get a new token
	return nil, fmt.Errorf("GCP IAP sessions cannot be renewed - user must re-authenticate through IAP")
}

// VerifyIAPJWT verifies an IAP JWT token and returns the claims and expiry time
func (p *gcpIAPProvider) VerifyIAPJWT(ctx context.Context, tokenString string) (*IAPClaims, time.Time, error) {
	logrus.WithFields(logrus.Fields{
		"audience":     p.audience,
		"token_length": len(tokenString),
	}).Debugln("Starting JWT verification")

	// Use Google's idtoken package for verification
	// This automatically fetches and caches public keys from Google
	logrus.Debugln("Calling idtoken.Validate")
	payload, err := idtoken.Validate(ctx, tokenString, p.audience)
	if err != nil {
		logrus.WithError(err).WithField("audience", p.audience).Errorln("idtoken.Validate failed")
		return nil, time.Time{}, fmt.Errorf("failed to validate IAP JWT: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"issuer":  payload.Issuer,
		"subject": payload.Subject,
		"expires": payload.Expires,
		"issued":  payload.IssuedAt,
	}).Debugln("Token validated by idtoken library")

	// Verify the issuer
	if payload.Issuer != IAPIssuer {
		logrus.WithFields(logrus.Fields{
			"expected": IAPIssuer,
			"actual":   payload.Issuer,
		}).Errorln("Invalid issuer")
		return nil, time.Time{}, fmt.Errorf("invalid issuer: expected %s, got %s", IAPIssuer, payload.Issuer)
	}

	logrus.WithField("issuer", payload.Issuer).Debugln("Issuer verified")

	// Parse the payload into our claims structure
	claims := &IAPClaims{}

	// Marshal and unmarshal to convert map to struct
	logrus.WithField("claims_count", len(payload.Claims)).Debugln("Parsing payload claims")
	payloadBytes, err := json.Marshal(payload.Claims)
	if err != nil {
		logrus.WithError(err).Errorln("Failed to marshal payload")
		return nil, time.Time{}, fmt.Errorf("failed to marshal payload: %w", err)
	}

	if err := json.Unmarshal(payloadBytes, claims); err != nil {
		logrus.WithError(err).Errorln("Failed to unmarshal claims")
		return nil, time.Time{}, fmt.Errorf("failed to unmarshal claims: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"email":   claims.Email,
		"subject": claims.Subject,
		"domain":  claims.HostedDomain,
	}).Debugln("Claims parsed successfully")

	// Verify expiration (with 30 second skew tolerance as per IAP docs)
	expiryTime := time.Unix(payload.Expires, 0)
	issuedTime := time.Unix(payload.IssuedAt, 0)
	now := time.Now()

	logrus.WithFields(logrus.Fields{
		"now":            now.Format(time.RFC3339),
		"issued_at":      issuedTime.Format(time.RFC3339),
		"expires_at":     expiryTime.Format(time.RFC3339),
		"seconds_old":    now.Sub(issuedTime).Seconds(),
		"seconds_left":   expiryTime.Sub(now).Seconds(),
		"skew_tolerance": 30,
	}).Debugln("Checking token timing")

	if now.After(expiryTime.Add(30 * time.Second)) {
		logrus.WithFields(logrus.Fields{
			"expired_at": expiryTime.Format(time.RFC3339),
			"now":        now.Format(time.RFC3339),
		}).Errorln("Token has expired")
		return nil, time.Time{}, fmt.Errorf("token has expired")
	}

	logrus.WithFields(logrus.Fields{
		"email":   claims.Email,
		"subject": claims.Subject,
		"hd":      claims.HostedDomain,
	}).Infoln("JWT verification successful")

	return claims, expiryTime, nil
}

// ConvertIAPClaimsToSession converts IAP JWT claims into a session
func (p *gcpIAPProvider) ConvertIAPClaimsToSession(ctx context.Context, claims *IAPClaims, expiryTime time.Time) *models.Session {
	logrus.WithFields(logrus.Fields{
		"subject": claims.Subject,
		"email":   claims.Email,
		"expiry":  expiryTime.Format(time.RFC3339),
	}).Debugln("Converting JWT claims to session")

	// Extract user information from claims
	verified := true
	user := &models.User{
		ID:       claims.Subject,
		Email:    claims.Email,
		Verified: &verified, // IAP has already verified the user
		Source:   GcpIAPProviderName,
	}

	logrus.WithFields(logrus.Fields{
		"user_id":    user.ID,
		"user_email": user.Email,
		"verified":   *user.Verified,
	}).Debugln("Created user object")

	// Try to extract name and username from email
	if len(claims.Email) != 0 {
		// Simple name extraction from email
		user.Name = common.ExtractNameFromEmail(claims.Email)
		user.Username = common.ExtractUsernameFromEmail(claims.Email)
	}

	// Parse Google claims if present
	if len(claims.GoogleClaims) != 0 {
		logrus.Debugln("Parsing Google claims")
		var googleClaims IAPGoogleClaims
		if err := json.Unmarshal([]byte(claims.GoogleClaims), &googleClaims); err == nil {
			logrus.WithField("access_levels", googleClaims.AccessLevels).
				Debugln("IAP access levels found")
		} else {
			logrus.WithError(err).Warnln("Failed to parse Google claims")
		}
	}

	// Check for hosted domain
	if len(claims.HostedDomain) != 0 {
		logrus.WithField("domain", claims.HostedDomain).Debugln("User has hosted domain")
	}

	// Create a session using the JWT's actual expiry time
	sessionUUID := uuid.New()

	logrus.WithFields(logrus.Fields{
		"session_id": sessionUUID,
		"expiry":     expiryTime.Format(time.RFC3339),
	}).Debugln("Creating session object")

	session := &models.Session{
		UUID:   sessionUUID,
		User:   user,
		Expiry: expiryTime, // Use JWT's actual expiry time
	}

	logrus.WithFields(logrus.Fields{
		"user_email": user.Email,
		"user_id":    user.ID,
		"session_id": session.UUID,
		"expiry":     session.Expiry.Format(time.RFC3339),
	}).Infoln("Session created from IAP claims")

	return session
}

// GetAudience returns the configured IAP audience
func (p *gcpIAPProvider) GetAudience() string {
	return p.audience
}

func init() {
	providers.Register(GcpIAPProviderName, &gcpIAPProvider{})
}
