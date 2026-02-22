package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/paul/clock-server/internal/application"
	"github.com/paul/clock-server/internal/domain"
	"github.com/paul/clock-server/internal/security"
)

type bddStubSender struct {
	calls   int
	lastCmd domain.ClockCommand
	err     error
}

func (s *bddStubSender) Send(_ context.Context, cmd domain.ClockCommand) error {
	s.calls++
	s.lastCmd = cmd
	return s.err
}

type bddReadinessChecker struct {
	err error
}

func (c bddReadinessChecker) Check(_ context.Context) error {
	return c.err
}

type bddWorld struct {
	sender *bddStubSender

	handler               http.Handler
	credentials           []security.Credential
	trustProxyTLS         bool
	requireTLS            bool
	readinessRequireAuth  bool
	maxBodyBytes          int64
	authFailLimitPerMin   int
	readinessCheckerError error
	senderError           error

	bearerToken    string
	requestHeaders map[string]string
	remoteAddr     string

	response *httptest.ResponseRecorder
}

func (w *bddWorld) reset() {
	w.sender = &bddStubSender{}
	w.handler = nil
	w.credentials = []security.Credential{{ID: "test", Token: "test-token", Devices: []string{"*"}}}
	w.trustProxyTLS = false
	w.requireTLS = false
	w.readinessRequireAuth = true
	w.maxBodyBytes = 64 * 1024
	w.authFailLimitPerMin = 100
	w.readinessCheckerError = nil
	w.senderError = nil
	w.bearerToken = ""
	w.requestHeaders = map[string]string{}
	w.remoteAddr = "203.0.113.1:1234"
	w.response = nil
}

func (w *bddWorld) ensureHandler() {
	if w.handler != nil {
		return
	}

	w.sender.err = w.senderError
	dispatcher := application.NewCommandDispatcher(w.sender)
	var checkers []application.ReadinessChecker
	if w.readinessCheckerError != nil {
		checkers = append(checkers, bddReadinessChecker{err: w.readinessCheckerError})
	}

	h := NewHandler(
		dispatcher,
		w.credentials,
		w.trustProxyTLS,
		w.requireTLS,
		w.readinessRequireAuth,
		w.maxBodyBytes,
		w.authFailLimitPerMin,
		checkers...,
	)
	w.handler = h.Routes()
}

func (w *bddWorld) apiHandlerIsRunning() {
	w.ensureHandler()
}

func (w *bddWorld) apiHandlerRequiresTLS() {
	w.requireTLS = true
	w.handler = nil
}

func (w *bddWorld) apiHandlerRequiresTLSAndTrustsProxyHeaders() {
	w.requireTLS = true
	w.trustProxyTLS = true
	w.handler = nil
}

func (w *bddWorld) apiHandlerAllowsUnauthenticatedReadinessChecks() {
	w.readinessRequireAuth = false
	w.handler = nil
}

func (w *bddWorld) apiHandlerHasFailingReadinessChecker() {
	w.readinessCheckerError = errors.New("dependency unavailable")
	w.handler = nil
}

func (w *bddWorld) apiHandlerRateLimitsAuthFailures(limit int) {
	w.authFailLimitPerMin = limit
	w.handler = nil
}

func (w *bddWorld) apiHandlerAuthorizesOnlyDevice(deviceID string) {
	w.credentials = []security.Credential{{ID: "ops", Token: "scoped-token", Devices: []string{deviceID}}}
	w.handler = nil
}

func (w *bddWorld) senderFailsDownstream() {
	w.senderError = errors.New("downstream unavailable")
	w.handler = nil
}

func (w *bddWorld) apiHandlerLimitsRequestBodiesTo(bytes int) {
	w.maxBodyBytes = int64(bytes)
	w.handler = nil
}

func (w *bddWorld) useBearerToken(token string) {
	w.bearerToken = token
}

func (w *bddWorld) clearBearerToken() {
	w.bearerToken = ""
}

func (w *bddWorld) setRequestHeader(name, value string) {
	w.requestHeaders[name] = value
}

func (w *bddWorld) callingFromRemoteAddress(remoteAddr string) {
	w.remoteAddr = remoteAddr
}

func (w *bddWorld) sendRequest(method, path string) {
	w.sendRequestWithJSON(method, path, "")
}

func (w *bddWorld) sendRequestWithJSON(method, path, body string) {
	w.ensureHandler()

	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if w.remoteAddr != "" {
		req.RemoteAddr = w.remoteAddr
	}
	for name, value := range w.requestHeaders {
		req.Header.Set(name, value)
	}
	if w.bearerToken != "" && strings.TrimSpace(req.Header.Get("Authorization")) == "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", w.bearerToken))
	}

	w.response = httptest.NewRecorder()
	w.handler.ServeHTTP(w.response, req)
}

func (w *bddWorld) responseStatusShouldBe(status int) error {
	if w.response == nil {
		return fmt.Errorf("no response recorded")
	}
	if w.response.Code != status {
		return fmt.Errorf("expected status %d, got %d", status, w.response.Code)
	}
	return nil
}

func (w *bddWorld) jsonFieldShouldEqual(field, expected string) error {
	if w.response == nil {
		return fmt.Errorf("no response recorded")
	}
	var payload map[string]any
	if err := json.Unmarshal(w.response.Body.Bytes(), &payload); err != nil {
		return fmt.Errorf("decode response body: %w", err)
	}
	actual, ok := payload[field]
	if !ok {
		return fmt.Errorf("response field %q missing", field)
	}
	if fmt.Sprint(actual) != expected {
		return fmt.Errorf("expected %q=%q, got %v", field, expected, actual)
	}
	return nil
}

