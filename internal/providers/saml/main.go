package saml

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
	"github.com/google/uuid"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
)

const SamlProviderName = "saml"

// samlProvider implements the ProviderImpl interface for SAML
type samlProvider struct {
	*models.BaseProvider
	middleware   *samlsp.Middleware
	idpMetadata  *saml.EntityDescriptor
	certificates []tls.Certificate
}

// SAMLConfig represents the SAML provider configuration
type SAMLConfig struct {
	IDPMetadataURL string          `yaml:"idp_metadata_url" json:"idp_metadata_url"`
	EntityID       string          `yaml:"entity_id" json:"entity_id"`
	RootURL        string          `yaml:"root_url" json:"root_url"`
	KeyPair        tls.Certificate `yaml:"-" json:"-"`
	SignRequests   bool            `yaml:"sign_requests" json:"sign_requests"`
}

func (p *samlProvider) Initialize(identifier string, provider models.Provider) error {
	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		SamlCapabilities,
	)

	// Parse SAML configuration from provider config
	config, err := p.parseSAMLConfig(provider.Config)
	if err != nil {
		return fmt.Errorf("failed to parse SAML config: %w", err)
	}

	// Load certificate and key for SAML signing
	keyPair := config.KeyPair

	var privateKey *rsa.PrivateKey
	if keyPair.PrivateKey != nil {
		if pk, ok := keyPair.PrivateKey.(*rsa.PrivateKey); ok {
			privateKey = pk
		}
	}

	// Fetch IdP metadata
	// Validate URL first
	if _, err := url.Parse(config.IDPMetadataURL); err != nil {
		return fmt.Errorf("invalid IdP metadata URL: %w", err)
	}

	// Fetch IdP metadata using common.InvokeHttpRequest
	resp, err := common.InvokeHttpRequest(&model.HTTPArguments{
		Method: http.MethodGet,
		Endpoint: &model.Endpoint{
			EndpointConfig: &model.EndpointConfiguration{
				URI: &model.LiteralUri{Value: config.IDPMetadataURL},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to fetch IdP metadata: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("failed to fetch IdP metadata: status code %d", resp.StatusCode())
	}

	idpMetadata := &saml.EntityDescriptor{}
	if err := xml.Unmarshal(resp.Body(), idpMetadata); err != nil {
		return fmt.Errorf("failed to parse IdP metadata: %w", err)
	}

	// Parse root URL
	rootURL, err := url.Parse(config.RootURL)
	if err != nil {
		return fmt.Errorf("invalid root URL: %w", err)
	}

	// Create SAML service provider with custom ACS URL
	// The ACS URL must match what's configured in Okta: /api/v1/auth/callback/{provider-name}
	acsURL := *rootURL
	acsURL.Path = fmt.Sprintf("/api/v1/auth/callback/%s", identifier)

	metadataURL := *rootURL
	metadataURL.Path = "/saml/metadata"

	// Create the ServiceProvider directly for more control
	sp := saml.ServiceProvider{
		EntityID:          config.EntityID,
		Key:               privateKey,
		Certificate:       keyPair.Leaf,
		MetadataURL:       metadataURL,
		AcsURL:            acsURL,
		IDPMetadata:       idpMetadata,
		AuthnNameIDFormat: saml.EmailAddressNameIDFormat,
		// Allow IDP-initiated flows (Okta can initiate)
		AllowIDPInitiated: true,
		// SignAuthnRequests: config.SignRequests,
	}

	// Create middleware wrapper
	samlSP := &samlsp.Middleware{
		ServiceProvider: sp,
	}

	p.middleware = samlSP
	p.idpMetadata = idpMetadata
	if keyPair.Certificate != nil {
		p.certificates = []tls.Certificate{keyPair}
	}

	logrus.WithFields(logrus.Fields{
		"provider":    provider.Name,
		"entityID":    samlSP.ServiceProvider.EntityID,
		"acsURL":      samlSP.ServiceProvider.AcsURL.String(),
		"metadataURL": samlSP.ServiceProvider.MetadataURL.String(),
		"idpIssuer":   idpMetadata.EntityID,
	}).Infof("SAML provider %s initialized successfully", provider.Name)
	return nil
}

func (p *samlProvider) AuthorizeSession(ctx context.Context, authRequest *models.AuthorizeUser) (*models.AuthorizeSessionResponse, error) {
	if p.middleware == nil {
		return nil, fmt.Errorf("SAML provider not initialized")
	}

	// Generate a SAML authentication request URL using the correct API
	// Use MakeRedirectAuthenticationRequest for redirect binding
	authURL, err := p.middleware.ServiceProvider.MakeRedirectAuthenticationRequest(authRequest.State)
	if err != nil {
		return nil, fmt.Errorf("failed to create SAML authentication request: %w", err)
	}

	logrus.Debugln("SAML auth request generated")

	return &models.AuthorizeSessionResponse{
		Url: authURL.String(),
	}, nil
}

func (p *samlProvider) CreateSession(ctx context.Context, authRequest *models.AuthorizeUser) (*models.Session, error) {

	if p.middleware == nil {
		return nil, fmt.Errorf("SAML provider not initialized")
	}

	if len(authRequest.Code) == 0 {
		return nil, fmt.Errorf("no SAML response provided")
	}

	// Log minimal debugging information without sensitive data
	logrus.WithFields(logrus.Fields{
		"entityID": p.middleware.ServiceProvider.EntityID,
		"acsURL":   p.middleware.ServiceProvider.AcsURL.String(),
	}).Debugln("Attempting to parse SAML response")

	// Parse the SAML response
	// IMPORTANT: The URL in the request must match the ACS URL for validation to pass
	// We need to use PostForm instead of Form for POST requests
	req := &http.Request{
		Method: "POST",
		URL:    &p.middleware.ServiceProvider.AcsURL,
		PostForm: url.Values{
			"SAMLResponse": {authRequest.Code},
		},
	}

	assertion, err := p.middleware.ServiceProvider.ParseResponse(
		req,
		[]string{authRequest.State},
	)

	if err != nil {
		// Log error without sensitive SAML response data
		errMsg := err.Error()
		errType := fmt.Sprintf("%T", err)

		// Check if it's an InvalidResponseError and try to extract more info
		var invalidErr *saml.InvalidResponseError
		if errors.As(err, &invalidErr) {
			logrus.WithFields(logrus.Fields{
				"error":     errMsg,
				"errorType": errType,
				"entityID":  p.middleware.ServiceProvider.EntityID,
				"acsURL":    p.middleware.ServiceProvider.AcsURL.String(),
			}).Errorln("Failed to parse SAML response")
		} else {
			logrus.WithFields(logrus.Fields{
				"error":     errMsg,
				"errorType": errType,
			}).Errorln("Failed to parse SAML response")
		}

		// InvalidResponseError typically means:
		// 1. Signature validation failed (most common)
		// 2. Time validation failed (NotBefore/NotOnOrAfter)
		// 3. Audience restriction mismatch

		return nil, fmt.Errorf("failed to parse SAML response: %w", err)
	}

	// Extract user information from SAML assertion
	var userID string
	var username string
	var email string
	var name string
	var groups []string

	// Extract attributes from the assertion
	if assertion != nil {
		// Get NameID (usually the username or email)
		if assertion.Subject != nil && assertion.Subject.NameID != nil {
			nameID := assertion.Subject.NameID.Value
			// Use NameID as email if it looks like an email
			if strings.Contains(nameID, "@") {
				email = nameID
				username = common.ExtractUsernameFromEmail(nameID)
			} else {
				username = nameID
			}
		}

		// Extract attributes
		for _, stmt := range assertion.AttributeStatements {
			for _, attr := range stmt.Attributes {
				switch attr.Name {
				case "email", "Email", "emailAddress", "mail":
					if len(attr.Values) > 0 {
						email = attr.Values[0].Value
					}
				case "name", "displayName", "Name", "cn", "commonName":
					if len(attr.Values) > 0 {
						name = attr.Values[0].Value
					}
				case "username", "Username", "sAMAccountName":
					if len(attr.Values) > 0 {
						username = attr.Values[0].Value
					}
				case "userid", "UserID", "uid", "objectGUID":
					if len(attr.Values) > 0 {
						userID = attr.Values[0].Value
					}
				case "groups", "Groups", "memberOf":
					for _, v := range attr.Values {
						groups = append(groups, v.Value)
					}
				}
			}
		}
	}

	if len(email) == 0 {
		return nil, fmt.Errorf("missing required user attributes in SAML assertion")
	}

	if len(userID) == 0 {
		userID = email
	}

	// Create user identity
	user := &models.User{
		ID:       userID,
		Username: username,
		Email:    email,
		Name:     name,
		Source:   "saml",
		Groups:   groups,
	}

	// Create session
	session := &models.Session{
		UUID:   uuid.New(),
		User:   user,
		Expiry: time.Now().Add(24 * time.Hour),
	}

	// Log session creation without PII details
	logrus.WithFields(logrus.Fields{
		"sessionUUID": session.UUID.String(),
		"source":      "saml",
	}).Info("Created SAML session successfully")

	return session, nil
}

func (p *samlProvider) ValidateSession(ctx context.Context, session *models.Session) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	// Check if session has expired
	if time.Now().After(session.Expiry) {
		return fmt.Errorf("session has expired")
	}

	// Validate user information
	if session.User == nil {
		return fmt.Errorf("session user is nil")
	}

	logrus.Debugf("SAML session validated for user: %s", session.User.Username)
	return nil
}

func (p *samlProvider) RenewSession(ctx context.Context, session *models.Session) (*models.Session, error) {
	if session == nil {
		return nil, fmt.Errorf("session is nil")
	}

	// Validate current session first
	if err := p.ValidateSession(ctx, session); err != nil {
		// If session is expired, we need a new authentication
		return nil, fmt.Errorf("cannot renew expired session: %w", err)
	}

	// Create a new session with extended expiry
	newSession := &models.Session{
		UUID:   uuid.New(),
		User:   session.User,
		Expiry: time.Now().Add(24 * time.Hour),
	}

	logrus.Infof("Renewed SAML session for user: %s", session.User.Username)
	return newSession, nil
}

// parseSAMLConfig parses the SAML configuration from the provider config
func (p *samlProvider) parseSAMLConfig(config *models.BasicConfig) (*SAMLConfig, error) {
	if config == nil {
		return nil, fmt.Errorf("config is nil")
	}

	samlConfig := &SAMLConfig{}

	// Parse required fields
	if idpURL, ok := config.GetString("idp_metadata_url"); ok {
		samlConfig.IDPMetadataURL = idpURL
	} else {
		return nil, fmt.Errorf("idp_metadata_url is required")
	}

	if entityID, ok := config.GetString("entity_id"); ok {
		samlConfig.EntityID = entityID
	} else {
		return nil, fmt.Errorf("entity_id is required")
	}

	if rootURL, ok := config.GetString("root_url"); ok {
		samlConfig.RootURL = rootURL
	} else {
		return nil, fmt.Errorf("root_url is required")
	}

	var certFile, cert string
	if v, ok := config.GetString("cert_file"); ok {
		certFile = v
	}
	if v, ok := config.GetString("cert"); ok {
		cert = v
	}

	var keyFile, key string
	if v, ok := config.GetString("key_file"); ok {
		keyFile = v
	}
	if v, ok := config.GetString("key"); ok {
		key = v
	}

	var keyPair tls.Certificate
	var err error

	if len(cert) != 0 {
		if len(key) != 0 {
			keyPair, err = tls.X509KeyPair([]byte(cert), []byte(key))
			if err != nil {
				return nil, fmt.Errorf("failed to parse SAML certificate from config: %w", err)
			}
		} else {
			// Certificate provided without a key. This is valid only for cases where signing and decryption are not required.
			logrus.Warn("SAML certificate provided without a private key. Signing and decryption will be unavailable.")
			block, _ := pem.Decode([]byte(cert))
			if block == nil {
				return nil, fmt.Errorf("failed to parse certificate PEM")
			}
			keyPair = tls.Certificate{
				Certificate: [][]byte{block.Bytes},
			}
		}
	} else if len(certFile) != 0 {
		if len(keyFile) != 0 {
			keyPair, err = tls.LoadX509KeyPair(certFile, keyFile)
			if err != nil {
				return nil, fmt.Errorf("failed to load SAML certificate: %w", err)
			}
		} else {
			// Certificate provided without a key. This is valid only for cases where signing and decryption are not required.
			logrus.Warn("SAML certificate provided without a private key. Signing and decryption will be unavailable.")
			certBytes, err := os.ReadFile(certFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read certificate file: %w", err)
			}
			block, _ := pem.Decode(certBytes)
			if block == nil {
				return nil, fmt.Errorf("failed to parse certificate PEM from file")
			}
			keyPair = tls.Certificate{
				Certificate: [][]byte{block.Bytes},
			}
		}
	}

	if len(keyPair.Certificate) > 0 {
		// Parse the certificate leaf
		keyPair.Leaf, err = x509.ParseCertificate(keyPair.Certificate[0])
		if err != nil {
			return nil, fmt.Errorf("failed to parse SAML certificate leaf: %w", err)
		}
		samlConfig.KeyPair = keyPair
	}

	// Parse optional fields
	if signRequests, ok := config.GetBool("sign_requests"); ok {
		samlConfig.SignRequests = signRequests
	} else {
		samlConfig.SignRequests = false // Default to false
	}

	// Validation: If signing is enabled, we MUST have a private key
	if samlConfig.SignRequests && samlConfig.KeyPair.PrivateKey == nil {
		return nil, fmt.Errorf("sign_requests is set to true, but no private key was provided (cert/key or cert_file/key_file)")
	}

	return samlConfig, nil
}

func init() {
	providers.Register(SamlProviderName, &samlProvider{})
}
