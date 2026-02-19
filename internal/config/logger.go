package config

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

const LoggerSystemAttributeAgentID = "agent.id"
const LoggerSystemAttributeSessionID = "agent.session_id"
const LoggerSystemAttributeFields = "fields"

var LoggerSystemAttributeError = logrus.ErrorKey

type thandLogger struct {
	// Ring buffer for storing events
	sessionUID  uuid.UUID
	eventBuffer []*models.LogEntry
	maxSize     int
	currentPos  int
	isFull      bool
	mu          sync.RWMutex

	// OpenTelemetry remote logging
	openTelemetryProvider *sdklog.LoggerProvider
	openTelemetryLogger   log.Logger
	remoteEnabled         bool
}

func NewThandLogger() *thandLogger {
	return &thandLogger{
		sessionUID:  uuid.New(),
		eventBuffer: make([]*models.LogEntry, 1000),
		maxSize:     1000,
		currentPos:  0,
		isFull:      false,
	}
}

// EnableRemoteLogging sets up OpenTelemetry log export after authentication
func (t *thandLogger) EnableRemoteLogging(ctx context.Context, endpoint model.Endpoint, identifier uuid.UUID) error {

	loggingHeaders := map[string]string{}

	// Extract endpoint URL and auth token
	loggingEndpoint := endpoint.EndpointConfig.URI.String()

	checkUrl, err := url.Parse(loggingEndpoint)

	if err != nil || (checkUrl.Scheme != "http" && checkUrl.Scheme != "https") {
		return fmt.Errorf("invalid OpenTelemetry endpoint URL: %s", loggingEndpoint)
	}

	logrus.WithField("hostname", checkUrl.Host).Infoln("Configuring OpenTelemetry")

	// Get auth token from endpoint configuration
	if endpoint.EndpointConfig.Authentication != nil && endpoint.EndpointConfig.Authentication.AuthenticationPolicy != nil {
		auth := endpoint.EndpointConfig.Authentication.AuthenticationPolicy

		switch {
		case auth.Basic != nil:
			credentials := base64.StdEncoding.EncodeToString([]byte(auth.Basic.Username + ":" + auth.Basic.Password))
			loggingHeaders["Authorization"] = "Basic " + credentials
		case auth.Bearer != nil:
			loggingHeaders["Authorization"] = "Bearer " + auth.Bearer.Token
		case auth.Digest != nil:
			// Digest auth not directly supported in headers; skipping
			fallthrough
		default:
			return fmt.Errorf("unsupported authentication type for OpenTelemetry endpoint")
		}
	}

	opts := []otlploghttp.Option{
		otlploghttp.WithEndpointURL(loggingEndpoint),
		otlploghttp.WithCompression(otlploghttp.GzipCompression),
		otlploghttp.WithHeaders(loggingHeaders),
		otlploghttp.WithTimeout(1 * time.Second),
	}

	if strings.HasPrefix(loggingEndpoint, "http://") {
		logrus.Warnln("Insecure OpenTelemetry endpoint detected (http). Proceeding without TLS.")
		opts = append(opts, otlploghttp.WithInsecure())
	}

	// Create OTLP HTTP exporter with authentication
	exporter, err := otlploghttp.New(ctx,
		opts...,
	)
	if err != nil {
		return fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Create resource with service info
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("thand-agent"),
			semconv.ServiceInstanceID(identifier.String()),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	// Create log provider with batching
	provider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(
			sdklog.NewBatchProcessor(exporter,
				sdklog.WithExportInterval(30*time.Second),
				sdklog.WithExportMaxBatchSize(100),
			),
		),
		sdklog.WithResource(res),
	)

	logger := provider.Logger("thand-agent")

	// Hold the lock only while updating shared state and copying buffered logs
	t.mu.Lock()
	t.openTelemetryProvider = provider
	t.openTelemetryLogger = logger
	t.remoteEnabled = true

	// Copy existing buffered logs while holding the lock
	existingLogs := t.getEventsInternal()
	t.mu.Unlock()

	// Send existing logs outside the lock to avoid blocking other operations.
	// The logger reference is safe to use here since we set it above and
	// OpenTelemetry's Logger.Emit is safe for concurrent use.
	for _, entry := range existingLogs {
		emitLogRecord(logger, entry, t.sessionUID)
	}

	// ForceFlush can be slow; execute outside the lock
	provider.ForceFlush(ctx)

	logrus.Info("Remote logging enabled via OpenTelemetry")

	return nil
}

