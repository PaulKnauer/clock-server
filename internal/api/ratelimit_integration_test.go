package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/paul/clock-server/internal/application"
	"github.com/paul/clock-server/internal/security"
)

// newRateLimitTestHandler returns a handler with the specified per-minute
// auth failure limit and a single valid credential.
func newRateLimitTestHandler(limitPerMin int) *Handler {
	dispatcher := application.NewCommandDispatcher(&stubSender{})
	return NewHandler(
		dispatcher,
		security.NewCredentialStore([]security.Credential{
			{ID: "legit", Token: "valid-token", Devices: []string{"*"}},
		}),
		false, // trustProxyTLS
		false, // requireTLS
		true,  // readinessRequireAuth
		64*1024,
		limitPerMin,
	)
}

// TestInMemoryRateLimiter_65FailedRequestsTrigger429 fires 65 bad-token requests
// from a single IP within a single window and asserts that the in-memory rate
// limiter returns 429 once the limit is exceeded.
//
// This exercises the per-replica defence-in-depth limiter that remains active
// regardless of whether Traefik ingress-layer rate limiting is configured.
func TestInMemoryRateLimiter_65FailedRequestsTrigger429(t *testing.T) {
	const limit = 60
	h := newRateLimitTestHandler(limit)
	router := h.Routes()

	const sourceIP = "203.0.113.42:55000"
	const badToken = "wrong-token-attacker"

	got429 := false
	for i := 0; i < 65; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		req.RemoteAddr = sourceIP
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", badToken))
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		switch rr.Code {
		case http.StatusUnauthorized:
			// Expected for the first `limit` requests.
		case http.StatusTooManyRequests:
			got429 = true
		default:
			t.Fatalf("request %d: unexpected status %d", i, rr.Code)
		}
	}

	if !got429 {
		t.Fatal("expected at least one 429 Too Many Requests after 65 failed auth attempts, got none")
	}
}

// TestInMemoryRateLimiter_LegitimateIPNotBlocked verifies that a conceptually
// different source IP is not blocked when another IP exhausts its rate limit.
//
// This is the multi-source isolation test required by AC-5.
func TestInMemoryRateLimiter_LegitimateIPNotBlocked(t *testing.T) {
	const limit = 10
	h := newRateLimitTestHandler(limit)
	router := h.Routes()

	const attackerIP = "198.51.100.1:1111"
	const legitimateIP = "198.51.100.2:2222"
	const badToken = "wrong-token-attacker"

	// Exhaust the attacker's rate limit.
	for i := 0; i <= limit; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		req.RemoteAddr = attackerIP
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", badToken))
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
	}

	// Verify the attacker is now blocked.
	checkReq := httptest.NewRequest(http.MethodGet, "/ready", nil)
	checkReq.RemoteAddr = attackerIP
	checkReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", badToken))
	checkRR := httptest.NewRecorder()
	router.ServeHTTP(checkRR, checkReq)
	if checkRR.Code != http.StatusTooManyRequests {
		t.Fatalf("attacker expected 429, got %d — rate limiter did not engage", checkRR.Code)
	}

	// The legitimate IP with a valid token must not be blocked.
	legitReq := httptest.NewRequest(http.MethodGet, "/ready", nil)
	legitReq.RemoteAddr = legitimateIP
	legitReq.Header.Set("Authorization", "Bearer valid-token")
	legitRR := httptest.NewRecorder()
	router.ServeHTTP(legitRR, legitReq)

	if legitRR.Code == http.StatusTooManyRequests {
		t.Fatal("legitimate IP was rate-limited (429) — cross-IP isolation failure")
	}
	if legitRR.Code != http.StatusOK {
		t.Fatalf("legitimate IP expected 200, got %d", legitRR.Code)
	}
}
