package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"
)

// AuditEntry defines the stable schema for every audit log line.
// All fields are required; use an empty string rather than omitting a field.
// The struct is intentionally exported so that tests can compile-time-verify
// the schema without relying on string matching.
type AuditEntry struct {
	Principal string `json:"principal"`
	Remote    string `json:"remote"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Device    string `json:"device"`
	Command   string `json:"command"`
	Result    string `json:"result"`
	RequestID string `json:"request_id"`
	Timestamp string `json:"timestamp"`
}

// auditWriter is the destination for audit log lines (defaults to the
// standard logger output; overridable in tests).
var auditWriter io.Writer = log.Writer()

// writeAuditEntry serialises e as a single JSON line to auditWriter.
// It never omits any field — a zero/empty value is written explicitly.
func writeAuditEntry(e AuditEntry) {
	data, err := json.Marshal(e)
	if err != nil {
		// Marshalling a plain struct cannot fail in practice, but be defensive.
		log.Printf("audit marshal error: %v", err)
		return
	}
	data = append(data, '\n')
	_, _ = auditWriter.Write(data)
}

// audit records a structured audit log entry for a completed command request.
func (h *Handler) audit(r *http.Request, deviceID, commandType, result string) {
	principalID := "unknown"
	if pr, ok := r.Context().Value(principalContextKey).(principal); ok {
		principalID = pr.ID
	}
	writeAuditEntry(AuditEntry{
		Principal: principalID,
		Remote:    clientIP(r),
		Method:    r.Method,
		Path:      r.URL.Path,
		Device:    deviceID,
		Command:   commandType,
		Result:    result,
		RequestID: sanitizeRequestID(r.Header.Get("X-Request-Id")),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}