// DisableRemoteLogging stops the OpenTelemetry exporter
func (t *thandLogger) DisableRemoteLogging() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.remoteEnabled = false
}

// IsRemoteLoggingEnabled returns whether remote logging is active
func (t *thandLogger) IsRemoteLoggingEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.remoteEnabled
}

// Shutdown gracefully shuts down the OpenTelemetry exporter
func (t *thandLogger) Shutdown(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.openTelemetryProvider != nil {
		t.remoteEnabled = false
		err := t.openTelemetryProvider.Shutdown(ctx)
		t.openTelemetryProvider = nil
		t.openTelemetryLogger = nil
		return err
	}
	return nil
}

// emitLogRecord converts a log entry to an OpenTelemetry record and emits it.
// This function is safe to call without holding t.mu as it only uses the
// provided logger reference and the entry data (which should be a copy or
// otherwise safe to read).
func emitLogRecord(logger log.Logger, entry *models.LogEntry, sessionUID uuid.UUID) {
	var record log.Record
	record.SetTimestamp(entry.Time)
	record.SetBody(log.StringValue(entry.Message))
	record.SetSeverity(logrusToOTelSeverity(entry.Level))
	// e.g. user_lgoin, system_error etc
	record.SetEventName("log.record")

	// Add system attributes
	record.AddAttributes(
		log.String(LoggerSystemAttributeAgentID, common.GetClientIdentifier().String()),
		log.String(LoggerSystemAttributeSessionID, sessionUID.String()),
	)

	fields := []log.KeyValue{}

	// Add logrus fields as attributes
	for key, value := range entry.Data {

		// Detect value and convert accordingly
		switch v := value.(type) {
		case error:
			fields = append(fields, log.String(key, v.Error()))
		case string:
			fields = append(fields, log.String(key, v))
		case int:
			fields = append(fields, log.Int(key, v))
		case int64:
			fields = append(fields, log.Int64(key, v))
		case float64:
			fields = append(fields, log.Float64(key, v))
		case bool:
			fields = append(fields, log.Bool(key, v))
		default:
			fields = append(fields, log.String(key, fmt.Sprintf("%v", value)))
		}
	}

	record.AddAttributes(
		log.Map(LoggerSystemAttributeFields, fields...),
	)

	logger.Emit(context.Background(), record)
}

// sendToOpenTelemetryLocked sends a log entry to the OpenTelemetry exporter.
// Caller must hold t.mu lock. Use Go's race detector (-race) during testing
// to verify proper synchronization.
func (t *thandLogger) sendToOpenTelemetryLocked(entry *models.LogEntry) {
	if !t.remoteEnabled || t.openTelemetryLogger == nil {
		return
	}

	emitLogRecord(t.openTelemetryLogger, entry, t.sessionUID)
}

// logrusToOTelSeverity converts logrus levels to OpenTelemetry severity
func logrusToOTelSeverity(level logrus.Level) log.Severity {
	switch level {
	case logrus.PanicLevel:
		return log.SeverityFatal4
	case logrus.FatalLevel:
		return log.SeverityFatal
	case logrus.ErrorLevel:
		return log.SeverityError
	case logrus.WarnLevel:
		return log.SeverityWarn
	case logrus.InfoLevel:
		return log.SeverityInfo
	case logrus.DebugLevel:
		return log.SeverityDebug
	case logrus.TraceLevel:
		return log.SeverityTrace
	default:
		return log.SeverityInfo
	}
}

