package main

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestGetEnv(t *testing.T) {
	key := "CLOCKCTL_TEST_GETENV"
	_ = os.Unsetenv(key)
	if got := getEnv(key, "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}
	if err := os.Setenv(key, " value "); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	t.Cleanup(func() { _ = os.Unsetenv(key) })
	if got := getEnv(key, "fallback"); got != "value" {
		t.Fatalf("expected trimmed env value, got %q", got)
	}
}

func TestParseTimeout(t *testing.T) {
	key := "CLOCKCTL_TEST_TIMEOUT"
	_ = os.Unsetenv(key)

	if got := parseTimeout(key, 3*time.Second); got != 3*time.Second {
		t.Fatalf("expected fallback, got %s", got)
	}

	if err := os.Setenv(key, "1200"); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	if got := parseTimeout(key, 3*time.Second); got != 1200*time.Millisecond {
		t.Fatalf("expected 1200ms, got %s", got)
	}

	if err := os.Setenv(key, "invalid"); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	if got := parseTimeout(key, 3*time.Second); got != 3*time.Second {
		t.Fatalf("expected fallback for invalid input, got %s", got)
	}

	t.Cleanup(func() { _ = os.Unsetenv(key) })
}

func TestAPIClientSendSuccess(t *testing.T) {
	client := &apiClient{
		baseURL: "http://clock-server.local",
		client: &http.Client{
			Timeout: 2 * time.Second,
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodPost {
					t.Fatalf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != "/commands/messages" {
					t.Fatalf("expected messages path, got %s", r.URL.Path)
				}
				return &http.Response{
					StatusCode: http.StatusAccepted,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewBufferString(`{"ok":true}`)),
				}, nil
			}),
		},
	}
	if err := client.send(http.MethodPost, "/commands/messages", map[string]any{"deviceId": "clock-1"}); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
}

func TestAPIClientSendFailureStatus(t *testing.T) {
	client := &apiClient{
		baseURL: "http://clock-server.local",
		client: &http.Client{
			Timeout: 2 * time.Second,
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewBufferString(`{"error":"bad payload"}`)),
				}, nil
			}),
		},
	}
	err := client.send(http.MethodPut, "/commands/brightness", map[string]any{"deviceId": "clock-1", "level": 101})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "status=400") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveServerBaseURL(t *testing.T) {
	t.Setenv("CLOCK_SERVER_BASE_URL", "")
	t.Setenv("CLOCK_SERVER_HOST", "")
	if got := resolveServerBaseURL(); got != "http://localhost:8080" {
		t.Fatalf("expected default url, got %q", got)
	}

	t.Setenv("CLOCK_SERVER_HOST", "tcp://clock-host:9999")
	if got := resolveServerBaseURL(); got != "https://clock-host:9999" {
		t.Fatalf("expected tcp mapping, got %q", got)
	}

	t.Setenv("CLOCK_SERVER_HOST", "https://clock.example.com")
	if got := resolveServerBaseURL(); got != "https://clock.example.com" {
		t.Fatalf("expected https passthrough, got %q", got)
	}

	t.Setenv("CLOCK_SERVER_HOST", "clock-host:8081")
	if got := resolveServerBaseURL(); got != "https://clock-host:8081" {
		t.Fatalf("expected bare host mapping, got %q", got)
	}
}

func TestResolveServerBaseURLExplicitOverride(t *testing.T) {
	t.Setenv("CLOCK_SERVER_HOST", "tcp://clock-host:8080")
	t.Setenv("CLOCK_SERVER_BASE_URL", "https://override.example.com/")

	if got := resolveServerBaseURL(); got != "https://override.example.com" {
		t.Fatalf("expected explicit base url to win, got %q", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestAPIClientRejectsInsecureTokenTransport(t *testing.T) {
	client := &apiClient{
		baseURL: "http://clock-server.local",
		token:   "secret",
		client:  &http.Client{Timeout: 2 * time.Second},
	}
	err := client.send(http.MethodPost, "/commands/messages", map[string]any{"deviceId": "clock-1"})
	if err == nil {
		t.Fatal("expected insecure transport error")
	}
	if !strings.Contains(err.Error(), "refusing to send bearer token") {
		t.Fatalf("unexpected error: %v", err)
	}
}
