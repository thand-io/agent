// Package daemon provides the HTTP server implementation for the Thand Agent
//
//	@title						Thand Agent API
//	@version					1.0
//	@description				Thand Agent API for managing access, roles, workflows, and providers
//	@termsOfService				https://thand.io/terms
//	@contact.name				Thand Support
//	@contact.url				https://thand.io/support
//	@contact.email				support@thand.io
//	@license.name				BSL
//	@license.url				https://mariadb.com/bsl11/
//	@host						localhost:8080
//	@BasePath					/api/v1
//	@schemes					http https
//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Bearer token authentication
package daemon

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/goccy/go-yaml"
	"github.com/sirupsen/logrus"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	_ "github.com/thand-io/agent/docs" // Import generated swagger docs
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/manager"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
	"go.temporal.io/api/workflowservice/v1"
)

//go:embed static/*
var staticFiles embed.FS

func NewServer(cfg *config.Config) *Server {

	workflows, err := manager.NewThandWorkflowManager(cfg)

	if err != nil {
		logrus.WithError(err).Fatal("Failed to create workflow manager")
		return nil
	}

	// Create template functions
	funcMap := template.FuncMap{
		"toJSON": func(v any) string {
			jsonBytes, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				return fmt.Sprintf("Error: %v", err)
			}
			return string(jsonBytes)
		},
		"toYAML": func(v any) string {
			yamlBytes, err := yaml.Marshal(v)
			if err != nil {
				return fmt.Sprintf("Error: %v", err)
			}
			return string(yamlBytes)
		},
	}

	// Parse the templates with custom functions
	tmpl, err := template.New("").Funcs(funcMap).ParseFS(staticFiles, "static/*.html")
	if err != nil {
		logrus.WithError(err).Fatal("Failed to parse templates")
	}

	// Create a new server instance with the provided configuration
	server := &Server{
		Config:         cfg,
		TemplateEngine: tmpl,
		Workflows:      workflows,
		StartTime:      time.Now().UTC(),
	}

	return server
}

// Server represents the web service that handles CLI requests
type Server struct {
	Config          *config.Config
	TemplateEngine  *template.Template
	StartTime       time.Time
	Workflows       *manager.ThandWorkflowManager
	TotalRequests   int64
	ElevateRequests int64
	server          *http.Server
}

func (s *Server) GetConfig() *config.Config {
	return s.Config
}

func (s *Server) GetVersion() string {
	return common.GetVersion()
}

func (s *Server) GetTemplateEngine() *template.Template {
	return s.TemplateEngine
}

func (s *Server) GetTemplateData(c *gin.Context) TemplateData {

	var foundUser *models.User
	var foundProvider string

	if s.Config.IsServer() {

		provider, foundSession, err := s.getSession(c)

		if err == nil {
			foundUser = foundSession.User
			foundProvider = provider
		}

	}

	serverName := "Thand Service"

	if s.Config.IsAgent() {
		serverName = "Thand Agent"
	} else if s.Config.IsServer() {
		serverName = "Thand Server"
	}

	hasTemporal := false

	if s.Config.GetServices().HasTemporal() {
		hasTemporal = true
	}

	hasAI := false

	if s.Config.HasLargeLanguageModel() {
		hasAI = true
	}

	return TemplateData{
		Config: SimpleConfig{
			ApiBasePath: s.Config.GetApiBasePath(),
			Server: SimpleServer{
				Host: s.Config.Server.Host,
				Port: s.Config.Server.Port,
				Health: SimpleHealth{
					Enabled: s.Config.Server.Health.Enabled,
					Path:    s.Config.Server.Health.Path,
				},
				Metrics: SimpleMetrics{
					Enabled: s.Config.Server.Metrics.Enabled,
					Path:    s.Config.Server.Metrics.Path,
				},
				Capabilities: s.Config.Server.Capabilities,
			},
			Services: SimpleServices{
				HasTemporal:           hasTemporal,
				HasLargeLanguageModel: hasAI,
			},
		},
		ServiceName: serverName,
		Provider:    foundProvider,
		Environment: SimpleEnvrinment{
			Platform: string(s.Config.Environment.Platform),
		},
		User:    foundUser,
		Version: s.GetVersion(),
		Status:  "Online",
	}
}

