package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/paul/clock-server/internal/application"
	"github.com/paul/clock-server/internal/domain"
	"github.com/paul/clock-server/internal/security"
)

const defaultBodyLimit = 64 * 1024

type contextKey string

const (
	principalContextKey contextKey = "principal"
)

// Handler exposes REST endpoints for issuing smart clock commands.
type Handler struct {
	dispatcher             *application.CommandDispatcher
	credentials            []security.Credential
	trustProxyTLS          bool
	requireTLS             bool
	readinessRequireAuth   bool
	maxBodyBytes           int64
	authFailureRateLimiter *authFailureLimiter
	requestCounter         uint64
	checkers               []application.ReadinessChecker
}

// NewHandler builds a new API handler.
func NewHandler(
	dispatcher *application.CommandDispatcher,
	credentials []security.Credential,
	trustProxyTLS bool,
	requireTLS bool,
	readinessRequireAuth bool,
	maxBodyBytes int64,
	authFailLimitPerMinute int,
	checkers ...application.ReadinessChecker,
) *Handler {
	if maxBodyBytes <= 0 {
		maxBodyBytes = defaultBodyLimit
	}
	if authFailLimitPerMinute <= 0 {
		authFailLimitPerMinute = 60
	}
	return &Handler{
		dispatcher:             dispatcher,
		credentials:            credentials,
		trustProxyTLS:          trustProxyTLS,
		requireTLS:             requireTLS,
		readinessRequireAuth:   readinessRequireAuth,
		maxBodyBytes:           maxBodyBytes,
		authFailureRateLimiter: newAuthFailureLimiter(authFailLimitPerMinute),
		checkers:               checkers,
	}
}

// Routes returns the HTTP router for the command API.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/ready", h.handleReady)
	mux.HandleFunc("/commands/alarms", h.handleSetAlarm)
	mux.HandleFunc("/commands/messages", h.handleDisplayMessage)
	mux.HandleFunc("/commands/brightness", h.handleSetBrightness)
	return h.authMiddleware(mux)
}

type setAlarmRequest struct {
	DeviceID  string `json:"deviceId"`
	AlarmTime string `json:"alarmTime"`
	Label     string `json:"label"`
}

type displayMessageRequest struct {
	DeviceID        string `json:"deviceId"`
	Message         string `json:"message"`
	DurationSeconds int    `json:"durationSeconds"`
}

type setBrightnessRequest struct {
	DeviceID string `json:"deviceId"`
	Level    int    `json:"level"`
}

type principal struct {
	ID      string
	Devices []string
}

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	for _, checker := range h.checkers {
		if checker == nil {
			continue
		}
		if err := checker.Check(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handler) handleSetAlarm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var payload setAlarmRequest
	if err := h.decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.authorizeDevice(r.Context(), payload.DeviceID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}

	alarmTime, err := time.Parse(time.RFC3339, payload.AlarmTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("alarmTime must be RFC3339"))
		return
	}

	cmd := domain.SetAlarmCommand{
		DeviceID:  payload.DeviceID,
		AlarmTime: alarmTime,
		Label:     payload.Label,
	}
	if err := h.dispatcher.Dispatch(r.Context(), cmd); err != nil {
		h.audit(r, payload.DeviceID, cmd.CommandType(), "failed")
		writeAppError(w, err)
		return
	}

	h.audit(r, payload.DeviceID, cmd.CommandType(), "accepted")
	writeJSON(w, http.StatusAccepted, map[string]string{"result": "scheduled"})
}

func (h *Handler) handleDisplayMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var payload displayMessageRequest
	if err := h.decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.authorizeDevice(r.Context(), payload.DeviceID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}

	cmd := domain.DisplayMessageCommand{
		DeviceID:        payload.DeviceID,
		Message:         payload.Message,
		DurationSeconds: payload.DurationSeconds,
	}
	if err := h.dispatcher.Dispatch(r.Context(), cmd); err != nil {
		h.audit(r, payload.DeviceID, cmd.CommandType(), "failed")
		writeAppError(w, err)
		return
	}

	h.audit(r, payload.DeviceID, cmd.CommandType(), "accepted")
	writeJSON(w, http.StatusAccepted, map[string]string{"result": "sent"})
}

