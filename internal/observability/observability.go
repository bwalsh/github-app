// Package observability provides structured logging and metrics utilities.
package observability

import (
	"encoding/json"
	"log"
	"time"
)

// LogLevel represents the severity of a log message.
type LogLevel string

const (
	LevelDebug LogLevel = "DEBUG"
	LevelInfo  LogLevel = "INFO"
	LevelWarn  LogLevel = "WARN"
	LevelError LogLevel = "ERROR"
)

// LogEvent represents a structured log event.
type LogEvent struct {
	Timestamp  time.Time              `json:"timestamp"`
	Level      LogLevel               `json:"level"`
	Message    string                 `json:"message"`
	Component  string                 `json:"component"`
	TraceID    string                 `json:"trace_id,omitempty"`
	InstallID  int64                  `json:"installation_id,omitempty"`
	RepoID     int64                  `json:"repository_id,omitempty"`
	RepoName   string                 `json:"repo_name,omitempty"`
	TenantName string                 `json:"tenant_name,omitempty"`
	StatusCode int                    `json:"status_code,omitempty"`
	Duration   int64                  `json:"duration_ms,omitempty"` // milliseconds
	ErrorMsg   string                 `json:"error,omitempty"`
	CustomData map[string]interface{} `json:"data,omitempty"`
}

// Log writes a structured log event.
func Log(level LogLevel, component, message string, event *LogEvent) {
	if event == nil {
		event = &LogEvent{}
	}
	event.Level = level
	event.Component = component
	event.Message = message
	event.Timestamp = time.Now()

	// Output as JSON
	data, _ := json.Marshal(event)
	log.Println(string(data))
}

// LogInfo logs an info-level event.
func LogInfo(component, message string, event *LogEvent) {
	Log(LevelInfo, component, message, event)
}

// LogError logs an error-level event.
func LogError(component, message string, event *LogEvent) {
	Log(LevelError, component, message, event)
}

// LogDebug logs a debug-level event.
func LogDebug(component, message string, event *LogEvent) {
	Log(LevelDebug, component, message, event)
}

// Metrics tracks operational metrics.
type Metrics struct {
	WebhooksReceived   int64
	WebhooksProcessed  int64
	WebhooksFailed     int64
	JobsEnqueued       int64
	JobsCompleted      int64
	JobsFailed         int64
	AverageJobDuration int64 // milliseconds
}

// AuditLog logs tenant operations for audit trail.
type AuditLog struct {
	Timestamp  time.Time
	Action     string // "REGISTER", "LOOKUP", "DELETE"
	InstallID  int64
	RepoID     int64
	TenantName string
	Success    bool
	ErrorMsg   string
}

// LogAudit logs a tenant audit event.
func LogAudit(action string, installID, repoID int64, tenantName string, success bool, err error) {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	event := &LogEvent{
		Timestamp:  time.Now(),
		Level:      LevelInfo,
		Component:  "tenant-registry",
		Message:    "audit: " + action,
		InstallID:  installID,
		RepoID:     repoID,
		TenantName: tenantName,
		CustomData: map[string]interface{}{
			"action":  action,
			"success": success,
		},
		ErrorMsg: errMsg,
	}

	data, _ := json.Marshal(event)
	log.Println(string(data))
}