// Start initializes and starts the web service
func (s *Server) Start() error {
	// Set Gin mode based on configuration
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()

	// Add middleware
	router.Use(gin.Logger())
	router.Use(gin.CustomRecovery(
		func(c *gin.Context, err any) {

			logrus.WithError(err.(error)).Error("Recovered from panic")

			foundError, ok := err.(error)

			if !ok {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
			}

			// If the client accepts html then return error.html otherwise,
			// return the json error

			s.getErrorPage(c, http.StatusInternalServerError, "Internal Server Error", foundError)
		},
	))
	router.Use(s.requestCounterMiddleware())

	// Build CORS config from server security settings
	corsConfig := s.Config.Server.Security.CORS.WithDefaults()
	corsConfig.AllowCredentials = true
	corsConfig.AddOrigins(s.Config.GetLocalServerUrl())
	if len(s.Config.GetLoginServerUrl()) > 0 {
		corsConfig.AddOrigins(s.Config.GetLoginServerUrl())
	}

	logrus.WithFields(logrus.Fields{
		"allowedOrigins": corsConfig.AllowedOrigins,
	}).Debugln("CORS configuration")

	router.Use(CORSMiddleware(corsConfig))

	cookieNames := []string{ThandCookieName}

	foundProviders := s.Config.GetProvidersByCapability(
		models.ProviderCapabilityAuthorizer,
	)

	for providerName := range foundProviders {
		cookieNames = append(cookieNames, CreateCookieName(providerName))
	}

	sessionStore := s.getSessionStore(s.GetConfig().GetSecret())
	router.Use(sessions.SessionsMany(
		cookieNames,
		sessionStore,
	))

	// Set HTML template engine
	router.SetHTMLTemplate(s.TemplateEngine)

	// Setup routes
	s.setupRoutes(router)

	// Start server
	addr := fmt.Sprintf("%s:%d", s.Config.Server.Host, s.Config.Server.Port)
	fmt.Printf("Starting web service on %s\n", addr)

	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  s.Config.Server.Limits.ReadTimeout,
		WriteTimeout: s.Config.Server.Limits.WriteTimeout,
		IdleTimeout:  s.Config.Server.Limits.IdleTimeout,
	}

	// Store server reference for shutdown
	s.server = server

	// Channel to capture startup errors
	errChan := make(chan error, 1)

	// Start server in goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil {
			errChan <- err
		}
	}()

	// Wait a moment to see if the server fails to start
	select {
	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("failed to start server: %v", err)
		}
		// Server shutdown gracefully
		return nil
	case <-time.After(100 * time.Millisecond):
		// Server started successfully
		fmt.Printf("Web service started successfully on %s\n", addr)
		return nil
	}
}

func (s *Server) Stop() {
	if s.server == nil {
		logrus.Warning("Server is not running")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		logrus.WithError(err).Error("Server Shutdown")
	}
	logrus.Info("Server exiting")
}

// requestCounterMiddleware increments the request counter
func (s *Server) requestCounterMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		atomic.AddInt64(&s.TotalRequests, 1)
		c.Next()
	}
}

