package daemon

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	ginsessions "github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/securecookie"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	sessionManager "github.com/thand-io/agent/internal/sessions"
)

const (
	testThandCookieV1 = ThandLegacyCookieName
	testThandCookieV2 = ThandCookieName
)

func TestAuthCookieLargeProviderSessionRoundTripUsesV2Shards(t *testing.T) {
	provider := "oauth2-large"
	server := newTestCookieServer(t, config.ModeServer, provider)
	localSession := newLocalSessionForSignedCookieRange(
		t,
		provider,
		server.Config.GetSecret(),
		providerCookieChunkSize+1,
		providerCookieChunkSize*providerCookieMaxShards,
	)

	writeResp := performCookieHandlerRequest(
		t,
		server,
		http.MethodGet,
		"/auth-cookie",
		"/auth-cookie",
		[]string{provider},
		nil,
		func(c *gin.Context) {
			if err := server.setAuthCookie(c, provider, localSession); err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return
			}
			c.Status(http.StatusNoContent)
		},
	)

	require.Equal(t, http.StatusNoContent, writeResp.Code, writeResp.Body.String())

	responseCookies := mergeCookies(nil, writeResp.Result().Cookies())
	assertCookiePresent(t, responseCookies, CreateCookieName(provider))
	assertCookiePresent(t, responseCookies, CreateCookieName(provider)+"C1")

	expectedSession, err := localSession.GetDecodedSession(newMockEncryptor())
	require.NoError(t, err)

	foundSessions := map[string]*models.Session{}
	readResp := performCookieHandlerRequest(
		t,
		server,
		http.MethodGet,
		"/auth-cookie",
		"/auth-cookie",
		[]string{provider},
		responseCookies,
		func(c *gin.Context) {
			server.processProviderCookies(c, newMockEncryptor(), foundSessions)
			c.Status(http.StatusNoContent)
		},
	)

	require.Equal(t, http.StatusNoContent, readResp.Code)
	require.Contains(t, foundSessions, provider)
	assertSessionsEqual(t, expectedSession.Session, foundSessions[provider])
}

func TestAuthCookieCleansStaleV2ShardsWhenSessionShrinks(t *testing.T) {
	provider := "oauth2-shrink"
	server := newTestCookieServer(t, config.ModeServer, provider)

	largeSession := newLocalSessionForExactShardCount(
		t,
		provider,
		server.Config.GetSecret(),
		providerCookieMaxShards,
	)
	smallSession := newTestLocalSession(provider, 32)

	firstResp := performCookieHandlerRequest(
		t,
		server,
		http.MethodGet,
		"/auth-cookie",
		"/auth-cookie",
		[]string{provider},
		nil,
		func(c *gin.Context) {
			if err := server.setAuthCookie(c, provider, largeSession); err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return
			}
			c.Status(http.StatusNoContent)
		},
	)

	require.Equal(t, http.StatusNoContent, firstResp.Code, firstResp.Body.String())

	secondResp := performCookieHandlerRequest(
		t,
		server,
		http.MethodGet,
		"/auth-cookie",
		"/auth-cookie",
		[]string{provider},
		mergeCookies(nil, firstResp.Result().Cookies()),
		func(c *gin.Context) {
			if err := server.setAuthCookie(c, provider, smallSession); err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return
			}
			c.Status(http.StatusNoContent)
		},
	)

	require.Equal(t, http.StatusNoContent, secondResp.Code, secondResp.Body.String())

	responseCookies := secondResp.Result().Cookies()
	assertNoExpiredCookieHeaders(t, responseCookies, CreateCookieName(provider))
	for shard := 1; shard <= providerCookieMaxShards; shard++ {
		assertExpiredCookieCount(t, responseCookies, fmt.Sprintf("%sC%d", CreateCookieName(provider), shard), 1)
	}
}

