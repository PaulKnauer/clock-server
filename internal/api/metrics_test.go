package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"

	"github.com/paul/clock-server/internal/application"
	"github.com/paul/clock-server/internal/metrics"
	"github.com/paul/clock-server/internal/security"
)

// newTestHandlerWithMetrics builds a Handler wired up with a fresh Metrics instance.
func newTestHandlerWithMetrics(sender application.ClockCommandSender) (*Handler, *metrics.Metrics) {
	m := metrics.NewMetrics()
	dispatcher := application.NewCommandDispatcher(sender)
	h := NewHandlerWithOptions(
		dispatcher,
		security.NewCredentialStore([]security.Credential{{ID: "test", Token: "test-token", Devices: []string{"*"}}}),
		false,
		false,
		true,
		64*1024,
		100,
		HandlerOptions{
			Metrics:            m,
			MetricsRequireAuth: false,
		},
	)
	return h, m
}

// gatherCounterVec collects label values → counter value for a named metric family.
func gatherCounterVec(t *testing.T, m *metrics.Metrics, name string) map[string]float64 {
	t.Helper()
	mfs, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("gather error: %v", err)
	}
	result := map[string]float64{}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, metric := range mf.GetMetric() {
			key := labelsToKey(metric.GetLabel())
			val := getMetricValue(metric)
			result[key] = val
		}
	}
	return result
}

func labelsToKey(labels []*dto.LabelPair) string {
	parts := make([]string, 0, len(labels))
	for _, lp := range labels {
		parts = append(parts, lp.GetName()+"="+lp.GetValue())
	}
	return strings.Join(parts, ",")
}

func getMetricValue(m *dto.Metric) float64 {
	if m.Counter != nil && m.Counter.Value != nil {
		return *m.Counter.Value
	}
	if m.Gauge != nil && m.Gauge.Value != nil {
		return *m.Gauge.Value
	}
	return 0
}

// TestRequestsTotalIncrementOnSuccess asserts that a successful request increments
// clock_requests_total with the correct method, path, and status labels.
func TestRequestsTotalIncrementOnSuccess(t *testing.T) {
	h, m := newTestHandlerWithMetrics(&stubSender{})
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	counts := gatherCounterVec(t, m, "clock_requests_total")
	key := "method=GET,path=/health,status=200"
	if counts[key] != 1 {
		t.Errorf("clock_requests_total{%s} = %v, want 1", key, counts[key])
	}
}

// TestAuthFailureMissingToken asserts that a request without a token increments
// clock_auth_failures_total with reason=missing_token.
func TestAuthFailureMissingToken(t *testing.T) {
	h, m := newTestHandlerWithMetrics(&stubSender{})
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}

	counts := gatherCounterVec(t, m, "clock_auth_failures_total")
	key := "reason=missing_token"
	if counts[key] != 1 {
		t.Errorf("clock_auth_failures_total{%s} = %v, want 1", key, counts[key])
	}
}

// TestAuthFailureInvalidToken asserts that a request with a bad token increments
// clock_auth_failures_total with reason=invalid_token.
func TestAuthFailureInvalidToken(t *testing.T) {
	h, m := newTestHandlerWithMetrics(&stubSender{})
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}

	counts := gatherCounterVec(t, m, "clock_auth_failures_total")
	key := "reason=invalid_token"
	if counts[key] != 1 {
		t.Errorf("clock_auth_failures_total{%s} = %v, want 1", key, counts[key])
	}
}

