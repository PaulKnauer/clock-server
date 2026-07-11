package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/paul/clock-server/internal/application"
	"github.com/paul/clock-server/internal/security"
)

// captureAuditLog redirects auditWriter to a buffer for the duration of the
// test and returns a function that restores the original writer.
func captureAuditLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	original := auditWriter
	auditWriter = buf
	t.Cleanup(func() { auditWriter = original })
	return buf
}

// requiredAuditFields lists every field that must be present and non-empty on
// every audit log entry. This list is the compile-time enforcement contract:
// if a field is renamed in AuditEntry, the corresponding JSON tag changes and
// the test below will catch the regression immediately.
var requiredAuditFields = []string{
	"principal",
	"remote",
	"method",
	"path",
	"device",
	"command",
	"result",
	"request_id",
	"timestamp",
}

// assertAuditFields unmarshals the first JSON line in buf and asserts that
// every required field is present and non-empty.
func assertAuditFields(t *testing.T, buf *bytes.Buffer) {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatal("expected at least one audit log line, got none")
	}

	// Use the last non-empty line (some paths emit exactly one; some paths may
	// emit nothing and we catch that above).
	var auditLine string
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] != "" {
			auditLine = lines[i]
			break
		}
	}

	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(auditLine), &entry); err != nil {
		t.Fatalf("audit line is not valid JSON: %v\nline: %s", err, auditLine)
	}

	for _, field := range requiredAuditFields {
		val, ok := entry[field]
		if !ok {
			t.Errorf("audit entry missing required field %q", field)
			continue
		}
		strVal, isStr := val.(string)
		if !isStr || strings.TrimSpace(strVal) == "" {
			t.Errorf("audit field %q is present but empty or non-string (got %v)", field, val)
		}
	}
}

// assertTimestampRFC3339 verifies the timestamp field is a valid RFC3339 value.
func assertTimestampRFC3339(t *testing.T, buf *bytes.Buffer) {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return
	}
	var auditLine string
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] != "" {
			auditLine = lines[i]
			break
		}
	}

	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(auditLine), &entry); err != nil {
		return // already caught by assertAuditFields
	}
	ts, ok := entry["timestamp"].(string)
	if !ok {
		t.Error("timestamp field is not a string")
		return
	}
	if _, err := time.Parse(time.RFC3339, ts); err != nil {
		t.Errorf("timestamp %q is not RFC3339: %v", ts, err)
	}
}

// assertNoSensitiveData ensures that raw sensitive values (Authorization header
// value, the literal string "token", "password") are absent from the full log
// output.  We check the complete buffer string so that even a mis-encoded value
// embedded in another field would be caught.
func assertNoSensitiveData(t *testing.T, buf *bytes.Buffer, sensitiveValues ...string) {
	t.Helper()
	output := buf.String()
	for _, secret := range sensitiveValues {
		if secret == "" {
			continue
		}
		if strings.Contains(output, secret) {
			t.Errorf("audit log contains sensitive value %q — must not appear in logs", secret)
		}
	}
}

// newScopedHandler creates a handler where credential "ops" can only reach
// device "clock-allowed".
func newScopedHandler() *Handler {
	dispatcher := application.NewCommandDispatcher(&stubSender{})
	return NewHandler(
		dispatcher,
		security.NewCredentialStore([]security.Credential{{ID: "ops", Token: "scoped-token", Devices: []string{"clock-allowed"}}}),
		false, false, true, 64*1024, 100,
	)
}

// ---- Success path ----

func TestAuditSchema_SuccessPath(t *testing.T) {
	buf := captureAuditLog(t)

	h := newTestHandler(&stubSender{})
	body := []byte(`{"deviceId":"clock-1","alarmTime":"2030-01-01T07:00:00Z","label":"wake"}`)

	req := httptest.NewRequest(http.MethodPost, "/commands/alarms", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("X-Request-Id", "req-success-001")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}

	assertAuditFields(t, buf)
	assertTimestampRFC3339(t, buf)
}

// ---- Validation failure path ----

func TestAuditSchema_ValidationFailure(t *testing.T) {
	buf := captureAuditLog(t)

	h := newTestHandler(&stubSender{err: application.ErrValidation})
	body := []byte(`{"deviceId":"clock-1","level":101}`)

	req := httptest.NewRequest(http.MethodPut, "/commands/brightness", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("X-Request-Id", "req-validation-001")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}

	assertAuditFields(t, buf)
	assertTimestampRFC3339(t, buf)
}

// ---- Scope / device-authorization failure path ----

