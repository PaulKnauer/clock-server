package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paul/clock-server/internal/application"
	"github.com/paul/clock-server/internal/domain"
	"github.com/paul/clock-server/internal/security"
)

type stubSender struct {
	err     error
	calls   int
	lastCmd domain.ClockCommand
}

func (s *stubSender) Send(_ context.Context, cmd domain.ClockCommand) error {
	s.calls++
	s.lastCmd = cmd
	return s.err
}

func newTestHandler(sender application.ClockCommandSender) *Handler {
	dispatcher := application.NewCommandDispatcher(sender)
	return NewHandler(
		dispatcher,
		[]security.Credential{{ID: "test", Token: "test-token", Devices: []string{"*"}}},
		false,
		false,
		true,
		64*1024,
		100,
	)
}

func TestRoutesHealth(t *testing.T) {
	h := newTestHandler(&stubSender{})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("expected json content type, got %q", ct)
	}
}

func TestSetAlarmUnauthorized(t *testing.T) {
	h := newTestHandler(&stubSender{})

	req := httptest.NewRequest(http.MethodGet, "/commands/alarms", nil)
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rr.Code)
	}
}

func TestSetAlarmMethodNotAllowed(t *testing.T) {
	h := newTestHandler(&stubSender{})

	req := httptest.NewRequest(http.MethodGet, "/commands/alarms", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rr.Code)
	}
}

func TestSetAlarmInvalidTime(t *testing.T) {
	h := newTestHandler(&stubSender{})
	body := []byte(`{"deviceId":"clock-1","alarmTime":"not-a-time","label":"wake"}`)

	req := httptest.NewRequest(http.MethodPost, "/commands/alarms", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

func TestSetAlarmAccepted(t *testing.T) {
	sender := &stubSender{}
	h := newTestHandler(sender)
	body := []byte(`{"deviceId":"clock-1","alarmTime":"2030-01-01T07:00:00Z","label":"wake"}`)

	req := httptest.NewRequest(http.MethodPost, "/commands/alarms", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", rr.Code)
	}
	if sender.calls != 1 {
		t.Fatalf("expected sender called once, got %d", sender.calls)
	}
	if _, ok := sender.lastCmd.(domain.SetAlarmCommand); !ok {
		t.Fatalf("expected SetAlarmCommand, got %T", sender.lastCmd)
	}
}

func TestSetBrightnessValidationErrorReturnsBadRequest(t *testing.T) {
	h := newTestHandler(&stubSender{})
	body := []byte(`{"deviceId":"clock-1","level":101}`)

	req := httptest.NewRequest(http.MethodPut, "/commands/brightness", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

func TestSetMessageSenderFailureReturnsBadGateway(t *testing.T) {
	h := newTestHandler(&stubSender{err: errors.New("downstream unavailable")})
	body := []byte(`{"deviceId":"clock-1","message":"hi","durationSeconds":10}`)

	req := httptest.NewRequest(http.MethodPost, "/commands/messages", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d", rr.Code)
	}

	var payload map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload["error"] != "command dispatch failed" {
		t.Fatalf("unexpected error message: %q", payload["error"])
	}
}

func TestDeviceScopeForbidden(t *testing.T) {
	dispatcher := application.NewCommandDispatcher(&stubSender{})
	h := NewHandler(
		dispatcher,
		[]security.Credential{{ID: "ops", Token: "scoped-token", Devices: []string{"clock-allowed"}}},
		false,
		false,
		true,
		64*1024,
		100,
	)
	body := []byte(`{"deviceId":"clock-denied","message":"hi","durationSeconds":10}`)

	req := httptest.NewRequest(http.MethodPost, "/commands/messages", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer scoped-token")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rr.Code)
	}
}

func TestRoutesRequireTLS(t *testing.T) {
	dispatcher := application.NewCommandDispatcher(&stubSender{})
	h := NewHandler(
		dispatcher,
		[]security.Credential{{ID: "test", Token: "test-token", Devices: []string{"*"}}},
		false,
		true,
		true,
		64*1024,
		100,
	)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusUpgradeRequired {
		t.Fatalf("expected status 426, got %d", rr.Code)
	}
}

func TestRoutesTrustProxyTLS(t *testing.T) {
	dispatcher := application.NewCommandDispatcher(&stubSender{})
	h := NewHandler(
		dispatcher,
		[]security.Credential{{ID: "test", Token: "test-token", Devices: []string{"*"}}},
		true,
		true,
		true,
		64*1024,
		100,
	)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
}

func TestReadyDoesNotRequireAuthWhenDisabled(t *testing.T) {
	dispatcher := application.NewCommandDispatcher(&stubSender{})
	h := NewHandler(
		dispatcher,
		[]security.Credential{{ID: "test", Token: "test-token", Devices: []string{"*"}}},
		false,
		false,
		false,
		64*1024,
		100,
	)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
}

func TestAuthFailureRateLimit(t *testing.T) {
	dispatcher := application.NewCommandDispatcher(&stubSender{})
	h := NewHandler(
		dispatcher,
		[]security.Credential{{ID: "test", Token: "test-token", Devices: []string{"*"}}},
		false,
		false,
		true,
		64*1024,
		1,
	)

	req1 := httptest.NewRequest(http.MethodGet, "/ready", nil)
	req1.RemoteAddr = "203.0.113.10:1234"
	rr1 := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusUnauthorized {
		t.Fatalf("expected first status 401, got %d", rr1.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/ready", nil)
	req2.RemoteAddr = "203.0.113.10:1234"
	rr2 := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second status 429, got %d", rr2.Code)
	}
}

func TestSharedIPThrottlingDoesNotBlockOtherTokens(t *testing.T) {
	// Two callers behind the same NAT IP use different tokens.
	// One attacker fails auth repeatedly; the legitimate user must
	// NOT be collaterally blocked (no 429).
	dispatcher := application.NewCommandDispatcher(&stubSender{})
	validToken := "valid-secret-token"
	h := NewHandler(
		dispatcher,
		[]security.Credential{{ID: "legit", Token: validToken, Devices: []string{"*"}}},
		false,
		false,
		true,
		64*1024,
		2, // allow only 2 failures per key per minute
	)

	sharedIP := "198.51.100.1:9999"
	attackerToken := "wrong-token-attacker"
	router := h.Routes()

	// Attacker: fail auth 3 times with a bad token from the shared IP.
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		req.RemoteAddr = sharedIP
		req.Header.Set("Authorization", "Bearer "+attackerToken)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		// First two should be 401, third should be 429.
		if i < 2 && rr.Code != http.StatusUnauthorized {
			t.Fatalf("attacker request %d: expected 401, got %d", i, rr.Code)
		}
		if i == 2 && rr.Code != http.StatusTooManyRequests {
			t.Fatalf("attacker request %d: expected 429, got %d", i, rr.Code)
		}
	}

	// Legitimate user: same source IP, different (valid) token.
	// Must NOT receive 429.
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	req.RemoteAddr = sharedIP
	req.Header.Set("Authorization", "Bearer "+validToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code == http.StatusTooManyRequests {
		t.Fatalf("legitimate user behind shared IP got 429 — collateral DoS not fixed")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("legitimate user expected 200, got %d", rr.Code)
	}
}