func (h *Handler) handleSetBrightness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		methodNotAllowed(w)
		return
	}

	var payload setBrightnessRequest
	if err := h.decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.authorizeDevice(r.Context(), payload.DeviceID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}

	cmd := domain.SetBrightnessCommand{
		DeviceID: payload.DeviceID,
		Level:    payload.Level,
	}
	if err := h.dispatcher.Dispatch(r.Context(), cmd); err != nil {
		h.audit(r, payload.DeviceID, cmd.CommandType(), "failed")
		writeAppError(w, err)
		return
	}

	h.audit(r, payload.DeviceID, cmd.CommandType(), "accepted")
	writeJSON(w, http.StatusAccepted, map[string]string{"result": "updated"})
}

func (h *Handler) decodeJSON(w http.ResponseWriter, r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodyBytes)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeAppError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, application.ErrValidation):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid command"})
	case errors.Is(err, application.ErrDownstream):
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "command dispatch failed"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	setSecurityHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-Id")
		if strings.TrimSpace(requestID) == "" {
			requestID = fmt.Sprintf("req-%d", atomic.AddUint64(&h.requestCounter, 1))
		}
		w.Header().Set("X-Request-Id", requestID)

		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		if h.requireTLS && !h.isSecureRequest(r) {
			writeJSON(w, http.StatusUpgradeRequired, map[string]string{"error": "https required"})
			return
		}

		if r.URL.Path == "/ready" && !h.readinessRequireAuth {
			next.ServeHTTP(w, r)
			return
		}

		remoteIP := clientIP(r)
		if h.authFailureRateLimiter.IsBlocked(remoteIP) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many auth failures"})
			return
		}

		authHeader := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(authHeader, prefix) {
			h.authFailureRateLimiter.RecordFailure(remoteIP)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(authHeader, prefix))
		principal, ok := h.lookupCredential(token)
		if !ok {
			h.authFailureRateLimiter.RecordFailure(remoteIP)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		ctx := context.WithValue(r.Context(), principalContextKey, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) lookupCredential(token string) (principal, bool) {
	for _, cred := range h.credentials {
		if subtle.ConstantTimeCompare([]byte(token), []byte(cred.Token)) == 1 {
			return principal{ID: cred.ID, Devices: cred.Devices}, true
		}
	}
	return principal{}, false
}

func (h *Handler) authorizeDevice(ctx context.Context, deviceID string) error {
	pr, ok := ctx.Value(principalContextKey).(principal)
	if !ok {
		return errors.New("unauthorized")
	}
	cred := security.Credential{ID: pr.ID, Devices: pr.Devices}
	if !cred.Allows(deviceID) {
		return errors.New("forbidden for target device")
	}
	return nil
}

func (h *Handler) isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if !h.trustProxyTLS {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func (h *Handler) audit(r *http.Request, deviceID, commandType, result string) {
	principalID := "unknown"
	if pr, ok := r.Context().Value(principalContextKey).(principal); ok {
		principalID = pr.ID
	}
	log.Printf("audit principal=%s remote=%s method=%s path=%s device=%s command=%s result=%s request_id=%s",
		principalID,
		clientIP(r),
		r.Method,
		r.URL.Path,
		deviceID,
		commandType,
		result,
		r.Header.Get("X-Request-Id"),
	)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}

type authFailureLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	entries map[string]authFailureEntry
}

type authFailureEntry struct {
	windowStart time.Time
	count       int
}

func newAuthFailureLimiter(limit int) *authFailureLimiter {
	return &authFailureLimiter{
		limit:   limit,
		window:  time.Minute,
		entries: make(map[string]authFailureEntry),
	}
}

func (l *authFailureLimiter) IsBlocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	entry, ok := l.entries[key]
	if !ok {
		return false
	}
	if now.Sub(entry.windowStart) >= l.window {
		delete(l.entries, key)
		return false
	}
	return entry.count >= l.limit
}

func (l *authFailureLimiter) RecordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	entry, ok := l.entries[key]
	if !ok || now.Sub(entry.windowStart) >= l.window {
		l.entries[key] = authFailureEntry{windowStart: now, count: 1}
		return
	}
	entry.count++
	l.entries[key] = entry
}