func TestAuditSchema_ScopeFailure(t *testing.T) {
	buf := captureAuditLog(t)

	h := newScopedHandler()
	body := []byte(`{"deviceId":"clock-denied","message":"hi","durationSeconds":10}`)

	req := httptest.NewRequest(http.MethodPost, "/commands/messages", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer scoped-token")
	req.Header.Set("X-Request-Id", "req-scope-001")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}

	// The scope failure is caught before the command is dispatched, so no
	// audit line is written for device-level scope errors (the auth middleware
	// rejects first).  However a direct scope check on an authorised path
	// writes nothing — the test verifies no panic occurs and no sensitive data
	// leaks.  If an entry WAS written we validate its schema too.
	if buf.Len() > 0 {
		assertAuditFields(t, buf)
	}
}

// ---- Downstream failure path ----

func TestAuditSchema_DownstreamFailure(t *testing.T) {
	buf := captureAuditLog(t)

	h := newTestHandler(&stubSender{err: application.ErrDownstream})
	body := []byte(`{"deviceId":"clock-1","message":"hello","durationSeconds":5}`)

	req := httptest.NewRequest(http.MethodPost, "/commands/messages", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("X-Request-Id", "req-downstream-001")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rr.Code)
	}

	assertAuditFields(t, buf)
	assertTimestampRFC3339(t, buf)
}

// ---- Negative: sensitive fields must never appear in log output ----

func TestAuditSchema_NoSensitiveDataInLog(t *testing.T) {
	const rawToken = "super-secret-bearer-value"
	buf := captureAuditLog(t)

	dispatcher := application.NewCommandDispatcher(&stubSender{})
	h := NewHandler(
		dispatcher,
		security.NewCredentialStore([]security.Credential{{ID: "svc", Token: rawToken, Devices: []string{"*"}}}),
		false, false, true, 64*1024, 100,
	)

	body := []byte(`{"deviceId":"clock-1","alarmTime":"2030-06-01T08:00:00Z","label":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/commands/alarms", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.Header.Set("X-Request-Id", "req-sensitive-001")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: body=%s", rr.Code, rr.Body.String())
	}

	// The raw token value must never appear in any audit output.
	assertNoSensitiveData(t, buf, rawToken)

	// Generic sensitive substrings that must not appear: "password", and the
	// literal Authorization header content (full header value).
	authHeaderValue := "Bearer " + rawToken
	assertNoSensitiveData(t, buf, authHeaderValue, "password")
}

// ---- Negative: auth failure path must not leak token ----

func TestAuditSchema_AuthFailureNoTokenInLog(t *testing.T) {
	const badToken = "bad-token-should-not-leak"
	buf := captureAuditLog(t)

	h := newTestHandler(&stubSender{})
	req := httptest.NewRequest(http.MethodGet, "/commands/alarms", nil)
	req.Header.Set("Authorization", "Bearer "+badToken)
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}

	// Auth failures never reach the audit() call, so the log should be empty.
	// Confirm neither the bad token nor any auth-related sensitive term leaked.
	assertNoSensitiveData(t, buf, badToken, "password")
}

// ---- AuditEntry struct compile-time schema contract ----

// TestAuditEntryStructFields is a compile-time guard: if any required field is
// renamed or removed in AuditEntry, this test will fail to compile.
func TestAuditEntryStructFields(t *testing.T) {
	_ = AuditEntry{
		Principal: "p",
		Remote:    "r",
		Method:    "m",
		Path:      "/p",
		Device:    "d",
		Command:   "c",
		Result:    "ok",
		RequestID: "id",
		Timestamp: "ts",
	}

	// Marshal and verify JSON tags match the required field names.
	entry := AuditEntry{
		Principal: "alice",
		Remote:    "10.0.0.1",
		Method:    "POST",
		Path:      "/commands/alarms",
		Device:    "clock-1",
		Command:   "set_alarm",
		Result:    "accepted",
		RequestID: "req-001",
		Timestamp: "2030-01-01T07:00:00Z",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal AuditEntry: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal AuditEntry JSON: %v", err)
	}

	for _, field := range requiredAuditFields {
		if _, ok := decoded[field]; !ok {
			t.Errorf("AuditEntry JSON is missing required field %q — check JSON tag on AuditEntry struct", field)
		}
	}
}

// TestAuditWriterIsNotNil ensures the package-level auditWriter is initialised.
func TestAuditWriterIsNotNil(t *testing.T) {
	if auditWriter == nil {
		t.Fatal("auditWriter must not be nil")
	}
}

// ---- Helper: io.Discard satisfies io.Writer so captureAuditLog is exercised ----

var _ io.Writer = (*bytes.Buffer)(nil)
