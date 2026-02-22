package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/paul/clock-server/internal/adapters/mqtt"
	"github.com/paul/clock-server/internal/adapters/rest"
	"github.com/paul/clock-server/internal/security"
)

// Config centralizes runtime settings for transport adapters and API.
type Config struct {
	ServerAddr           string
	ServerReadTimeout    time.Duration
	ServerWriteTimeout   time.Duration
	ServerIdleTimeout    time.Duration
	ServerHeaderTimeout  time.Duration
	ServerShutdownPeriod time.Duration
	TLSCertFile          string
	TLSKeyFile           string
	RequireTLS           bool
	TrustProxyTLS        bool
	ReadinessRequireAuth bool
	MaxBodyBytes         int64
	AuthFailLimitPerMin  int
	AuthCredentials      []security.Credential
	EnabledSenders       []string
	MQTT                 mqtt.Config
	REST                 rest.Config
}

// LoadFromEnv reads configuration from environment variables.
func LoadFromEnv() (Config, error) {
	cfg := Config{
		ServerAddr:           getEnv("HTTP_ADDR", ":8080"),
		ServerReadTimeout:    mustPositiveDuration("HTTP_READ_TIMEOUT_MS", 10000),
		ServerWriteTimeout:   mustPositiveDuration("HTTP_WRITE_TIMEOUT_MS", 10000),
		ServerIdleTimeout:    mustPositiveDuration("HTTP_IDLE_TIMEOUT_MS", 60000),
		ServerHeaderTimeout:  mustPositiveDuration("HTTP_READ_HEADER_TIMEOUT_MS", 5000),
		ServerShutdownPeriod: mustPositiveDuration("HTTP_SHUTDOWN_TIMEOUT_MS", 15000),
		TLSCertFile:          strings.TrimSpace(os.Getenv("TLS_CERT_FILE")),
		TLSKeyFile:           strings.TrimSpace(os.Getenv("TLS_KEY_FILE")),
		RequireTLS:           parseBool("REQUIRE_TLS", true),
		TrustProxyTLS:        parseBool("TRUST_PROXY_TLS", false),
		ReadinessRequireAuth: parseBool("READINESS_REQUIRE_AUTH", true),
		MaxBodyBytes:         int64(mustIntInRange("MAX_BODY_BYTES", 65536, 1024, 10*1024*1024)),
		AuthFailLimitPerMin:  mustIntInRange("AUTH_FAIL_LIMIT_PER_MIN", 60, 1, 10000),
		EnabledSenders: splitCSV(
			getEnv("ENABLED_SENDERS", "mqtt,rest"),
		),
		MQTT: mqtt.Config{
			BrokerURL:              os.Getenv("MQTT_BROKER_URL"),
			ClientID:               os.Getenv("MQTT_CLIENT_ID"),
			Username:               os.Getenv("MQTT_USERNAME"),
			Password:               os.Getenv("MQTT_PASSWORD"),
			TopicPrefix:            getEnv("MQTT_TOPIC_PREFIX", "clocks/commands"),
			ConnectRetry:           parseBool("MQTT_CONNECT_RETRY", true),
			QoS:                    byte(mustIntInRange("MQTT_QOS", 1, 0, 2)),
			Retained:               parseBool("MQTT_RETAINED", false),
			TLSInsecureSkipVerify:  parseBool("MQTT_TLS_INSECURE_SKIP_VERIFY", false),
			AllowInsecureTLS:       parseBool("ALLOW_INSECURE_TLS_VERIFY", false),
			AllowInsecureTransport: parseBool("ALLOW_INSECURE_MQTT", false),
		},
		REST: rest.Config{
			BaseURL:           os.Getenv("CLOCK_REST_BASE_URL"),
			AuthToken:         os.Getenv("CLOCK_REST_TOKEN"),
			Timeout:           mustPositiveDuration("CLOCK_REST_TIMEOUT_MS", 5000),
			HealthPath:        strings.TrimSpace(os.Getenv("CLOCK_REST_HEALTH_PATH")),
			AllowInsecureHTTP: parseBool("ALLOW_INSECURE_DOWNSTREAM_HTTP", false),
		},
	}
	creds, err := security.ParseCredentials(os.Getenv("API_AUTH_CREDENTIALS"))
	if err != nil {
		return Config{}, fmt.Errorf("parse API_AUTH_CREDENTIALS: %w", err)
	}
	legacyToken := strings.TrimSpace(os.Getenv("API_AUTH_TOKEN"))
	if len(creds) == 0 && legacyToken != "" {
		creds = []security.Credential{{ID: "legacy", Token: legacyToken, Devices: []string{"*"}}}
	}
	if len(creds) == 0 {
		return Config{}, fmt.Errorf("API_AUTH_CREDENTIALS or API_AUTH_TOKEN is required")
	}
	cfg.AuthCredentials = creds

	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return Config{}, fmt.Errorf("TLS_CERT_FILE and TLS_KEY_FILE must both be set")
	}
	if cfg.RequireTLS && cfg.TLSCertFile == "" && !cfg.TrustProxyTLS {
		return Config{}, fmt.Errorf("TLS is required: set TLS_CERT_FILE/TLS_KEY_FILE or TRUST_PROXY_TLS=true")
	}
	if cfg.REST.BaseURL != "" {
		if _, err := url.Parse(cfg.REST.BaseURL); err != nil {
			return Config{}, fmt.Errorf("invalid CLOCK_REST_BASE_URL: %w", err)
		}
	}

	for _, sender := range cfg.EnabledSenders {
		switch sender {
		case "mqtt", "rest":
		default:
			return Config{}, fmt.Errorf("unknown sender in ENABLED_SENDERS: %s", sender)
		}
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(strings.ToLower(p))
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func parseInt(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback, err
	}
	return parsed, nil
}

func parseBool(key string, fallback bool) bool {
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

func mustPositiveDuration(key string, fallbackMS int) time.Duration {
	ms, err := parseInt(key, fallbackMS)
	if err != nil || ms <= 0 {
		return time.Duration(fallbackMS) * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}

func mustIntInRange(key string, fallback, min, max int) int {
	value, err := parseInt(key, fallback)
	if err != nil || value < min || value > max {
		return fallback
	}
	return value
}