func TestAuthCookieRewritingSameWidthShardSetDoesNotExpireRewrittenNames(t *testing.T) {
	provider := "oauth2-rewrite"
	server := newTestCookieServer(t, config.ModeServer, provider)

	firstSession := newLocalSessionForExactShardCount(
		t,
		provider,
		server.Config.GetSecret(),
		2,
	)
	secondSession := newLocalSessionForExactShardCount(
		t,
		provider,
		server.Config.GetSecret(),
		2,
	)

	firstResp := performCookieHandlerRequest(
		t,
		server,
		http.MethodGet,
		"/auth-cookie",
		"/auth-cookie",
		[]string{provider},
		nil,
		func(c *gin.Context) {
			if err := server.setAuthCookie(c, provider, firstSession); err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return
			}
			c.Status(http.StatusNoContent)
		},
	)

	require.Equal(t, http.StatusNoContent, firstResp.Code, firstResp.Body.String())

	secondResp := performCookieHandlerRequest(
		t,
		server,
		http.MethodGet,
		"/auth-cookie",
		"/auth-cookie",
		[]string{provider},
		mergeCookies(nil, firstResp.Result().Cookies()),
		func(c *gin.Context) {
			if err := server.setAuthCookie(c, provider, secondSession); err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return
			}
			c.Status(http.StatusNoContent)
		},
	)

	require.Equal(t, http.StatusNoContent, secondResp.Code, secondResp.Body.String())

	responseCookies := secondResp.Result().Cookies()
	assertNoExpiredCookieHeaders(
		t,
		responseCookies,
		CreateCookieName(provider),
		CreateCookieName(provider)+"C1",
		CreateCookieName(provider)+"C2",
	)
	assertCookieCount(t, responseCookies, CreateCookieName(provider), 1)
	assertCookieCount(t, responseCookies, CreateCookieName(provider)+"C1", 1)
	assertCookieCount(t, responseCookies, CreateCookieName(provider)+"C2", 1)
}

func TestProviderCookieReassemblesV2ShardedSession(t *testing.T) {
	provider := "oauth2-v2"
	server := newTestCookieServer(t, config.ModeServer, provider)
	localSession := newLocalSessionForSignedCookieRange(
		t,
		provider,
		server.Config.GetSecret(),
		providerCookieChunkSize+1,
		providerCookieChunkSize*providerCookieMaxShards,
	)

	v2CookieName := createVersionedCookieName(testThandCookieV2, provider)
	signedValue := encodeProviderCookieValue(t, server.Config.GetSecret(), v2CookieName, localSession)
	requestCookies := shardProviderCookieValue(t, v2CookieName, signedValue)

	expectedSession, err := localSession.GetDecodedSession(newMockEncryptor())
	require.NoError(t, err)

	foundSessions := map[string]*models.Session{}
	resp := performCookieHandlerRequest(
		t,
		server,
		http.MethodGet,
		"/provider-cookie",
		"/provider-cookie",
		[]string{provider},
		requestCookies,
		func(c *gin.Context) {
			server.processProviderCookies(c, newMockEncryptor(), foundSessions)
			c.Status(http.StatusNoContent)
		},
	)

	require.Equal(t, http.StatusNoContent, resp.Code)
	require.Contains(t, foundSessions, provider)
	assertSessionsEqual(t, expectedSession.Session, foundSessions[provider])
}

func TestProviderCookieFallsBackToLegacyV1WhenV2Absent(t *testing.T) {
	provider := "oauth2-v1-fallback"
	server := newTestCookieServer(t, config.ModeServer, provider)
	localSession := newTestLocalSession(provider, 64)

	expectedSession, err := localSession.GetDecodedSession(newMockEncryptor())
	require.NoError(t, err)

	legacyCookie := &http.Cookie{
		Name:  createVersionedCookieName(testThandCookieV1, provider),
		Value: encodeLegacyProviderCookieValue(t, server.Config.GetSecret(), provider, localSession),
	}

	foundSessions := map[string]*models.Session{}
	resp := performCookieHandlerRequest(
		t,
		server,
		http.MethodGet,
		"/provider-cookie",
		"/provider-cookie",
		[]string{provider},
		[]*http.Cookie{legacyCookie},
		func(c *gin.Context) {
			server.processProviderCookies(c, newMockEncryptor(), foundSessions)
			c.Status(http.StatusNoContent)
		},
	)

	require.Equal(t, http.StatusNoContent, resp.Code)
	require.Contains(t, foundSessions, provider)
	assertSessionsEqual(t, expectedSession.Session, foundSessions[provider])
}