// TestAuthFailureRateLimited asserts that when the rate limiter blocks a request
// clock_auth_failures_total is incremented with reason=rate_limited.
func TestAuthFailureRateLimited(t *testing.T) {
	m := metrics.NewMetrics()
	dispatcher := application.NewCommandDispatcher(&stubSender{})
	h := NewHandlerWithOptions(
		dispatcher,
		security.NewCredentialStore([]security.Credential{{ID: "test", Token: "test-token", Devices: []string{"*"}}}),
		false,
		false,
		true,
		64*1024,
		1, // limit: 1 failure before blocking
		HandlerOptions{
			Metrics:            m,
			MetricsRequireAuth: false,
		},
	)
	router := h.Routes()

	// First request — triggers missing_token failure and records it.
	req1 := httptest.NewRequest(http.MethodGet, "/ready", nil)
	req1.RemoteAddr = "10.0.0.1:1234"
	rr1 := httptest.NewRecorder()
	router.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusUnauthorized {
		t.Fatalf("first request: expected 401, got %d", rr1.Code)
	}

	// Second request — should be rate-limited (429).
	req2 := httptest.NewRequest(http.MethodGet, "/ready", nil)
	req2.RemoteAddr = "10.0.0.1:1234"
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d", rr2.Code)
	}

	counts := gatherCounterVec(t, m, "clock_auth_failures_total")
	if counts["reason=rate_limited"] < 1 {
		t.Errorf("clock_auth_failures_total{reason=rate_limited} = %v, want >= 1", counts["reason=rate_limited"])
	}
}

// TestMetricsEndpointReturns200 asserts that GET /metrics returns 200 with
// a Prometheus text content type.
func TestMetricsEndpointReturns200(t *testing.T) {
	h, _ := newTestHandlerWithMetrics(&stubSender{})
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected text/plain content type, got %q", ct)
	}
}

// TestMetricsEndpointNotBlockedByRateLimiter asserts that repeated unauthenticated
// requests to /metrics do NOT trigger rate limiting (429), even after the limiter
// threshold is exceeded on other paths.
func TestMetricsEndpointNotBlockedByRateLimiter(t *testing.T) {
	m := metrics.NewMetrics()
	dispatcher := application.NewCommandDispatcher(&stubSender{})
	h := NewHandlerWithOptions(
		dispatcher,
		security.NewCredentialStore([]security.Credential{{ID: "test", Token: "test-token", Devices: []string{"*"}}}),
		false,
		false,
		true,
		64*1024,
		1, // very tight limit
		HandlerOptions{
			Metrics:            m,
			MetricsRequireAuth: false, // /metrics requires no auth → no rate limiting
		},
	)
	router := h.Routes()

	ip := "10.0.0.2:5678"

	// Exhaust the rate limiter for this IP via /ready.
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		req.RemoteAddr = ip
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
	}

	// /metrics must still be accessible from the same IP.
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = ip
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code == http.StatusTooManyRequests {
		t.Fatal("/metrics was blocked by the rate limiter — should be excluded")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("/metrics: expected 200, got %d", rr.Code)
	}
}

// TestMetricsEndpointRequiresAuthWhenConfigured asserts that when metricsRequireAuth
// is true, an unauthenticated request to /metrics returns 401.
func TestMetricsEndpointRequiresAuthWhenConfigured(t *testing.T) {
	m := metrics.NewMetrics()
	dispatcher := application.NewCommandDispatcher(&stubSender{})
	h := NewHandlerWithOptions(
		dispatcher,
		security.NewCredentialStore([]security.Credential{{ID: "test", Token: "test-token", Devices: []string{"*"}}}),
		false,
		false,
		true,
		64*1024,
		100,
		HandlerOptions{
			Metrics:            m,
			MetricsRequireAuth: true,
		},
	)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when MetricsRequireAuth=true, got %d", rr.Code)
	}
}

// TestRequestsTotalIncrementOnCommandSuccess asserts that a successful command
// request increments clock_requests_total with status=202.
func TestRequestsTotalIncrementOnCommandSuccess(t *testing.T) {
	h, m := newTestHandlerWithMetrics(&stubSender{})
	router := h.Routes()

	body := []byte(`{"deviceId":"clock-1","alarmTime":"2030-01-01T07:00:00Z","label":"wake"}`)
	req := httptest.NewRequest(http.MethodPost, "/commands/alarms", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}

	counts := gatherCounterVec(t, m, "clock_requests_total")
	key := "method=POST,path=/commands/alarms,status=202"
	if counts[key] != 1 {
		t.Errorf("clock_requests_total{%s} = %v, want 1", key, counts[key])
	}
}
