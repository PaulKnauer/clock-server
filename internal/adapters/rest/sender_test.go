package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/paul/clock-server/internal/domain"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// helpers

func okTransport(statusCode int, body string) roundTripFunc {
	return func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: statusCode,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	}
}

func newTestSender(t *testing.T, transport http.RoundTripper) *Sender {
	t.Helper()
	s, err := NewSender(Config{BaseURL: "http://clock-api.local", AllowInsecureHTTP: true})
	if err != nil {
		t.Fatalf("new sender: %v", err)
	}
	s.client = &http.Client{Timeout: 2 * time.Second, Transport: transport}
	return s
}

// ── NewSender construction ───────────────────────────────────────────────────

func TestNewSender_MissingBaseURL(t *testing.T) {
	_, err := NewSender(Config{})
	if err == nil || !strings.Contains(err.Error(), "rest base url is required") {
		t.Fatalf("expected base-url error, got %v", err)
	}
}

func TestNewSender_InvalidBaseURL(t *testing.T) {
	_, err := NewSender(Config{BaseURL: "://bad url", AllowInsecureHTTP: true})
	if err == nil || !strings.Contains(err.Error(), "invalid rest base url") {
		t.Fatalf("expected invalid-url error, got %v", err)
	}
}

func TestNewSender_InsecureHTTPRejected(t *testing.T) {
	_, err := NewSender(Config{BaseURL: "http://example.com"})
	if err == nil || !strings.Contains(err.Error(), "insecure downstream http is disabled") {
		t.Fatalf("expected insecure-http error, got %v", err)
	}
}

func TestNewSender_HTTPSAllowed(t *testing.T) {
	s, err := NewSender(Config{BaseURL: "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error for https: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil sender")
	}
}