func TestProviderCookieRejectsPartialV2AndDoesNotFallbackToV1(t *testing.T) {
	provider := "oauth2-partial"
	server := newTestCookieServer(t, config.ModeServer, provider)

	legacySession := newTestLocalSession(provider, 64)
	legacyCookie := &http.Cookie{
		Name:  createVersionedCookieName(testThandCookieV1, provider),
		Value: encodeLegacyProviderCookieValue(t, server.Config.GetSecret(), provider, legacySession),
	}

	partialSession := newLocalSessionForSignedCookieRange(
		t,
		provider,
		server.Config.GetSecret(),
		providerCookieChunkSize+1,
		providerCookieChunkSize*providerCookieMaxShards,
	)
	v2CookieName := createVersionedCookieName(testThandCookieV2, provider)
	partialValue := encodeProviderCookieValue(t, server.Config.GetSecret(), v2CookieName, partialSession)
	partialCookies := shardProviderCookieValue(t, v2CookieName, partialValue)
	require.GreaterOrEqual(t, len(partialCookies), 2)

	requestCookies := append([]*http.Cookie{
		partialCookies[0],
		partialCookies[1],
		legacyCookie,
	})

	foundSessions := map[string]*models.Session{}
	resp := performCookieHandlerRequest(
		t,
		server,
		http.MethodGet,
		"/provider-cookie",
		"/provider-cookie",
		[]string{provider},
		requestCookies,
		func(c *gin.Context) {
			server.processProviderCookies(c, newMockEncryptor(), foundSessions)
			c.Status(http.StatusNoContent)
		},
	)

	require.Equal(t, http.StatusNoContent, resp.Code)
	assert.NotContains(t, foundSessions, provider)

	responseCookies := cookiesByName(resp.Result().Cookies())
	assertExpiredCookie(t, responseCookies, v2CookieName)
	assertExpiredCookie(t, responseCookies, v2CookieName+"C1")
}

func TestProviderCookieRejectsOversizedUnshardedV2BaseCookie(t *testing.T) {
	provider := "oauth2-oversized-base"
	server := newTestCookieServer(t, config.ModeServer, provider)
	v2CookieName := createVersionedCookieName(testThandCookieV2, provider)

	requestCookies := []*http.Cookie{
		{
			Name:  v2CookieName,
			Value: repeatedToken("oversized-base", providerCookieChunkSize+1),
		},
	}

	foundSessions := map[string]*models.Session{}
	resp := performCookieHandlerRequest(
		t,
		server,
		http.MethodGet,
		"/provider-cookie",
		"/provider-cookie",
		[]string{provider},
		requestCookies,
		func(c *gin.Context) {
			server.processProviderCookies(c, newMockEncryptor(), foundSessions)
			c.Status(http.StatusNoContent)
		},
	)

	require.Equal(t, http.StatusNoContent, resp.Code)
	assert.NotContains(t, foundSessions, provider)

	responseCookies := cookiesByName(resp.Result().Cookies())
	assertExpiredCookie(t, responseCookies, v2CookieName)
}

func TestProviderCookieRejectsOversizedV2Shard(t *testing.T) {
	provider := "oauth2-oversized-shard"
	server := newTestCookieServer(t, config.ModeServer, provider)
	v2CookieName := createVersionedCookieName(testThandCookieV2, provider)

	requestCookies := []*http.Cookie{
		{
			Name:  v2CookieName,
			Value: "chunks-1",
		},
		{
			Name:  v2CookieName + "C1",
			Value: repeatedToken("oversized-shard", providerCookieChunkSize+1),
		},
	}

	foundSessions := map[string]*models.Session{}
	resp := performCookieHandlerRequest(
		t,
		server,
		http.MethodGet,
		"/provider-cookie",
		"/provider-cookie",
		[]string{provider},
		requestCookies,
		func(c *gin.Context) {
			server.processProviderCookies(c, newMockEncryptor(), foundSessions)
			c.Status(http.StatusNoContent)
		},
	)

	require.Equal(t, http.StatusNoContent, resp.Code)
	assert.NotContains(t, foundSessions, provider)

	responseCookies := cookiesByName(resp.Result().Cookies())
	assertExpiredCookie(t, responseCookies, v2CookieName)
	assertExpiredCookie(t, responseCookies, v2CookieName+"C1")
}