func (w *bddWorld) jsonFieldShouldContain(field, fragment string) error {
	if w.response == nil {
		return fmt.Errorf("no response recorded")
	}
	var payload map[string]any
	if err := json.Unmarshal(w.response.Body.Bytes(), &payload); err != nil {
		return fmt.Errorf("decode response body: %w", err)
	}
	actual, ok := payload[field]
	if !ok {
		return fmt.Errorf("response field %q missing", field)
	}
	if !strings.Contains(fmt.Sprint(actual), fragment) {
		return fmt.Errorf("expected %q to contain %q, got %v", field, fragment, actual)
	}
	return nil
}

func (w *bddWorld) responseHeaderShouldEqual(name, expected string) error {
	if w.response == nil {
		return fmt.Errorf("no response recorded")
	}
	actual := w.response.Header().Get(name)
	if actual != expected {
		return fmt.Errorf("expected header %q=%q, got %q", name, expected, actual)
	}
	return nil
}

func (w *bddWorld) responseHeaderShouldHavePrefix(name, expectedPrefix string) error {
	if w.response == nil {
		return fmt.Errorf("no response recorded")
	}
	actual := w.response.Header().Get(name)
	if !strings.HasPrefix(actual, expectedPrefix) {
		return fmt.Errorf("expected header %q prefix %q, got %q", name, expectedPrefix, actual)
	}
	return nil
}

func (w *bddWorld) dispatchedCommandsShouldBe(expected int) error {
	if w.sender.calls != expected {
		return fmt.Errorf("expected %d dispatched command(s), got %d", expected, w.sender.calls)
	}
	return nil
}

func (w *bddWorld) lastDispatchedCommandTypeShouldBe(expected string) error {
	if w.sender.lastCmd == nil {
		return fmt.Errorf("no command dispatched")
	}
	if actual := w.sender.lastCmd.CommandType(); actual != expected {
		return fmt.Errorf("expected last command type %q, got %q", expected, actual)
	}
	return nil
}

func (w *bddWorld) lastDispatchedTargetDeviceShouldBe(expected string) error {
	if w.sender.lastCmd == nil {
		return fmt.Errorf("no command dispatched")
	}
	if actual := w.sender.lastCmd.TargetDeviceID(); actual != expected {
		return fmt.Errorf("expected last target device %q, got %q", expected, actual)
	}
	return nil
}

func InitializeScenario(ctx *godog.ScenarioContext) {
	world := &bddWorld{}

	ctx.Before(func(_ context.Context, _ *godog.Scenario) (context.Context, error) {
		world.reset()
		return context.Background(), nil
	})

	ctx.Step(`^the API handler is running$`, world.apiHandlerIsRunning)
	ctx.Step(`^the API handler requires TLS$`, world.apiHandlerRequiresTLS)
	ctx.Step(`^the API handler requires TLS and trusts proxy headers$`, world.apiHandlerRequiresTLSAndTrustsProxyHeaders)
	ctx.Step(`^the API handler allows unauthenticated readiness checks$`, world.apiHandlerAllowsUnauthenticatedReadinessChecks)
	ctx.Step(`^the API handler has a failing readiness checker$`, world.apiHandlerHasFailingReadinessChecker)
	ctx.Step(`^the API handler rate limits auth failures to (\d+) per minute$`, world.apiHandlerRateLimitsAuthFailures)
	ctx.Step(`^the API handler authorizes only device "([^"]*)"$`, world.apiHandlerAuthorizesOnlyDevice)
	ctx.Step(`^the sender fails downstream$`, world.senderFailsDownstream)
	ctx.Step(`^the API handler limits request bodies to (\d+) bytes$`, world.apiHandlerLimitsRequestBodiesTo)
	ctx.Step(`^I use bearer token "([^"]*)"$`, world.useBearerToken)
	ctx.Step(`^I clear the bearer token$`, world.clearBearerToken)
	ctx.Step(`^I set request header "([^"]*)" to "([^"]*)"$`, world.setRequestHeader)
	ctx.Step(`^I am calling from remote address "([^"]*)"$`, world.callingFromRemoteAddress)
	ctx.Step(`^I send a "([^"]*)" request to "([^"]*)"$`, world.sendRequest)
	ctx.Step(`^I send a "([^"]*)" request to "([^"]*)" with JSON:$`, world.sendRequestWithJSON)
	ctx.Step(`^the response status should be (\d+)$`, world.responseStatusShouldBe)
	ctx.Step(`^the JSON response field "([^"]*)" should equal "([^"]*)"$`, world.jsonFieldShouldEqual)
	ctx.Step(`^the JSON response field "([^"]*)" should contain "([^"]*)"$`, world.jsonFieldShouldContain)
	ctx.Step(`^the response header "([^"]*)" should equal "([^"]*)"$`, world.responseHeaderShouldEqual)
	ctx.Step(`^the response header "([^"]*)" should have prefix "([^"]*)"$`, world.responseHeaderShouldHavePrefix)
	ctx.Step(`^exactly (\d+) command should be dispatched$`, world.dispatchedCommandsShouldBe)
	ctx.Step(`^the last dispatched command type should be "([^"]*)"$`, world.lastDispatchedCommandTypeShouldBe)
	ctx.Step(`^the last dispatched target device should be "([^"]*)"$`, world.lastDispatchedTargetDeviceShouldBe)
}

func TestFeatures(t *testing.T) {
	t.Helper()

	suite := godog.TestSuite{
		Name:                "api-bdd",
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			TestingT: t,
		},
	}

	if suite.Run() != 0 {
		t.Fatalf("godog scenarios failed")
	}
}
