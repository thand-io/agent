package daemon

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/manager"
	"go.temporal.io/sdk/client"
)

//go:embed static/*
var staticFiles embed.FS

func NewServer(cfg *config.Config) *Server {

	workflows := manager.NewWorkflowManager(cfg)

	// Parse the templates
	tmpl, err := template.ParseFS(staticFiles, "static/*.html")
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
	Workflows       *manager.WorkflowManager
	TotalRequests   int64
	ElevateRequests int64
	server          *http.Server
}

func (s *Server) GetConfig() *config.Config {
	return s.Config
}

func (s *Server) GetVersion() string {
	version, gitCommit, ok := common.GetModuleBuildInfo()
	if ok {
		return fmt.Sprintf("%s (git: %s)", version, gitCommit)
	}
	return "unknown"
}

func (s *Server) GetTemplateEngine() *template.Template {
	return s.TemplateEngine
}

func (s *Server) GetTemplateData(c *gin.Context) config.TemplateData {

	var user *models.User

	sessions, foundSessions := c.Get(SessionContextKey)

	if foundSessions {

		foundSessions, ok := sessions.(map[string]*models.Session)

		if !ok {
			logrus.Warnln("Invalid session type found in context")
		} else if len(foundSessions) > 0 {
			logrus.WithField("providers", foundSessions).Debugln("User sessions found in context")

			for _, session := range foundSessions {
				user = session.User
				break
			}
		}
	}

	serverName := "Thand Service"

	if s.Config.IsAgent() {
		serverName = "Thand Agent"
	} else if s.Config.IsServer() {
		serverName = "Thand Server"
	}

	return config.TemplateData{
		Config:      s.Config,
		ServiceName: serverName,
		User:        user,
		Version:     s.GetVersion(),
		Status:      "Online",
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

	allowedOrigins := []string{
		s.Config.GetLocalServerUrl(),
	}

	if len(s.Config.GetLoginServerUrl()) > 0 {
		allowedOrigins = append(allowedOrigins, s.Config.GetLoginServerUrl())
	}

	logrus.WithFields(logrus.Fields{
		"allowedOrigins": allowedOrigins,
	}).Debugln("CORS configuration")

	router.Use(cors.New(cors.Config{
		AllowOrigins: allowedOrigins,
		AllowHeaders: []string{
			"Origin",
			"Content-Length",
			"Content-Type",
			"Authorization",
			"Accept",
			"X-Requested-With",
		},
		AllowCredentials: false,
	}))

	router.Use(sessions.Sessions("thand", getSessionStore(s.GetConfig().GetSecret())))

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
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		log.Println("Server Shutdown:", err)
	}
	log.Println("Server exiting")
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

	// Health endpoint
	if s.Config.Server.Health.Enabled {
		router.GET(s.Config.Server.Health.Path, s.healthHandler)
	}

	// Metrics endpoint
	if s.Config.Server.Metrics.Enabled {
		router.GET(s.Config.Server.Metrics.Path, s.metricsHandler)
	}

	// Now enable auth
	router.Use(s.AuthMiddleware())

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
		router.GET("/execution/:id/terminate", s.terminateRunningWorkflow)

		router.GET("/workflow/:name", s.getWorkflowByName)

	} else if s.Config.IsAgent() || s.Config.IsClient() {

		router.GET("/auth", func(ctx *gin.Context) {

			loginServer := s.Config.GetLoginServerUrl()
			callbackUrl := s.Config.GetLocalServerUrl()

			if strings.Compare(loginServer, callbackUrl) == 0 {
				s.getErrorPage(ctx,
					http.StatusBadRequest,
					"Invalid Configuration",
					fmt.Errorf("login server URL cannot be the same as local server URL"))
				return
			}

			ctx.Redirect(http.StatusTemporaryRedirect, loginServer+"/auth?callback="+callbackUrl)
		})
	}

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

		if s.Config.IsAgent() || s.Config.IsClient() {

			// Agent endpoints

			// Session management
			api.GET("/sessions", s.getSessions)
			api.GET("/session/:provider", s.getSessionByProvider)
			api.POST("/sessions", s.postSession)
			api.DELETE("/session/:provider", s.deleteSession)

		} else if s.Config.IsServer() {

			// Register handlers
			api.POST("/preflight", func(ctx *gin.Context) {})
			api.POST("/register", s.postRegister)
			api.POST("/postflight", func(ctx *gin.Context) {})

			// Server endpoints
			api.GET("/roles", s.getRoles)
			api.GET("/workflows", s.getWorkflows)
			api.GET("/providers", s.getProviders)

			api.GET("/role/:role", s.getRoleByName)
			api.GET("/workflow/:name", s.getWorkflowByName)
			api.GET("/provider/:provider", s.getProviderByName)
			api.GET("/provider/:provider/permissions", s.getProviderPermissions)
			api.GET("/provider/:provider/roles", s.getProviderRoles)
			api.POST("/provider/:provider/authorizeSession", s.postProviderAuthorizeSession)

			// Sync endpoints
			api.GET("/sync", s.getSync)

			api.GET("/auth/request/:provider", s.getAuthRequest)
			api.GET("/auth/callback/:provider", s.getAuthCallback)

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
			api.GET("/execution/:id/terminate", s.terminateRunningWorkflow)

		}

	}

}

// healthHandler handles the health check endpoint
func (s *Server) healthHandler(c *gin.Context) {

	servicesHealth := make(map[string]models.HealthState)

	services := s.Config.GetServices()

	if services.HasTemporal() {
		_, err := services.GetTemporal().GetClient().CheckHealth(
			c.Request.Context(), &client.CheckHealthRequest{})
		if err != nil {

			logrus.WithError(err).Error("Temporal service health check failed")

			servicesHealth["temporal"] = models.HealthStatusUnhealthy
		} else {
			servicesHealth["temporal"] = models.HealthStatusHealthy
		}
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

// metricsHandler handles the metrics endpoint
func (s *Server) metricsHandler(c *gin.Context) {
	uptime := time.Since(s.StartTime)

	metrics := models.MetricsInfo{
		Uptime:          uptime.String(),
		TotalRequests:   atomic.LoadInt64(&s.TotalRequests),
		RolesCount:      len(s.Config.Roles.Definitions),
		WorkflowsCount:  len(s.Config.Workflows.Definitions),
		ProvidersCount:  len(s.Config.Providers.Definitions),
		ElevateRequests: atomic.LoadInt64(&s.ElevateRequests),
	}

	c.JSON(http.StatusOK, metrics)
}

func (s *Server) getFavicon(c *gin.Context) {
	c.FileFromFS("static/favicon.ico", http.FS(staticFiles))
}

func (s *Server) getStyle(c *gin.Context) {
	c.FileFromFS("static/styles.css", http.FS(staticFiles))
}

// In your server setup
func getSessionStore(secret string) sessions.Store {
	store := cookie.NewStore([]byte(secret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
		Secure:   true, // Set to true in production with HTTPS
	})
	return store
}