func TestProviderCookieRejectsReassembledPayloadOverBoundedBudget(t *testing.T) {
	provider := "oauth2-oversized-total"
	server := newTestCookieServer(t, config.ModeServer, provider)
	v2CookieName := createVersionedCookieName(testThandCookieV2, provider)

	requestCookies := []*http.Cookie{
		{
			Name:  v2CookieName,
			Value: fmt.Sprintf("chunks-%d", providerCookieMaxShards),
		},
		{
			Name:  v2CookieName + "C1",
			Value: repeatedToken("oversized-total-1", providerCookieChunkSize),
		},
		{
			Name:  v2CookieName + "C2",
			Value: repeatedToken("oversized-total-2", providerCookieChunkSize),
		},
		{
			Name:  v2CookieName + "C3",
			Value: repeatedToken("oversized-total-3", providerCookieChunkSize+1),
		},
	}

	foundSessions := map[string]*models.Session{}
	resp := performCookieHandlerRequest(
		t,
		server,
		http.MethodGet,
		"/provider-cookie",
		"/provider-cookie",
		[]string{provider},
		requestCookies,
		func(c *gin.Context) {
			server.processProviderCookies(c, newMockEncryptor(), foundSessions)
			c.Status(http.StatusNoContent)
		},
	)

	require.Equal(t, http.StatusNoContent, resp.Code)
	assert.NotContains(t, foundSessions, provider)

	responseCookies := cookiesByName(resp.Result().Cookies())
	assertExpiredCookie(t, responseCookies, v2CookieName)
	assertExpiredCookie(t, responseCookies, v2CookieName+"C1")
	assertExpiredCookie(t, responseCookies, v2CookieName+"C2")
	assertExpiredCookie(t, responseCookies, v2CookieName+"C3")
}

func TestReadCurrentProviderCookieValueRejectsOversizedUnshardedBase(t *testing.T) {
	provider := "oauth2-read-oversized-base"
	server := newTestCookieServer(t, config.ModeServer, provider)
	v2CookieName := createVersionedCookieName(testThandCookieV2, provider)

	req := httptest.NewRequest(http.MethodGet, "/provider-cookie", nil)
	req.AddCookie(&http.Cookie{
		Name:  v2CookieName,
		Value: repeatedToken("oversized-base", providerCookieChunkSize+1),
	})

	value, found, err := server.readCurrentProviderCookieValue(req, v2CookieName)
	require.True(t, found)
	require.Error(t, err)
	assert.Empty(t, value)
	assert.Contains(t, err.Error(), "provider cookie base value exceeds shard limit")
}

func TestReadCurrentProviderCookieValueRejectsOversizedChunksPrefixedBase(t *testing.T) {
	provider := "oauth2-read-oversized-chunks-prefix"
	server := newTestCookieServer(t, config.ModeServer, provider)
	v2CookieName := createVersionedCookieName(testThandCookieV2, provider)

	req := httptest.NewRequest(http.MethodGet, "/provider-cookie", nil)
	req.AddCookie(&http.Cookie{
		Name:  v2CookieName,
		Value: providerCookieChunkPrefix + repeatedToken("1", providerCookieChunkSize),
	})

	value, found, err := server.readCurrentProviderCookieValue(req, v2CookieName)
	require.True(t, found)
	require.Error(t, err)
	assert.Empty(t, value)
	assert.Contains(t, err.Error(), "provider cookie base value exceeds shard limit")
}

func TestReadCurrentProviderCookieValueRejectsOversizedShard(t *testing.T) {
	provider := "oauth2-read-oversized-shard"
	server := newTestCookieServer(t, config.ModeServer, provider)
	v2CookieName := createVersionedCookieName(testThandCookieV2, provider)

	req := httptest.NewRequest(http.MethodGet, "/provider-cookie", nil)
	req.AddCookie(&http.Cookie{
		Name:  v2CookieName,
		Value: "chunks-1",
	})
	req.AddCookie(&http.Cookie{
		Name:  v2CookieName + "C1",
		Value: repeatedToken("oversized-shard", providerCookieChunkSize+1),
	})

	value, found, err := server.readCurrentProviderCookieValue(req, v2CookieName)
	require.True(t, found)
	require.Error(t, err)
	assert.Empty(t, value)
	assert.Contains(t, err.Error(), "provider cookie shard 1 exceeds shard limit")
}