func TestNewSender_AllowInsecureHTTP(t *testing.T) {
	s, err := NewSender(Config{BaseURL: "http://example.com", AllowInsecureHTTP: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil sender")
	}
}

func TestNewSender_DefaultTimeout(t *testing.T) {
	s, err := NewSender(Config{BaseURL: "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.client.Timeout != 5*time.Second {
		t.Fatalf("expected default timeout 5s, got %v", s.client.Timeout)
	}
}

func TestNewSender_CustomTimeout(t *testing.T) {
	s, err := NewSender(Config{BaseURL: "https://example.com", Timeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.client.Timeout != 30*time.Second {
		t.Fatalf("expected 30s timeout, got %v", s.client.Timeout)
	}
}

func TestNewSender_TrailingSlashStripped(t *testing.T) {
	s, err := NewSender(Config{BaseURL: "https://example.com/api///", Timeout: time.Second})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.HasSuffix(s.baseURL, "/") {
		t.Fatalf("base url should not end with slash, got %q", s.baseURL)
	}
}

// ── Send: nil command ────────────────────────────────────────────────────────

func TestSend_NilCommand(t *testing.T) {
	s := newTestSender(t, okTransport(200, ""))
	err := s.Send(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "command is required") {
		t.Fatalf("expected command-required error, got %v", err)
	}
}

// ── Send: alarm command ──────────────────────────────────────────────────────

func TestRESTSenderMapsAlarmCommand(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotBody map[string]any

	sender, err := NewSender(Config{BaseURL: "http://clock-api.local", AllowInsecureHTTP: true})
	if err != nil {
		t.Fatalf("new sender: %v", err)
	}
	sender.client = &http.Client{
		Timeout: 2 * time.Second,
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusAccepted,
				Body:       io.NopCloser(bytes.NewBufferString(`{"ok":true}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	cmd := domain.SetAlarmCommand{DeviceID: "clock-22", AlarmTime: time.Date(2030, 1, 1, 8, 0, 0, 0, time.UTC), Label: "run"}
	if err := sender.Send(context.Background(), cmd); err != nil {
		t.Fatalf("send command: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/clocks/clock-22/alarms" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotBody["label"] != "run" {
		t.Fatalf("expected label run, got %v", gotBody["label"])
	}
}

func TestSend_AlarmCommand_SetsAuthHeader(t *testing.T) {
	var gotAuth string
	s, err := NewSender(Config{BaseURL: "http://clock-api.local", AllowInsecureHTTP: true, AuthToken: "secret-token"})
	if err != nil {
		t.Fatalf("new sender: %v", err)
	}
	s.client = &http.Client{
		Timeout: 2 * time.Second,
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotAuth = r.Header.Get("Authorization")
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		}),
	}
	cmd := domain.SetAlarmCommand{DeviceID: "dev1", AlarmTime: time.Date(2030, 1, 1, 8, 0, 0, 0, time.UTC), Label: "wake"}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("expected Bearer token, got %q", gotAuth)
	}
}

func TestSend_AlarmCommand_NoAuthHeader_WhenTokenEmpty(t *testing.T) {
	var gotAuth string
	s := newTestSender(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotAuth = r.Header.Get("Authorization")
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	}))
	cmd := domain.SetAlarmCommand{DeviceID: "dev1", AlarmTime: time.Date(2030, 1, 1, 8, 0, 0, 0, time.UTC), Label: "wake"}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotAuth != "" {
		t.Fatalf("expected no auth header, got %q", gotAuth)
	}
}

func TestSend_AlarmCommand_ContentTypeJSON(t *testing.T) {
	var gotContentType string
	s := newTestSender(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotContentType = r.Header.Get("Content-Type")
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	}))
	cmd := domain.SetAlarmCommand{DeviceID: "dev1", AlarmTime: time.Date(2030, 1, 1, 8, 0, 0, 0, time.UTC)}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotContentType != "application/json" {
		t.Fatalf("expected application/json, got %q", gotContentType)
	}
}

func TestSend_AlarmCommand_AlarmTimeFormat(t *testing.T) {
	var gotBody map[string]any
	s := newTestSender(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	}))
	alarmTime := time.Date(2030, 6, 15, 9, 30, 0, 0, time.UTC)
	cmd := domain.SetAlarmCommand{DeviceID: "dev1", AlarmTime: alarmTime, Label: "meeting"}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotBody["alarmTime"] != alarmTime.Format(time.RFC3339) {
		t.Fatalf("expected alarmTime %s, got %v", alarmTime.Format(time.RFC3339), gotBody["alarmTime"])
	}
}

// ── Send: message command ────────────────────────────────────────────────────

func TestSend_MessageCommand_MapsCorrectly(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	s := newTestSender(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	}))
	cmd := domain.DisplayMessageCommand{DeviceID: "clock-5", Message: "Hello!", DurationSeconds: 30}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/clocks/clock-5/messages" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotBody["message"] != "Hello!" {
		t.Fatalf("expected message Hello!, got %v", gotBody["message"])
	}
	// JSON numbers decode as float64
	if gotBody["durationSeconds"] != float64(30) {
		t.Fatalf("expected durationSeconds 30, got %v", gotBody["durationSeconds"])
	}
}

// ── Send: brightness command ─────────────────────────────────────────────────

func TestSend_BrightnessCommand_MapsCorrectly(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	s := newTestSender(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	}))
	cmd := domain.SetBrightnessCommand{DeviceID: "clock-7", Level: 75}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("expected PUT, got %s", gotMethod)
	}
	if gotPath != "/clocks/clock-7/brightness" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotBody["level"] != float64(75) {
		t.Fatalf("expected level 75, got %v", gotBody["level"])
	}
}

func TestSend_BrightnessCommand_ZeroLevel(t *testing.T) {
	var gotBody map[string]any
	s := newTestSender(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	}))
	cmd := domain.SetBrightnessCommand{DeviceID: "dev1", Level: 0}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotBody["level"] != float64(0) {
		t.Fatalf("expected level 0, got %v", gotBody["level"])
	}
}

func TestSend_BrightnessCommand_MaxLevel(t *testing.T) {
	var gotBody map[string]any
	s := newTestSender(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	}))
	cmd := domain.SetBrightnessCommand{DeviceID: "dev1", Level: 100}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotBody["level"] != float64(100) {
		t.Fatalf("expected level 100, got %v", gotBody["level"])
	}
}

// ── Send: unsupported command type ───────────────────────────────────────────

type unknownCommand struct{}

func (u unknownCommand) Execute(_ context.Context) error { return nil }
func (u unknownCommand) TargetDeviceID() string          { return "dev" }
func (u unknownCommand) CommandType() string             { return "unknown" }
func (u unknownCommand) Validate() error                 { return nil }

func TestSend_UnsupportedCommandType(t *testing.T) {
	s := newTestSender(t, okTransport(200, ""))
	err := s.Send(context.Background(), unknownCommand{})
	if err == nil || !strings.Contains(err.Error(), "unsupported command type") {
		t.Fatalf("expected unsupported-command-type error, got %v", err)
	}
}

// ── Send: device ID URL-encoding ─────────────────────────────────────────────

func TestSend_DeviceID_URLEncoded(t *testing.T) {
	var gotPath string
	s := newTestSender(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.RawPath
		if gotPath == "" {
			gotPath = r.URL.Path
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	}))
	// Device IDs with special chars that need escaping are prevented by domain validation,
	// but the adapter must still handle alphanumeric-only IDs cleanly.
	cmd := domain.SetBrightnessCommand{DeviceID: "clock-99", Level: 50}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.Contains(gotPath, "clock-99") {
		t.Fatalf("expected device id in path, got %q", gotPath)
	}
}

// ── Send: non-2xx HTTP status codes ─────────────────────────────────────────

func TestSend_Non2xxStatusCodes(t *testing.T) {
	codes := []int{400, 401, 403, 404, 422, 429, 500, 502, 503, 504}
	for _, code := range codes {
		code := code
		t.Run(fmt.Sprintf("status_%d", code), func(t *testing.T) {
			s := newTestSender(t, okTransport(code, fmt.Sprintf(`{"error":"code %d"}`, code)))
			cmd := domain.SetBrightnessCommand{DeviceID: "dev1", Level: 50}
			err := s.Send(context.Background(), cmd)
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", code)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("status=%d", code)) {
				t.Fatalf("expected status code in error for %d, got %v", code, err)
			}
		})
	}
}

func TestSend_Non2xx_IncludesBodyInError(t *testing.T) {
	s := newTestSender(t, okTransport(500, "internal server error details"))
	cmd := domain.SetBrightnessCommand{DeviceID: "dev1", Level: 50}
	err := s.Send(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "internal server error details") {
		t.Fatalf("expected body in error message, got %v", err)
	}
}

func TestSend_Non2xx_LargeBodyTruncated(t *testing.T) {
	// Body larger than the 1024-byte limit should not cause a hang/panic.
	largeBody := strings.Repeat("x", 8192)
	s := newTestSender(t, okTransport(500, largeBody))
	cmd := domain.SetBrightnessCommand{DeviceID: "dev1", Level: 50}
	err := s.Send(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Error body should be capped at 1024 bytes.
	if len(err.Error()) > 1100 { // some slack for the prefix text
		t.Fatalf("error message seems suspiciously long (%d chars), body may not be truncated", len(err.Error()))
	}
}

func TestSend_2xxBoundary_200(t *testing.T) {
	s := newTestSender(t, okTransport(200, `{"ok":true}`))
	cmd := domain.SetBrightnessCommand{DeviceID: "dev1", Level: 50}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("expected success for 200, got %v", err)
	}
}

func TestSend_2xxBoundary_201(t *testing.T) {
	s := newTestSender(t, okTransport(201, ""))
	cmd := domain.SetBrightnessCommand{DeviceID: "dev1", Level: 50}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("expected success for 201, got %v", err)
	}
}

func TestSend_2xxBoundary_299(t *testing.T) {
	s := newTestSender(t, okTransport(299, ""))
	cmd := domain.SetBrightnessCommand{DeviceID: "dev1", Level: 50}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("expected success for 299, got %v", err)
	}
}

func TestSend_300_IsError(t *testing.T) {
	s := newTestSender(t, okTransport(300, ""))
	cmd := domain.SetBrightnessCommand{DeviceID: "dev1", Level: 50}
	if err := s.Send(context.Background(), cmd); err == nil {
		t.Fatal("expected error for 300, got nil")
	}
}

// ── Send: network / transport errors ─────────────────────────────────────────

func TestSend_NetworkError(t *testing.T) {
	networkErr := errors.New("connection refused")
	s := newTestSender(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, networkErr
	}))
	cmd := domain.SetBrightnessCommand{DeviceID: "dev1", Level: 50}
	err := s.Send(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
	if !strings.Contains(err.Error(), "send rest request") {
		t.Fatalf("expected wrapped network error, got %v", err)
	}
}

func TestSend_TimeoutError(t *testing.T) {
	s := newTestSender(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		// Simulate a timeout by returning a context deadline exceeded error.
		return nil, fmt.Errorf("context deadline exceeded (Client.Timeout exceeded while awaiting headers)")
	}))
	cmd := domain.SetBrightnessCommand{DeviceID: "dev1", Level: 50}
	err := s.Send(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "send rest request") {
		t.Fatalf("expected wrapped timeout error, got %v", err)
	}
}

func TestSend_ContextCancelled(t *testing.T) {
	s := newTestSender(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, context.Canceled
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	cmd := domain.SetBrightnessCommand{DeviceID: "dev1", Level: 50}
	err := s.Send(ctx, cmd)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestSend_ContextDeadlineExceeded(t *testing.T) {
	s := newTestSender(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	cmd := domain.SetBrightnessCommand{DeviceID: "dev1", Level: 50}
	err := s.Send(ctx, cmd)
	if err == nil {
		t.Fatal("expected error for deadline exceeded, got nil")
	}
}

// ── Send: response body read error ───────────────────────────────────────────

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }
func (e errReader) Close() error             { return nil }

func TestSend_Non2xx_BodyReadError(t *testing.T) {
	// When body read fails on a non-2xx response, Send should still return an error
	// containing the status code (body message will be empty/partial).
	s := newTestSender(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 500,
			Body:       errReader{err: errors.New("read error")},
			Header:     make(http.Header),
		}, nil
	}))
	cmd := domain.SetBrightnessCommand{DeviceID: "dev1", Level: 50}
	err := s.Send(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "status=500") {
		t.Fatalf("expected status=500 in error, got %v", err)
	}
}

// ── Send: large / malformed response bodies (2xx) ────────────────────────────

func TestSend_2xx_LargeResponseBodyDoesNotFail(t *testing.T) {
	// The sender does not read the body on success; it should close it cleanly.
	largeBody := strings.Repeat("z", 1<<20) // 1 MB
	s := newTestSender(t, okTransport(200, largeBody))
	cmd := domain.SetBrightnessCommand{DeviceID: "dev1", Level: 50}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("expected success with large body, got %v", err)
	}
}

func TestSend_2xx_MalformedJSONResponseDoesNotFail(t *testing.T) {
	s := newTestSender(t, okTransport(200, "{not valid json at all!!!"))
	cmd := domain.SetBrightnessCommand{DeviceID: "dev1", Level: 50}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("expected success (sender doesn't parse response body), got %v", err)
	}
}

// ── Check (health) ───────────────────────────────────────────────────────────

func TestCheck_NoHealthPath_ReturnsNil(t *testing.T) {
	s, err := NewSender(Config{BaseURL: "http://clock-api.local", AllowInsecureHTTP: true})
	if err != nil {
		t.Fatalf("new sender: %v", err)
	}
	// Transport should never be called if healthPath is empty.
	s.client = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			t.Fatal("transport should not be called when healthPath is empty")
			return nil, nil
		}),
	}
	if err := s.Check(context.Background()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestCheck_HealthPath_Success(t *testing.T) {
	var gotPath string
	s, err := NewSender(Config{BaseURL: "http://clock-api.local", AllowInsecureHTTP: true, HealthPath: "/health"})
	if err != nil {
		t.Fatalf("new sender: %v", err)
	}
	s.client = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotPath = r.URL.Path
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		}),
	}
	if err := s.Check(context.Background()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if gotPath != "/health" {
		t.Fatalf("expected /health, got %q", gotPath)
	}
}

func TestCheck_HealthPath_Non2xx(t *testing.T) {
	s, err := NewSender(Config{BaseURL: "http://clock-api.local", AllowInsecureHTTP: true, HealthPath: "/health"})
	if err != nil {
		t.Fatalf("new sender: %v", err)
	}
	s.client = &http.Client{
		Transport: okTransport(503, "service unavailable"),
	}
	err = s.Check(context.Background())
	if err == nil {
		t.Fatal("expected error for 503 health check, got nil")
	}
	if !strings.Contains(err.Error(), "readiness status=503") {
		t.Fatalf("expected readiness-status error, got %v", err)
	}
}

func TestCheck_HealthPath_NetworkError(t *testing.T) {
	s, err := NewSender(Config{BaseURL: "http://clock-api.local", AllowInsecureHTTP: true, HealthPath: "/health"})
	if err != nil {
		t.Fatalf("new sender: %v", err)
	}
	s.client = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("connection refused")
		}),
	}
	err = s.Check(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "readiness request failed") {
		t.Fatalf("expected readiness-request-failed error, got %v", err)
	}
}

func TestCheck_HealthPath_SendsAuthHeader(t *testing.T) {
	var gotAuth string
	s, err := NewSender(Config{
		BaseURL:           "http://clock-api.local",
		AllowInsecureHTTP: true,
		HealthPath:        "/health",
		AuthToken:         "check-token",
	})
	if err != nil {
		t.Fatalf("new sender: %v", err)
	}
	s.client = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotAuth = r.Header.Get("Authorization")
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		}),
	}
	if err := s.Check(context.Background()); err != nil {
		t.Fatalf("check: %v", err)
	}
	if gotAuth != "Bearer check-token" {
		t.Fatalf("expected Bearer token on health check, got %q", gotAuth)
	}
}

func TestCheck_HealthPath_UsesGETMethod(t *testing.T) {
	var gotMethod string
	s, err := NewSender(Config{BaseURL: "http://clock-api.local", AllowInsecureHTTP: true, HealthPath: "/health"})
	if err != nil {
		t.Fatalf("new sender: %v", err)
	}
	s.client = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotMethod = r.Method
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		}),
	}
	if err := s.Check(context.Background()); err != nil {
		t.Fatalf("check: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("expected GET for health check, got %s", gotMethod)
	}
}
