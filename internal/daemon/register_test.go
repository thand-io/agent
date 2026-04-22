package daemon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
)

func TestPostRegisterReturnsConfigurationDefinitions(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Roles: config.RoleConfig{
			Definitions: map[string]models.Role{
				"viewer": {
					Name:        "Viewer",
					Description: "Read-only access",
					Enabled:     true,
				},
			},
		},
		Workflows: config.WorkflowConfig{
			Definitions: map[string]models.Workflow{
				"approval": {
					Name:        "Approval",
					Description: "Approval workflow",
					Enabled:     true,
					Workflow: &model.Workflow{
						Do: &model.TaskList{},
					},
				},
			},
		},
		Providers: config.ProviderDefinitionsConfig{
			Definitions: map[string]models.ProviderConfig{
				"oauth2-directory": {
					Name:        "Directory Login",
					Description: "Remote OAuth2 provider",
					Provider:    "oauth2",
					Enabled:     true,
				},
			},
		},
	}

	server := NewServer(cfg)
	router := gin.New()
	router.POST("/register", server.postRegister)

	body, err := json.Marshal(config.RegistrationRequest{
		Identifier: uuid.New(),
		Environment: &models.EnvironmentConfig{
			Name:     "device-alpha",
			Hostname: "device-alpha.example.test",
			Platform: models.Local,
		},
	})
	if err != nil {
		t.Fatalf("Marshal registration request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp config.RegistrationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal response: %v", err)
	}

	if resp.Providers == nil || resp.Providers.Definitions["oauth2-directory"].Provider != "oauth2" {
		t.Fatalf("response providers missing expected definition: %#v", resp.Providers)
	}
	if resp.Roles == nil || resp.Roles.Definitions["viewer"].Name != "Viewer" {
		t.Fatalf("response roles missing expected definition: %#v", resp.Roles)
	}
	if resp.Workflows == nil || resp.Workflows.Definitions["approval"].Name != "Approval" {
		t.Fatalf("response workflows missing expected definition: %#v", resp.Workflows)
	}
}

func TestPostRegisterOmitsDeviceData(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	deviceID := uuid.NewString()
	cfg := &config.Config{
		Devices: config.DeviceDefinitionsConfig{
			Definitions: map[string]models.Device{
				"workstation-alpha": {
					ID:      deviceID,
					Name:    "Workstation Alpha",
					Enabled: true,
				},
			},
		},
	}

	server := NewServer(cfg)
	router := gin.New()
	router.POST("/register", server.postRegister)

	body, err := json.Marshal(config.RegistrationRequest{
		Mode:       config.ModeAgent,
		Identifier: uuid.MustParse(deviceID),
		Environment: &models.EnvironmentConfig{
			Name:     "alpha",
			Hostname: "alpha.example.test",
			Platform: models.Local,
		},
	})
	if err != nil {
		t.Fatalf("Marshal registration request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal response: %v", err)
	}

	if _, found := resp["device"]; found {
		t.Fatalf("expected registration response to omit device data, got %#v", resp["device"])
	}
}