func TestReadCurrentProviderCookieValueRejectsOversizedReassembledValue(t *testing.T) {
	provider := "oauth2-read-oversized-total"
	server := newTestCookieServer(t, config.ModeServer, provider)
	v2CookieName := createVersionedCookieName(testThandCookieV2, provider)

	req := httptest.NewRequest(http.MethodGet, "/provider-cookie", nil)
	req.AddCookie(&http.Cookie{
		Name:  v2CookieName,
		Value: fmt.Sprintf("chunks-%d", providerCookieMaxShards),
	})
	req.AddCookie(&http.Cookie{
		Name:  v2CookieName + "C1",
		Value: repeatedToken("oversized-total-1", providerCookieChunkSize),
	})
	req.AddCookie(&http.Cookie{
		Name:  v2CookieName + "C2",
		Value: repeatedToken("oversized-total-2", providerCookieChunkSize),
	})
	req.AddCookie(&http.Cookie{
		Name:  v2CookieName + "C3",
		Value: repeatedToken("oversized-total-3", providerCookieChunkSize+1),
	})

	value, found, err := server.readCurrentProviderCookieValue(req, v2CookieName)
	require.True(t, found)
	require.Error(t, err)
	assert.Empty(t, value)
	assert.Contains(t, err.Error(), "provider cookie shard 3 exceeds shard limit")
}

func TestDeleteSessionClearsLegacyAndV2ProviderCookies(t *testing.T) {
	provider := "oauth2-delete"
	server := newTestCookieServer(t, config.ModeServer, provider)
	localSession := newTestLocalSession(provider, 64)

	requestCookies := []*http.Cookie{
		{
			Name:  createVersionedCookieName(testThandCookieV1, provider),
			Value: encodeLegacyProviderCookieValue(t, server.Config.GetSecret(), provider, localSession),
		},
		{
			Name:  createVersionedCookieName(testThandCookieV2, provider),
			Value: encodeProviderCookieValue(t, server.Config.GetSecret(), createVersionedCookieName(testThandCookieV2, provider), localSession),
		},
		{
			Name:  testThandCookieV1,
			Value: encodeDefaultProviderCookieValue(t, server.Config.GetSecret(), testThandCookieV1, provider),
		},
		{
			Name:  testThandCookieV2,
			Value: encodeDefaultProviderCookieValue(t, server.Config.GetSecret(), testThandCookieV2, provider),
		},
	}

	resp := performCookieHandlerRequest(
		t,
		server,
		http.MethodDelete,
		"/session/:provider",
		fmt.Sprintf("/session/%s", provider),
		[]string{provider},
		requestCookies,
		func(c *gin.Context) {
			server.deleteSession(c)
		},
	)

	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	responseCookies := cookiesByName(resp.Result().Cookies())
	assertExpiredCookie(t, responseCookies, createVersionedCookieName(testThandCookieV1, provider))
	assertExpiredCookie(t, responseCookies, createVersionedCookieName(testThandCookieV2, provider))
	assertExpiredCookie(t, responseCookies, testThandCookieV1)
	assertExpiredCookie(t, responseCookies, testThandCookieV2)
}

func TestGetLogoutPageClearsLegacyAndV2Cookies(t *testing.T) {
	provider := "oauth2-logout"
	server := newTestCookieServer(t, config.ModeServer, provider)
	localSession := newTestLocalSession(provider, 64)

	requestCookies := []*http.Cookie{
		{
			Name:  createVersionedCookieName(testThandCookieV1, provider),
			Value: encodeLegacyProviderCookieValue(t, server.Config.GetSecret(), provider, localSession),
		},
		{
			Name:  createVersionedCookieName(testThandCookieV2, provider),
			Value: encodeProviderCookieValue(t, server.Config.GetSecret(), createVersionedCookieName(testThandCookieV2, provider), localSession),
		},
		{
			Name:  testThandCookieV1,
			Value: encodeDefaultProviderCookieValue(t, server.Config.GetSecret(), testThandCookieV1, provider),
		},
		{
			Name:  testThandCookieV2,
			Value: encodeDefaultProviderCookieValue(t, server.Config.GetSecret(), testThandCookieV2, provider),
		},
	}

	resp := performCookieHandlerRequest(
		t,
		server,
		http.MethodGet,
		"/auth/logout",
		"/auth/logout",
		[]string{provider},
		requestCookies,
		func(c *gin.Context) {
			server.getLogoutPage(c)
		},
	)

	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	responseCookies := cookiesByName(resp.Result().Cookies())
	assertExpiredCookie(t, responseCookies, createVersionedCookieName(testThandCookieV1, provider))
	assertExpiredCookie(t, responseCookies, createVersionedCookieName(testThandCookieV2, provider))
	assertExpiredCookie(t, responseCookies, testThandCookieV1)
	assertExpiredCookie(t, responseCookies, testThandCookieV2)
}

