package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

type bddRoundTripper struct {
	handler func(*http.Request) (*http.Response, error)
}

func (r bddRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return r.handler(req)
}

type cliWorld struct {
	envKeys []string

	baseURL   string
	token     string
	timeout   time.Duration
	allowHTTP bool

	responseStatus int
	responseBody   string
	transportErr   error

	lastMethod string
	lastPath   string
	lastBody   map[string]any
	lastAuth   string

	sendErr error
}

func (w *cliWorld) reset() {
	w.envKeys = []string{
		"CLOCK_SERVER_BASE_URL",
		"CLOCK_SERVER_HOST",
		"CLOCK_SERVER_TOKEN",
		"CLOCKCTL_TIMEOUT_MS",
		"CLOCKCTL_ALLOW_INSECURE_HTTP",
	}
	w.clearEnv()

	w.baseURL = "http://localhost:8080"
	w.token = ""
	w.timeout = 2 * time.Second
	w.allowHTTP = false

	w.responseStatus = http.StatusAccepted
	w.responseBody = `{"result":"ok"}`
	w.transportErr = nil

	w.lastMethod = ""
	w.lastPath = ""
	w.lastBody = nil
	w.lastAuth = ""
	w.sendErr = nil
}

func (w *cliWorld) clearEnv() {
	for _, key := range w.envKeys {
		_ = os.Unsetenv(key)
	}
}

func (w *cliWorld) serverBaseURLIs(baseURL string) {
	w.baseURL = baseURL
}

func (w *cliWorld) bearerTokenIs(token string) {
	w.token = token
}

func (w *cliWorld) serverRespondsWithStatusAndBody(status int, body string) {
	w.responseStatus = status
	w.responseBody = body
}

func (w *cliWorld) transportFailsWith(message string) {
	w.transportErr = fmt.Errorf(message)
}

func (w *cliWorld) allowInsecureHTTPTransport() {
	w.allowHTTP = true
}

func (w *cliWorld) callMessageCommand(deviceID, message string, duration int) {
	client := w.newClient()
	payload := map[string]any{
		"deviceId":        deviceID,
		"message":         message,
		"durationSeconds": duration,
	}
	w.sendErr = client.send(http.MethodPost, "/commands/messages", payload)
}

func (w *cliWorld) callAlarmCommand(deviceID, alarmTime string, label string) {
	client := w.newClient()
	payload := map[string]any{
		"deviceId":  deviceID,
		"alarmTime": alarmTime,
		"label":     label,
	}
	w.sendErr = client.send(http.MethodPost, "/commands/alarms", payload)
}

func (w *cliWorld) callBrightnessCommand(deviceID string, level int) {
	client := w.newClient()
	payload := map[string]any{
		"deviceId": deviceID,
		"level":    level,
	}
	w.sendErr = client.send(http.MethodPut, "/commands/brightness", payload)
}

func (w *cliWorld) callSendWithPayload(method, path string, payload string) error {
	client := w.newClient()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	w.sendErr = client.send(method, path, decoded)
	return nil
}

func (w *cliWorld) setEnv(key, value string) error {
	if err := os.Setenv(key, value); err != nil {
		return fmt.Errorf("set env %s: %w", key, err)
	}
	return nil
}

func (w *cliWorld) resolveBaseURLFromEnvironment() {
	w.baseURL = resolveServerBaseURL()
}

func (w *cliWorld) createClientFromEnvironment() {
	client := newAPIClientFromEnv()
	w.baseURL = client.baseURL
	w.token = client.token
	w.timeout = client.client.Timeout
}

func (w *cliWorld) sendShouldSucceed() error {
	if w.sendErr != nil {
		return fmt.Errorf("expected success, got error: %w", w.sendErr)
	}
	return nil
}

func (w *cliWorld) sendShouldFailContaining(fragment string) error {
	if w.sendErr == nil {
		return fmt.Errorf("expected error containing %q, got nil", fragment)
	}
	if !strings.Contains(w.sendErr.Error(), fragment) {
		return fmt.Errorf("expected error containing %q, got %q", fragment, w.sendErr.Error())
	}
	return nil
}