func (t *thandLogger) Fire(entry *logrus.Entry) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	thandLog := models.NewLogEntry(entry)

	// Add to ring buffer for local queries
	t.eventBuffer[t.currentPos] = thandLog
	t.currentPos = (t.currentPos + 1) % t.maxSize

	if t.currentPos == 0 {
		t.isFull = true
	}

	// Send to OpenTelemetry if remote logging is enabled
	if t.remoteEnabled {
		t.sendToOpenTelemetryLocked(thandLog)
	}

	return nil
}

func (t *thandLogger) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
		logrus.WarnLevel,
		logrus.InfoLevel,
		// logrus.DebugLevel,
		// logrus.TraceLevel,
	}
}

func (t *thandLogger) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.eventBuffer = make([]*models.LogEntry, t.maxSize)
	t.currentPos = 0
	t.isFull = false
}

func (t *thandLogger) GetEvents() []*models.LogEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.isFull {
		// Return only filled portion
		result := make([]*models.LogEntry, t.currentPos)
		copy(result, t.eventBuffer[:t.currentPos])
		return result
	}

	// Return in chronological order (oldest first)
	result := make([]*models.LogEntry, t.maxSize)
	copy(result, t.eventBuffer[t.currentPos:])
	copy(result[t.maxSize-t.currentPos:], t.eventBuffer[:t.currentPos])
	return result
}

func (t *thandLogger) GetRecentEvents(count int) []*models.LogEntry {
	events := t.GetEvents()
	if len(events) <= count {
		return events
	}
	return events[len(events)-count:]
}

// LogFilter contains the filtering criteria for log events
type LogFilter struct {
	// Filter by log levels (if empty, all levels are included)
	Levels []logrus.Level `json:"levels,omitempty"`
	// Filter events after this time (if nil, no time filter from start)
	Since *time.Time `json:"since,omitempty"`
	// Filter events before this time (if nil, no time filter to end)
	Until *time.Time `json:"until,omitempty"`
	// Maximum number of events to return (if 0, no limit)
	Limit int `json:"limit,omitempty"`
}

func (f *LogFilter) GetLimit() int {
	if f.Limit <= 0 {
		return 100
	}
	return f.Limit
}

// GetEventsWithFilter returns events that match the specified filter criteria
func (t *thandLogger) GetEventsWithFilter(filter LogFilter) []*models.LogEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	allEvents := t.getEventsInternal()
	var filtered []*models.LogEntry

	// Create a map for quick level lookup if levels are specified
	levelMap := make(map[logrus.Level]bool)
	if len(filter.Levels) > 0 {
		for _, level := range filter.Levels {
			levelMap[level] = true
		}
	}

	for _, entry := range allEvents {
		// Filter by log level
		if len(filter.Levels) > 0 && !levelMap[entry.Level] {
			continue
		}

		// Filter by time range
		if filter.Since != nil && entry.Time.Before(*filter.Since) {
			continue
		}
		if filter.Until != nil && entry.Time.After(*filter.Until) {
			continue
		}

		filtered = append(filtered, entry)

		// Apply limit if specified
		if filter.GetLimit() > 0 && len(filtered) >= filter.GetLimit() {
			break
		}
	}

	return filtered
}

// getEventsInternal returns events without additional locking (assumes caller has lock)
func (t *thandLogger) getEventsInternal() []*models.LogEntry {
	if !t.isFull {
		// Return only filled portion
		result := make([]*models.LogEntry, t.currentPos)
		copy(result, t.eventBuffer[:t.currentPos])
		return result
	}

	// Return in chronological order (oldest first)
	result := make([]*models.LogEntry, t.maxSize)
	copy(result, t.eventBuffer[t.currentPos:])
	copy(result[t.maxSize-t.currentPos:], t.eventBuffer[:t.currentPos])
	return result
}
