package config

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverLoginServerApiUrl_LogsLoginServerDiscovery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/.well-known/api-configuration", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"baseUrl":"https://auth.example.com","apiBasePath":"/api/v1"}`))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	hook := logrustest.NewGlobal()
	defer hook.Reset()

	oldLevel := logrus.GetLevel()
	logrus.SetLevel(logrus.DebugLevel)
	defer logrus.SetLevel(oldLevel)

	config := &Config{}

	apiURL := config.DiscoverLoginServerApiUrl(server.URL)

	require.Equal(t, "https://auth.example.com/api/v1", apiURL)
	lastEntry := hook.LastEntry()
	require.NotNil(t, lastEntry)
	assert.Equal(t, "Discovered login server base URL: https://auth.example.com", lastEntry.Message)
}

func TestDiscoverThandServerApiUrl_LogsThandServerDiscovery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/.well-known/api-configuration", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"baseUrl":"https://config.example.com","apiBasePath":"/api/v1"}`))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	hook := logrustest.NewGlobal()
	defer hook.Reset()

	oldLevel := logrus.GetLevel()
	logrus.SetLevel(logrus.DebugLevel)
	defer logrus.SetLevel(oldLevel)

	config := &Config{}
	config.Thand.Endpoint = server.URL
	config.Thand.ApiKey = "test-token"

	apiURL := config.DiscoverThandServerApiUrl()

	require.Equal(t, "https://config.example.com/api/v1", apiURL)
	lastEntry := hook.LastEntry()
	require.NotNil(t, lastEntry)
	assert.Equal(t, "Discovered Thand server base URL: https://config.example.com", lastEntry.Message)
}