func (w *cliWorld) requestMethodShouldBe(expected string) error {
	if w.lastMethod != expected {
		return fmt.Errorf("expected method %q, got %q", expected, w.lastMethod)
	}
	return nil
}

func (w *cliWorld) requestPathShouldBe(expected string) error {
	if w.lastPath != expected {
		return fmt.Errorf("expected path %q, got %q", expected, w.lastPath)
	}
	return nil
}

func (w *cliWorld) requestFieldShouldEqual(field, expected string) error {
	if w.lastBody == nil {
		return fmt.Errorf("no request body captured")
	}
	actual, ok := w.lastBody[field]
	if !ok {
		return fmt.Errorf("request field %q missing", field)
	}
	if fmt.Sprint(actual) != expected {
		return fmt.Errorf("expected field %q=%q, got %v", field, expected, actual)
	}
	return nil
}

func (w *cliWorld) authorizationHeaderShouldBe(expected string) error {
	if w.lastAuth != expected {
		return fmt.Errorf("expected authorization %q, got %q", expected, w.lastAuth)
	}
	return nil
}

func (w *cliWorld) resolvedBaseURLShouldBe(expected string) error {
	if w.baseURL != expected {
		return fmt.Errorf("expected base URL %q, got %q", expected, w.baseURL)
	}
	return nil
}

func (w *cliWorld) clientTimeoutShouldBeMilliseconds(ms int) error {
	expected := time.Duration(ms) * time.Millisecond
	if w.timeout != expected {
		return fmt.Errorf("expected timeout %s, got %s", expected, w.timeout)
	}
	return nil
}

func (w *cliWorld) clientTokenShouldBe(expected string) error {
	if w.token != expected {
		return fmt.Errorf("expected token %q, got %q", expected, w.token)
	}
	return nil
}

func (w *cliWorld) newClient() *apiClient {
	baseURL := w.baseURL
	token := w.token
	if w.allowHTTP {
		_ = os.Setenv("CLOCKCTL_ALLOW_INSECURE_HTTP", "true")
	}
	return &apiClient{
		baseURL: baseURL,
		token:   token,
		client: &http.Client{
			Timeout: w.timeout,
			Transport: bddRoundTripper{handler: func(req *http.Request) (*http.Response, error) {
				if w.transportErr != nil {
					return nil, w.transportErr
				}
				w.lastMethod = req.Method
				w.lastPath = req.URL.Path
				w.lastAuth = req.Header.Get("Authorization")

				raw, err := io.ReadAll(req.Body)
				if err != nil {
					return nil, err
				}
				if len(raw) > 0 {
					var body map[string]any
					if err := json.Unmarshal(raw, &body); err != nil {
						return nil, err
					}
					w.lastBody = body
				}

				return &http.Response{
					StatusCode: w.responseStatus,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewBufferString(w.responseBody)),
				}, nil
			}},
		},
	}
}

func (w *cliWorld) parseTimeoutForKey(key string, fallbackMS int) {
	w.timeout = parseTimeout(key, time.Duration(fallbackMS)*time.Millisecond)
}

func (w *cliWorld) timeoutShouldBeMilliseconds(ms int) error {
	expected := time.Duration(ms) * time.Millisecond
	if w.timeout != expected {
		return fmt.Errorf("expected timeout %s, got %s", expected, w.timeout)
	}
	return nil
}

func (w *cliWorld) setTimeoutEnvValue(value string) error {
	if err := os.Setenv("CLOCKCTL_TIMEOUT_MS", value); err != nil {
		return err
	}
	return nil
}

func (w *cliWorld) parseTimeoutUsingEnvFallback(fallbackMS int) {
	w.timeout = parseTimeout("CLOCKCTL_TIMEOUT_MS", time.Duration(fallbackMS)*time.Millisecond)
}

func (w *cliWorld) timeoutShouldMatchFallback(ms int) error {
	return w.timeoutShouldBeMilliseconds(ms)
}