// setupRoutes configures all the HTTP routes
func (s *Server) setupRoutes(router *gin.Engine) {
	// Serve static files and landing page
	// router.StaticFS("/static", http.FS(staticFiles))

	// Add favicon
	router.GET("/favicon.ico", s.getFavicon)
	router.GET("/styles.css", s.getStyle)

	// Swagger documentation
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	router.GET("/.well-known/api-configuration", s.apiConfigurationHandler)

	// Health endpoint
	if s.Config.Server.Health.Enabled {
		router.GET(s.Config.Server.Health.Path, s.healthHandler)
	}

	// Ready endpoint
	if s.Config.Server.Ready.Enabled {
		router.GET(s.Config.Server.Ready.Path, s.readyHandler)
	}

	// Metrics endpoint
	if s.Config.Server.Metrics.Enabled {
		router.GET(s.Config.Server.Metrics.Path, s.metricsHandler)
	}

	// Now enable auth
	router.Use(s.AuthMiddleware())

	// Enable hostname detection
	router.Use(s.SetupMiddleware())

	// Serve the landing page at root
	router.GET("/", s.getIndexPage)

	// Server endpoint
	if s.Config.IsServer() {

		router.GET("/elevate", s.getElevatePage)
		router.GET("/elevate/static", s.getElevateStaticPage)
		router.GET("/elevate/dynamic", s.getElevateDynamicPage)
		router.GET("/elevate/llm", s.getElevateLLMPage)

		router.GET("/auth", s.getAuthPage)
		router.GET("/logout", s.getLogoutPage)

		router.GET("/executions", s.getExecutionsPage)
		router.GET("/execution/:id", s.getRunningWorkflow)
		router.GET("/execution/:id/cancel", s.cancelRunningWorkflow)       // Graceful cancellation
		router.GET("/execution/:id/terminate", s.terminateRunningWorkflow) // Forceful termination

		// Form routes for workflow task forms
		router.GET("/execution/:id/form", s.getFormPage)
		router.POST("/execution/:id/form", s.submitForm)

		router.GET("/workflow/:name", s.getWorkflowByName)
		router.GET("/role/:role", s.getRolePage)

	} else if s.Config.IsAgent() || s.Config.IsClient() {

		router.GET("/auth", func(ctx *gin.Context) {

			loginServer := s.Config.GetLoginServerUrl()
			callbackUrl := s.Config.GetLocalServerUrl()

			if strings.EqualFold(callbackUrl, loginServer) {
				s.getErrorPage(ctx,
					http.StatusBadRequest,
					"Invalid Configuration",
					fmt.Errorf("login server URL cannot be the same as local server URL"))
				return
			}

			if !s.Config.GetServices().HasEncryption() {
				s.getErrorPage(ctx,
					http.StatusInternalServerError,
					"Encryption service not configured",
					fmt.Errorf("encryption service must be configured to create session codes"))
				return
			}

			// Create a code to identify the session after authentication
			// This code is encrypted and can only be used by the agent
			sessionCode := models.EncodingWrapper{
				Type: sdkConstants.ENCODED_SESSION_CODE,
				Data: models.NewCodeWrapper(loginServer),
			}.EncodeAndEncrypt(
				s.Config.GetServices().GetEncryption(),
			)

			params := url.Values{
				"callback": {callbackUrl},
				"code":     {sessionCode},
			}

			ctx.Redirect(http.StatusTemporaryRedirect, loginServer+"/auth?"+params.Encode())
		})
	}

	router.GET("/logs", s.getLogsPage)

	// Server shows the server info and calls the local daemon
	// for session info. If in agent mode then this call just
	// shows local session info
	router.GET("/user", s.getUserPage)

	// Either agent or server mode
	router.GET("/roles", s.getRolesPage)
	router.GET("/workflows", s.getWorkflowsPage)
	router.GET("/providers", s.getProvidersPage)

	// API endpoints
	api := router.Group(s.Config.GetApiBasePath())
	{

		api.GET("/logs", s.getLogsPage)
		api.GET("/sessions", s.getSessions)
		api.GET("/whoami", s.getWhoami)

		// Agent endpoints
		if s.Config.IsAgent() || s.Config.IsClient() {

			// Session management
			api.GET("/session/:provider", s.getSessionByProvider)
			api.POST("/sessions", s.postSession)
			api.DELETE("/session/:provider", s.deleteSession)
			api.DELETE("/sessions", s.deleteSessions)

		} else if s.Config.IsServer() {

			// Session management
			api.PUT("/sessions", s.putSession)
			api.DELETE("/session/:provider", s.deleteSession)
			api.DELETE("/sessions", s.deleteSessions)

			// Register handlers
			api.POST("/preflight", func(c *gin.Context) {
				// Just a stub for now
				c.JSON(http.StatusOK, config.PreflightResponse{
					Success: true,
				})
			})
			api.POST("/register", s.postRegister)
			api.POST("/postflight", func(c *gin.Context) {

				// Just a stub for now
				c.JSON(http.StatusOK, config.PostflightResponse{
					Success: true,
				})
			})

			api.POST("/logs", func(c *gin.Context) {

				// Just a stub for now
				c.JSON(http.StatusOK, gin.H{})
			})

			// Server endpoints
			api.GET("/roles", s.getRoles)
			api.POST("/roles/evaluate", s.postEvaluateRole)
			api.GET("/workflows", s.getWorkflows)
			api.GET("/providers", s.getProviders)
			api.GET("/identities", s.getIdentities)

			api.GET("/role/:role", s.getRoleByName)
			api.GET("/workflow/:name", s.getWorkflowByName)
			api.GET("/provider/:provider", s.getProviderByName)
			api.GET("/provider/:provider/permissions", s.getProviderPermissions)
			api.GET("/provider/:provider/roles", s.getProviderRoles)
			api.GET("/provider/:provider/identities", s.getProviderIdentities)
			api.GET("/provider/:provider/resources", s.getProviderResources)
			api.GET("/provider/:provider/tenants", s.getProviderTenants)

			// Proxied provider calls. These help the client invoke
			// provider specific functions via the server.
			api.Any("/provider/:provider/:function", s.providerFunctionHandler)

			// Sync endpoints
			api.GET("/sync", s.getSync)

			api.GET("/auth/request/:provider", s.getAuthRequest)
			api.GET("/auth/callback/:provider", s.getAuthCallback)   // OAuth2 callbacks
			api.POST("/auth/callback/:provider", s.postAuthCallback) // SAML callbacks
			api.GET("/auth/logout/:provider", s.getLogoutPage)
			api.GET("/auth/logout", s.getLogoutPage)

			// /elevate?role=admin&provider=server&reason=maintenance&duration=1h
			api.GET("/elevate", s.getElevate)
			api.POST("/elevate", s.postElevate)
			api.GET("/elevate/llm", s.getElevateLLM)
			api.POST("/elevate/llm", s.postElevateLLM)

			// resume a workflow given a state
			api.GET("/elevate/resume", s.getElevateResume)
			api.POST("/elevate/resume", s.postElevateResume)

			// get workflow info
			api.GET("/executions", s.listRunningWorkflows)
			api.POST("/execution", s.createWorkflow)

			api.GET("/execution/:id", s.getRunningWorkflow)
			api.GET("/execution/:id/cancel", s.cancelRunningWorkflow)
			api.GET("/execution/:id/terminate", s.terminateRunningWorkflow)
			api.GET("/execution/:id/signal", s.signalRunningWorkflow)

			// Approval shortcut: send approval/denial for current execution
			api.GET("/execution/:id/approvals", s.approveRunningWorkflow)

			// Form API endpoints
			api.GET("/execution/:id/form", s.getFormPage)
			api.POST("/execution/:id/form", s.submitForm)

		}
	}
}

