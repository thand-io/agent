package daemon

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/thand-io/agent/internal/config"
)

func TestGetElevationPagePrefillIncludesDevice(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		"GET",
		"/elevate/static?provider=local-elevation&role=local_sudo&device=device-alpha&duration=1h&reason=test",
		nil,
	)

	server := NewServer(config.DefaultConfig())
	data := server.getElevationPagePrefill(ctx)

	assert.Equal(t, []string{"local-elevation"}, data.Providers)
	assert.Equal(t, []string{"local_sudo"}, data.Roles)
	assert.Equal(t, "device-alpha", data.Device)
	assert.Equal(t, "1h", data.Duration)
	assert.Equal(t, "test", data.Reason)
}