func TestHandleAgentModeMigratesLegacyV1CookieWithoutRedirectLoop(t *testing.T) {
	provider := "oauth2-agent"
	server := newTestCookieServer(t, config.ModeAgent)

	restoreSessionManagerPath := sessionManager.SESSION_MANAGER_PATH
	restoreServers := sessionManager.GetSessionManager().Servers
	sessionManager.SESSION_MANAGER_PATH = t.TempDir()
	sessionManager.GetSessionManager().Servers = make(map[string]sessionManager.LoginServer)
	t.Cleanup(func() {
		sessionManager.SESSION_MANAGER_PATH = restoreSessionManagerPath
		sessionManager.GetSessionManager().Servers = restoreServers
	})

	localSession := newTestLocalSession(provider, 64)
	err := sessionManager.GetSessionManager().AddSession(
		server.Config.GetLoginServerHostname(),
		provider,
		*localSession,
	)
	require.NoError(t, err)

	legacyCookie := &http.Cookie{
		Name:  createVersionedCookieName(testThandCookieV1, provider),
		Value: encodeLegacyProviderCookieValue(t, server.Config.GetSecret(), provider, localSession),
	}

	firstResp := performCookieHandlerRequest(
		t,
		server,
		http.MethodGet,
		"/agent-mode",
		"/agent-mode",
		[]string{provider},
		[]*http.Cookie{legacyCookie},
		func(c *gin.Context) {
			server.handleAgentMode(c)
			if !c.Writer.Written() {
				c.Status(http.StatusOK)
			}
		},
	)

	require.Equal(t, http.StatusTemporaryRedirect, firstResp.Code, firstResp.Body.String())
	require.Equal(t, "/agent-mode", firstResp.Header().Get("Location"))

	nextCookies := mergeCookies([]*http.Cookie{legacyCookie}, firstResp.Result().Cookies())
	secondResp := performCookieHandlerRequest(
		t,
		server,
		http.MethodGet,
		"/agent-mode",
		"/agent-mode",
		[]string{provider},
		nextCookies,
		func(c *gin.Context) {
			server.handleAgentMode(c)
			if !c.Writer.Written() {
				c.Status(http.StatusOK)
			}
		},
	)

	require.Equal(t, http.StatusOK, secondResp.Code, secondResp.Body.String())
}

func newTestCookieServer(t *testing.T, mode config.Mode, providers ...string) *Server {
	t.Helper()

	cfg := &config.Config{
		Secret: "test-secret-0123456789abcdef0123456789abcdef",
		Login: models.LoginConfig{
			Endpoint: model.NewEndpoint("https://login.example.com"),
		},
	}
	cfg.SetMode(mode)

	for _, providerName := range providers {
		provider := models.NewBaseProvider(
			providerName,
			models.ProviderConfig{
				Name:     providerName,
				Provider: "test",
				Config:   &models.BasicConfig{},
				Enabled:  true,
			},
			models.NewProviderCapabilities(),
		)
		provider.EnableCapability(models.ProviderCapabilityAuthorizer)
		cfg.AddProvider(providerName, provider)
	}

	return &Server{
		Config: cfg,
	}
}

func performCookieHandlerRequest(
	t *testing.T,
	server *Server,
	method string,
	routePath string,
	requestPath string,
	providers []string,
	requestCookies []*http.Cookie,
	handler gin.HandlerFunc,
) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()

	cookieNames := []string{ThandCookieName}
	for _, provider := range providers {
		cookieNames = append(cookieNames, CreateCookieName(provider))
	}

	router.Use(ginsessions.SessionsMany(
		cookieNames,
		server.getSessionStore(server.Config.GetSecret()),
	))
	router.Handle(method, routePath, handler)

	req := httptest.NewRequest(method, requestPath, nil)
	for _, cookie := range requestCookies {
		req.AddCookie(cookie)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func createVersionedCookieName(version, provider string) string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(provider))
	return fmt.Sprintf("%s_%s", version, encoded)
}