// healthHandler handles the health check endpoint
//
//	@Summary		Health check
//	@Description	Get the health status of the service and its dependencies
//	@Tags			health
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	models.HealthResponse	"Health status"
//	@Router			/health [get]
func (s *Server) healthHandler(c *gin.Context) {

	servicesHealth := make(map[string]models.HealthState)

	services := s.Config.GetServices()

	if services.HasTemporal() {

		// Use count rather than health check as temporal cloud
		// does not support external health checks
		_, err := services.GetTemporal().GetClient().CountWorkflow(
			c.Request.Context(),
			&workflowservice.CountWorkflowExecutionsRequest{},
		)
		if err != nil {

			logrus.WithError(err).Error("Temporal service health check failed")

			servicesHealth["temporal"] = models.HealthStatusUnhealthy
		} else {
			servicesHealth["temporal"] = models.HealthStatusHealthy
		}
	}

	if services.HasAnalytics() {
		servicesHealth["analytics"] = models.HealthStatusHealthy
	}

	if services.HasLargeLanguageModel() {
		servicesHealth["llm"] = models.HealthStatusHealthy
	}

	if services.HasEncryption() {
		servicesHealth["encryption"] = models.HealthStatusHealthy
	}

	if services.HasVault() {
		servicesHealth["vault"] = models.HealthStatusHealthy
	}

	if services.HasScheduler() {
		servicesHealth["scheduler"] = models.HealthStatusHealthy
	}

	if services.HasStorage() {
		servicesHealth["storage"] = models.HealthStatusHealthy
	}

	overallStatus := models.HealthStatusHealthy

	for _, status := range servicesHealth {
		if status != models.HealthStatusHealthy {
			overallStatus = models.HealthStatusDegraded
			break
		}
	}

	response := models.HealthResponse{
		Status:      overallStatus,
		ApiBasePath: s.Config.GetApiBasePath(),
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Version:     s.GetVersion(),
		Services:    servicesHealth,
	}

	c.JSON(http.StatusOK, response)
}