func (w *cliWorld) parseBoolEnvKey(key string, fallback string) error {
	parsedFallback, err := strconv.ParseBool(fallback)
	if err != nil {
		return fmt.Errorf("invalid fallback bool: %w", err)
	}
	w.allowHTTP = parseBoolEnv(key, parsedFallback)
	return nil
}

func (w *cliWorld) boolResultShouldBe(expected string) error {
	want, err := strconv.ParseBool(expected)
	if err != nil {
		return fmt.Errorf("invalid bool expected value: %w", err)
	}
	if w.allowHTTP != want {
		return fmt.Errorf("expected bool %t, got %t", want, w.allowHTTP)
	}
	return nil
}

func InitializeScenario(ctx *godog.ScenarioContext) {
	world := &cliWorld{}

	ctx.Before(func(_ context.Context, _ *godog.Scenario) (context.Context, error) {
		world.reset()
		return context.Background(), nil
	})
	ctx.After(func(_ context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		world.clearEnv()
		return context.Background(), nil
	})

	ctx.Step(`^the server base URL is "([^"]*)"$`, world.serverBaseURLIs)
	ctx.Step(`^the bearer token is "([^"]*)"$`, world.bearerTokenIs)
	ctx.Step(`^the server responds with status (\d+) and body:$`, world.serverRespondsWithStatusAndBody)
	ctx.Step(`^the transport fails with "([^"]*)"$`, world.transportFailsWith)
	ctx.Step(`^insecure HTTP transport is allowed$`, world.allowInsecureHTTPTransport)
	ctx.Step(`^I send a message command for device "([^"]*)" message "([^"]*)" and duration (\d+)$`, world.callMessageCommand)
	ctx.Step(`^I send an alarm command for device "([^"]*)" time "([^"]*)" label "([^"]*)"$`, world.callAlarmCommand)
	ctx.Step(`^I send a brightness command for device "([^"]*)" level (\d+)$`, world.callBrightnessCommand)
	ctx.Step(`^I call send with method "([^"]*)" path "([^"]*)" payload:$`, world.callSendWithPayload)
	ctx.Step(`^I set env "([^"]*)" to "([^"]*)"$`, world.setEnv)
	ctx.Step(`^I resolve the server base URL$`, world.resolveBaseURLFromEnvironment)
	ctx.Step(`^I create an API client from environment$`, world.createClientFromEnvironment)
	ctx.Step(`^the send call should succeed$`, world.sendShouldSucceed)
	ctx.Step(`^the send call should fail containing "([^"]*)"$`, world.sendShouldFailContaining)
	ctx.Step(`^the request method should be "([^"]*)"$`, world.requestMethodShouldBe)
	ctx.Step(`^the request path should be "([^"]*)"$`, world.requestPathShouldBe)
	ctx.Step(`^the request JSON field "([^"]*)" should equal "([^"]*)"$`, world.requestFieldShouldEqual)
	ctx.Step(`^the Authorization header should be "([^"]*)"$`, world.authorizationHeaderShouldBe)
	ctx.Step(`^the resolved base URL should be "([^"]*)"$`, world.resolvedBaseURLShouldBe)
	ctx.Step(`^the client timeout should be (\d+) milliseconds$`, world.clientTimeoutShouldBeMilliseconds)
	ctx.Step(`^the client token should be "([^"]*)"$`, world.clientTokenShouldBe)
	ctx.Step(`^I set CLOCKCTL_TIMEOUT_MS to "([^"]*)"$`, world.setTimeoutEnvValue)
	ctx.Step(`^I parse timeout using fallback (\d+) milliseconds$`, world.parseTimeoutUsingEnvFallback)
	ctx.Step(`^the parsed timeout should be (\d+) milliseconds$`, world.timeoutShouldMatchFallback)
	ctx.Step(`^I parse bool env "([^"]*)" with fallback "([^"]*)"$`, world.parseBoolEnvKey)
	ctx.Step(`^the parsed bool should be "([^"]*)"$`, world.boolResultShouldBe)
}

func TestFeatures(t *testing.T) {
	t.Helper()

	suite := godog.TestSuite{
		Name:                "clockctl-bdd",
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