func newTestLocalSession(provider string, tokenLen int) *models.LocalSession {
	token := repeatedToken("id", tokenLen)
	accessToken := repeatedToken("access", tokenLen)
	refreshToken := repeatedToken("refresh", tokenLen)

	exportableSession := &models.ExportableSession{
		Session: &models.Session{
			UUID: uuid.New(),
			User: &models.User{
				ID:       "user-123",
				Username: "test-user",
				Email:    "test@example.com",
				Name:     "Test User",
				Source:   provider,
			},
			Token:        token,
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			Expiry:       time.Now().UTC().Add(4 * time.Hour),
		},
		Provider: provider,
	}

	return exportableSession.ToLocalSession(newMockEncryptor())
}

func newLocalSessionForSignedCookieRange(
	t *testing.T,
	provider string,
	secret string,
	minSignedLen int,
	maxSignedLen int,
) *models.LocalSession {
	t.Helper()

	cookieName := createVersionedCookieName(testThandCookieV2, provider)
	for tokenLen := 256; tokenLen <= 4096; tokenLen += 128 {
		localSession := newTestLocalSession(provider, tokenLen)
		signedValue := encodeProviderCookieValue(t, secret, cookieName, localSession)
		if len(signedValue) >= minSignedLen && len(signedValue) <= maxSignedLen {
			return localSession
		}
	}

	t.Fatalf("failed to create local session for signed cookie range %d-%d", minSignedLen, maxSignedLen)
	return nil
}

func newLocalSessionForExactShardCount(
	t *testing.T,
	provider string,
	secret string,
	shardCount int,
) *models.LocalSession {
	t.Helper()

	cookieName := createVersionedCookieName(testThandCookieV2, provider)
	for tokenLen := 256; tokenLen <= 4096; tokenLen += 128 {
		localSession := newTestLocalSession(provider, tokenLen)
		signedValue := encodeProviderCookieValue(t, secret, cookieName, localSession)
		actualShardCount := max(1, (len(signedValue)+providerCookieChunkSize-1)/providerCookieChunkSize)
		if actualShardCount == shardCount {
			return localSession
		}
	}

	t.Fatalf("failed to create local session for exact shard count %d", shardCount)
	return nil
}

func repeatedToken(prefix string, targetLen int) string {
	token := prefix
	for len(token) < targetLen {
		token += "-" + uuid.NewString()
	}
	return token[:targetLen]
}

func encodeProviderCookieValue(t *testing.T, secret string, cookieName string, localSession *models.LocalSession) string {
	t.Helper()

	codecs := secureCookieCodecs(secret, true)
	value, err := securecookie.EncodeMulti(cookieName, localSession.GetEncodedLocalSession(), codecs...)
	require.NoError(t, err)
	return value
}

func encodeLegacyProviderCookieValue(t *testing.T, secret string, provider string, localSession *models.LocalSession) string {
	t.Helper()

	legacyCookieName := createVersionedCookieName(testThandCookieV1, provider)
	codecs := secureCookieCodecs(secret, false)
	value, err := securecookie.EncodeMulti(
		legacyCookieName,
		map[interface{}]interface{}{
			ThandCookieAttributeSessionName: localSession.GetEncodedLocalSessionBytes(),
		},
		codecs...,
	)
	require.NoError(t, err)
	return value
}

func encodeDefaultProviderCookieValue(t *testing.T, secret string, cookieName string, provider string) string {
	t.Helper()

	codecs := secureCookieCodecs(secret, false)
	value, err := securecookie.EncodeMulti(
		cookieName,
		map[interface{}]interface{}{
			ThandCookieAttributeActiveName: provider,
		},
		codecs...,
	)
	require.NoError(t, err)
	return value
}

func secureCookieCodecs(secret string, unlimited bool) []securecookie.Codec {
	codecs := securecookie.CodecsFromPairs([]byte(secret))
	if unlimited {
		for _, codec := range codecs {
			if sc, ok := codec.(*securecookie.SecureCookie); ok {
				sc.MaxLength(0)
			}
		}
	}
	return codecs
}

