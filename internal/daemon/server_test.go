package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/config"
)

func TestAPIConfigurationHandlerPrefersRequestOriginInServerMode(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	cfg := config.DefaultConfig()
	cfg.SetMode(config.ModeServer)
	require.NoError(t, cfg.SetLoginServer("http://localhost:5225"))

	server := NewServer(cfg)
	router := gin.New()
	router.GET("/.well-known/api-configuration", server.apiConfigurationHandler)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/api-configuration", nil)
	req.Host = "thand.test:5225"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var response struct {
		BaseURL string `json:"baseUrl"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Equal(t, "http://thand.test:5225", response.BaseURL)
}