// apiConfigurationHandler handles the API configuration endpoint
//
//	@Summary		API configuration
//	@Description	Get service configuration including endpoints, capabilities, and authentication methods
//	@Tags			discovery
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]any	"API configuration"
//	@Router			/.well-known/api-configuration [get]
func (s *Server) apiConfigurationHandler(c *gin.Context) {
	serviceType := "agent"
	if s.Config.IsServer() {
		serviceType = "server"
	} else if s.Config.IsClient() {
		serviceType = "client"
	}

	capabilities := make(map[string]bool)
	workflows := []string{}  // TODO: populate workflows list
	activities := []string{} // TODO: populate activities list

	if s.Config.IsServer() {
		services := s.Config.GetServices()
		capabilities["temporal"] = services.HasTemporal()
		capabilities["vault"] = services.HasVault()
		capabilities["encryption"] = services.HasEncryption()
		capabilities["scheduler"] = services.HasScheduler()
		capabilities["llm"] = services.HasLargeLanguageModel()
		capabilities["storage"] = services.HasStorage()
	}

	response := gin.H{
		// Service Identity
		"serviceName": "Thand " + serviceType,
		"serviceType": serviceType,
		"version":     s.GetVersion(),

		// Endpoints
		"baseUrl":     s.Config.GetLocalServerUrl(),
		"apiBasePath": s.Config.GetApiBasePath(),
		//"hostname":    s.Config.Environment.Hostname,
		//"port":        s.Config.Server.Port,

		// Authentication
		"authEndpoint": s.Config.GetLocalServerUrl() + "/auth",
		"authMethods":  []string{"session", "bearer"},

		// Documentation
		"docsUrl":     s.Config.GetLocalServerUrl() + "/swagger/index.html",
		"openApiSpec": s.Config.GetLocalServerUrl() + "/swagger/doc.json",

		// Capabilities
		"workflows":    workflows,
		"activities":   activities,
		"capabilities": capabilities,

		// Health
		"healthEndpoint":  s.Config.Server.Health.Path,
		"metricsEndpoint": s.Config.Server.Metrics.Path,

		// Related Services
		"loginServer": s.Config.GetLoginServerUrl(),
	}

	c.JSON(http.StatusOK, response)
}

// readyHandler handles the readiness check endpoint
//
//	@Summary		Readiness check
//	@Description	Check if the service is ready to accept requests
//	@Tags			health
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]any	"Ready status"
//	@Router			/ready [get]
func (s *Server) readyHandler(c *gin.Context) {
	response := gin.H{
		"status":    "ready",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   s.GetVersion(),
	}

	c.JSON(http.StatusOK, response)
}

// metricsHandler handles the metrics endpoint
//
//	@Summary		Service metrics
//	@Description	Get service metrics including uptime, request counts, and resource counts
//	@Tags			metrics
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	models.MetricsInfo	"Service metrics"
//	@Router			/metrics [get]
func (s *Server) metricsHandler(c *gin.Context) {
	uptime := time.Since(s.StartTime)

	identitiesCount := int64(0)
	tenantsCount := int64(0)

	if s.Config.IsServer() {
		identitiesCount = s.Config.GetIdentitiesCount()
		tenantsCount = s.Config.GetTenantsCount()
	}

	metrics := models.MetricsInfo{
		Uptime: uptime.String(),

		RequestsCount:   atomic.LoadInt64(&s.TotalRequests),
		ElevateRequests: atomic.LoadInt64(&s.ElevateRequests),

		RolesCount:     int64(len(s.Config.Roles.Definitions)),
		WorkflowsCount: int64(len(s.Config.Workflows.Definitions)),
		ProvidersCount: int64(len(s.Config.Providers.Definitions)),

		IdentitiesCount: identitiesCount,
		TenantsCount:    tenantsCount,
	}

	c.JSON(http.StatusOK, metrics)
}

func (s *Server) getFavicon(c *gin.Context) {
	c.FileFromFS("static/favicon.ico", http.FS(staticFiles))
}

func (s *Server) getStyle(c *gin.Context) {
	c.FileFromFS("static/styles.css", http.FS(staticFiles))
}