func shardProviderCookieValue(t *testing.T, cookieName, value string) []*http.Cookie {
	t.Helper()

	if len(value) <= providerCookieChunkSize {
		return []*http.Cookie{
			{
				Name:  cookieName,
				Value: value,
				Path:  "/",
			},
		}
	}

	chunkCount := (len(value) + providerCookieChunkSize - 1) / providerCookieChunkSize
	require.LessOrEqual(t, chunkCount, providerCookieMaxShards)

	cookies := []*http.Cookie{
		{
			Name:  cookieName,
			Value: fmt.Sprintf("chunks-%d", chunkCount),
			Path:  "/",
		},
	}

	for index := 0; index < chunkCount; index++ {
		start := index * providerCookieChunkSize
		end := start + providerCookieChunkSize
		if end > len(value) {
			end = len(value)
		}

		cookies = append(cookies, &http.Cookie{
			Name:  fmt.Sprintf("%sC%d", cookieName, index+1),
			Value: value[start:end],
			Path:  "/",
		})
	}

	return cookies
}

func cookiesByName(cookies []*http.Cookie) map[string]*http.Cookie {
	result := make(map[string]*http.Cookie, len(cookies))
	for _, cookie := range cookies {
		result[cookie.Name] = cookie
	}
	return result
}

func findCookiesByName(cookies []*http.Cookie, name string) []*http.Cookie {
	result := []*http.Cookie{}
	for _, cookie := range cookies {
		if cookie.Name == name {
			result = append(result, cookie)
		}
	}
	return result
}

func assertCookiePresent(t *testing.T, cookies []*http.Cookie, name string) {
	t.Helper()
	byName := cookiesByName(cookies)
	if _, ok := byName[name]; !ok {
		t.Fatalf("expected cookie %s to be present", name)
	}
}

func isExpiredCookie(cookie *http.Cookie) bool {
	return cookie.MaxAge < 0 || (!cookie.Expires.IsZero() && cookie.Expires.Before(time.Now().Add(1*time.Minute)))
}

func assertExpiredCookie(t *testing.T, cookies map[string]*http.Cookie, name string) {
	t.Helper()

	cookie, ok := cookies[name]
	require.True(t, ok, "expected expired cookie %s to be set", name)
	assert.True(
		t,
		isExpiredCookie(cookie),
		"expected cookie %s to be expired, got MaxAge=%d Expires=%s",
		name,
		cookie.MaxAge,
		cookie.Expires,
	)
}

func assertCookieCount(t *testing.T, cookies []*http.Cookie, name string, count int) {
	t.Helper()
	assert.Len(t, findCookiesByName(cookies, name), count, "expected %d Set-Cookie headers for %s", count, name)
}

func assertExpiredCookieCount(t *testing.T, cookies []*http.Cookie, name string, count int) {
	t.Helper()

	expiredCount := 0
	for _, cookie := range findCookiesByName(cookies, name) {
		if isExpiredCookie(cookie) {
			expiredCount++
		}
	}

	assert.Equal(t, count, expiredCount, "expected %d expired Set-Cookie headers for %s", count, name)
}

func assertNoExpiredCookieHeaders(t *testing.T, cookies []*http.Cookie, names ...string) {
	t.Helper()

	for _, name := range names {
		assertExpiredCookieCount(t, cookies, name, 0)
	}
}

func mergeCookies(base []*http.Cookie, updates []*http.Cookie) []*http.Cookie {
	merged := make(map[string]*http.Cookie)
	for _, cookie := range base {
		merged[cookie.Name] = cookie
	}

	for _, cookie := range updates {
		if isExpiredCookie(cookie) {
			delete(merged, cookie.Name)
			continue
		}
		merged[cookie.Name] = cookie
	}

	result := make([]*http.Cookie, 0, len(merged))
	for _, cookie := range merged {
		result = append(result, cookie)
	}
	return result
}

func assertSessionsEqual(t *testing.T, expected *models.Session, actual *models.Session) {
	t.Helper()

	require.NotNil(t, expected)
	require.NotNil(t, actual)
	require.NotNil(t, expected.User)
	require.NotNil(t, actual.User)

	assert.Equal(t, expected.Token, actual.Token)
	assert.Equal(t, expected.AccessToken, actual.AccessToken)
	assert.Equal(t, expected.RefreshToken, actual.RefreshToken)
	assert.Equal(t, expected.User.ID, actual.User.ID)
	assert.Equal(t, expected.User.Email, actual.User.Email)
	assert.Equal(t, expected.User.Name, actual.User.Name)
	assert.True(t, expected.Expiry.Equal(actual.Expiry))
}
