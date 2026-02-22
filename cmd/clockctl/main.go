package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultServerBaseURL = "http://localhost:8080"
	defaultTimeout       = 5 * time.Second
)

type apiClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func main() {
	if len(os.Args) < 2 {
		usageAndExit("missing command")
	}

	client := newAPIClientFromEnv()

	switch os.Args[1] {
	case "alarm":
		runAlarm(client, os.Args[2:])
	case "message":
		runMessage(client, os.Args[2:])
	case "brightness":
		runBrightness(client, os.Args[2:])
	default:
		usageAndExit("unknown command")
	}
}

func newAPIClientFromEnv() *apiClient {
	baseURL := resolveServerBaseURL()
	timeout := parseTimeout("CLOCKCTL_TIMEOUT_MS", defaultTimeout)
	return &apiClient{
		baseURL: baseURL,
		token:   strings.TrimSpace(os.Getenv("CLOCK_SERVER_TOKEN")),
		client:  &http.Client{Timeout: timeout},
	}
}

func runAlarm(client *apiClient, args []string) {
	fs := flag.NewFlagSet("alarm", flag.ExitOnError)
	deviceID := fs.String("device", "", "clock device id")
	alarmTime := fs.String("time", "", "alarm time in RFC3339")
	label := fs.String("label", "", "alarm label")
	_ = fs.Parse(args)

	if strings.TrimSpace(*deviceID) == "" {
		log.Fatal("device is required")
	}
	if _, err := time.Parse(time.RFC3339, *alarmTime); err != nil {
		log.Fatalf("time must be RFC3339: %v", err)
	}

	payload := map[string]any{
		"deviceId":  *deviceID,
		"alarmTime": *alarmTime,
		"label":     *label,
	}
	if err := client.send(http.MethodPost, "/commands/alarms", payload); err != nil {
		log.Fatalf("dispatch alarm command via server: %v", err)
	}
	fmt.Println("alarm command dispatched")
}

func runMessage(client *apiClient, args []string) {
	fs := flag.NewFlagSet("message", flag.ExitOnError)
	deviceID := fs.String("device", "", "clock device id")
	message := fs.String("message", "", "message text")
	duration := fs.Int("duration", 10, "duration in seconds")
	_ = fs.Parse(args)

	if strings.TrimSpace(*deviceID) == "" {
		log.Fatal("device is required")
	}
	if strings.TrimSpace(*message) == "" {
		log.Fatal("message is required")
	}

	payload := map[string]any{
		"deviceId":        *deviceID,
		"message":         *message,
		"durationSeconds": *duration,
	}
	if err := client.send(http.MethodPost, "/commands/messages", payload); err != nil {
		log.Fatalf("dispatch message command via server: %v", err)
	}
	fmt.Println("display-message command dispatched")
}

func runBrightness(client *apiClient, args []string) {
	fs := flag.NewFlagSet("brightness", flag.ExitOnError)
	deviceID := fs.String("device", "", "clock device id")
	level := fs.Int("level", 50, "brightness 0..100")
	_ = fs.Parse(args)

	if strings.TrimSpace(*deviceID) == "" {
		log.Fatal("device is required")
	}

	payload := map[string]any{
		"deviceId": *deviceID,
		"level":    *level,
	}
	if err := client.send(http.MethodPut, "/commands/brightness", payload); err != nil {
		log.Fatalf("dispatch brightness command via server: %v", err)
	}
	fmt.Println("brightness command dispatched")
}

func (c *apiClient) send(method, path string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		if err := ensureSafeTokenTransport(c.baseURL); err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("call server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("server returned status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	return nil
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func parseTimeout(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	ms, err := strconv.Atoi(value)
	if err != nil || ms <= 0 {
		return fallback
	}
	return time.Duration(ms) * time.Millisecond
}

func resolveServerBaseURL() string {
	if explicit := strings.TrimSpace(os.Getenv("CLOCK_SERVER_BASE_URL")); explicit != "" {
		return strings.TrimRight(explicit, "/")
	}

	rawHost := strings.TrimSpace(os.Getenv("CLOCK_SERVER_HOST"))
	if rawHost == "" {
		return defaultServerBaseURL
	}

	switch {
	case strings.HasPrefix(rawHost, "http://"), strings.HasPrefix(rawHost, "https://"):
		return strings.TrimRight(rawHost, "/")
	case strings.HasPrefix(rawHost, "tcp://"):
		host := strings.TrimPrefix(rawHost, "tcp://")
		if isLocalHost(hostnameOnly(host)) {
			return strings.TrimRight("http://"+host, "/")
		}
		return strings.TrimRight("https://"+host, "/")
	default:
		if isLocalHost(hostnameOnly(rawHost)) {
			return strings.TrimRight("http://"+rawHost, "/")
		}
		return strings.TrimRight("https://"+rawHost, "/")
	}
}

func ensureSafeTokenTransport(baseURL string) error {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("invalid CLOCK_SERVER_BASE_URL: %w", err)
	}
	if strings.EqualFold(parsed.Scheme, "https") {
		return nil
	}
	if parseBoolEnv("CLOCKCTL_ALLOW_INSECURE_HTTP", false) {
		return nil
	}
	if strings.EqualFold(parsed.Scheme, "http") && isLocalHost(parsed.Hostname()) {
		return nil
	}
	return fmt.Errorf("refusing to send bearer token over insecure transport to %s; set CLOCKCTL_ALLOW_INSECURE_HTTP=true to override", parsed.Host)
}

func parseBoolEnv(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func hostnameOnly(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport
	}
	return host
}

func isLocalHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" || host == "::1" || strings.HasPrefix(host, "127.") {
		return true
	}
	return false
}

func usageAndExit(msg string) {
	fmt.Fprintf(os.Stderr, "%s\n\n", msg)
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  clockctl alarm --device <id> --time <RFC3339> [--label <text>]")
	fmt.Fprintln(os.Stderr, "  clockctl message --device <id> --message <text> [--duration <seconds>]")
	fmt.Fprintln(os.Stderr, "  clockctl brightness --device <id> --level <0-100>")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "environment:")
	fmt.Fprintln(os.Stderr, "  CLOCK_SERVER_BASE_URL (default http://localhost:8080)")
	fmt.Fprintln(os.Stderr, "  CLOCK_SERVER_HOST (docker-style host, e.g. tcp://clock-server:8080)")
	fmt.Fprintln(os.Stderr, "  CLOCK_SERVER_TOKEN (optional bearer token)")
	fmt.Fprintln(os.Stderr, "  CLOCKCTL_TIMEOUT_MS (default 5000)")
	os.Exit(2)
}
