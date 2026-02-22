package config

import (
	"strings"
	"testing"
	"time"
)

func clearConfigEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"HTTP_ADDR",
		"ENABLED_SENDERS",
		"MQTT_BROKER_URL",
		"MQTT_CLIENT_ID",
		"MQTT_USERNAME",
		"MQTT_PASSWORD",
		"MQTT_TOPIC_PREFIX",
		"MQTT_CONNECT_RETRY",
		"MQTT_QOS",
		"MQTT_RETAINED",
		"MQTT_TLS_INSECURE_SKIP_VERIFY",
		"CLOCK_REST_BASE_URL",
		"CLOCK_REST_TOKEN",
		"CLOCK_REST_TIMEOUT_MS",
		"CLOCK_REST_HEALTH_PATH",
		"API_AUTH_TOKEN",
		"HTTP_READ_TIMEOUT_MS",
		"HTTP_WRITE_TIMEOUT_MS",
		"HTTP_IDLE_TIMEOUT_MS",
		"HTTP_READ_HEADER_TIMEOUT_MS",
		"HTTP_SHUTDOWN_TIMEOUT_MS",
		"TLS_CERT_FILE",
		"TLS_KEY_FILE",
		"REQUIRE_TLS",
		"TRUST_PROXY_TLS",
		"READINESS_REQUIRE_AUTH",
		"MAX_BODY_BYTES",
		"AUTH_FAIL_LIMIT_PER_MIN",
		"API_AUTH_CREDENTIALS",
		"ALLOW_INSECURE_TLS_VERIFY",
		"ALLOW_INSECURE_MQTT",
		"ALLOW_INSECURE_DOWNSTREAM_HTTP",
	}
	for _, key := range keys {
		t.Setenv(key, "")
	}
}

func TestLoadFromEnvDefaults(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("API_AUTH_TOKEN", "test-token")
	t.Setenv("REQUIRE_TLS", "false")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.ServerAddr != ":8080" {
		t.Fatalf("expected default server addr, got %q", cfg.ServerAddr)
	}
	if len(cfg.EnabledSenders) != 2 || cfg.EnabledSenders[0] != "mqtt" || cfg.EnabledSenders[1] != "rest" {
		t.Fatalf("unexpected default senders: %#v", cfg.EnabledSenders)
	}
	if cfg.MQTT.TopicPrefix != "clocks/commands" {
		t.Fatalf("expected default topic prefix, got %q", cfg.MQTT.TopicPrefix)
	}
	if cfg.MQTT.QoS != 1 {
		t.Fatalf("expected default qos 1, got %d", cfg.MQTT.QoS)
	}
	if !cfg.MQTT.ConnectRetry {
		t.Fatal("expected default connect retry true")
	}
	if cfg.REST.Timeout != 5*time.Second {
		t.Fatalf("expected default timeout 5s, got %s", cfg.REST.Timeout)
	}
}

func TestLoadFromEnvParsesOverrides(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("API_AUTH_TOKEN", "test-token")
	t.Setenv("REQUIRE_TLS", "false")
	t.Setenv("HTTP_ADDR", "  :9090 ")
	t.Setenv("ENABLED_SENDERS", " REST ")
	t.Setenv("MQTT_TOPIC_PREFIX", " custom/topic ")
	t.Setenv("MQTT_QOS", "2")
	t.Setenv("MQTT_RETAINED", "true")
	t.Setenv("MQTT_CONNECT_RETRY", "false")
	t.Setenv("CLOCK_REST_TIMEOUT_MS", "1200")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.ServerAddr != ":9090" {
		t.Fatalf("expected :9090, got %q", cfg.ServerAddr)
	}
	if len(cfg.EnabledSenders) != 1 || cfg.EnabledSenders[0] != "rest" {
		t.Fatalf("unexpected enabled senders: %#v", cfg.EnabledSenders)
	}
	if cfg.MQTT.TopicPrefix != "custom/topic" {
		t.Fatalf("expected custom topic prefix, got %q", cfg.MQTT.TopicPrefix)
	}
	if cfg.MQTT.QoS != 2 {
		t.Fatalf("expected qos 2, got %d", cfg.MQTT.QoS)
	}
	if cfg.MQTT.ConnectRetry {
		t.Fatal("expected connect retry false")
	}
	if !cfg.MQTT.Retained {
		t.Fatal("expected retained true")
	}
	if cfg.REST.Timeout != 1200*time.Millisecond {
		t.Fatalf("expected timeout 1200ms, got %s", cfg.REST.Timeout)
	}
}

func TestLoadFromEnvRejectsUnknownSender(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("API_AUTH_TOKEN", "test-token")
	t.Setenv("REQUIRE_TLS", "false")
	t.Setenv("ENABLED_SENDERS", "mqtt,unknown")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for unknown sender")
	}
	if !strings.Contains(err.Error(), "unknown sender") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadFromEnvFallsBackOnInvalidBoolAndInt(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("API_AUTH_TOKEN", "test-token")
	t.Setenv("REQUIRE_TLS", "false")
	t.Setenv("MQTT_QOS", "NaN")
	t.Setenv("MQTT_CONNECT_RETRY", "not-bool")
	t.Setenv("CLOCK_REST_TIMEOUT_MS", "invalid")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.MQTT.QoS != 1 {
		t.Fatalf("expected fallback qos 1, got %d", cfg.MQTT.QoS)
	}
	if !cfg.MQTT.ConnectRetry {
		t.Fatal("expected fallback connect retry true")
	}
	if cfg.REST.Timeout != 5*time.Second {
		t.Fatalf("expected fallback timeout 5s, got %s", cfg.REST.Timeout)
	}
}

func TestLoadFromEnvRequiresAPIToken(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("REQUIRE_TLS", "false")
	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error when API_AUTH_TOKEN is not set")
	}
	if !strings.Contains(err.Error(), "API_AUTH_TOKEN") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadFromEnvParsesCredentialScopes(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("REQUIRE_TLS", "false")
	t.Setenv("API_AUTH_CREDENTIALS", "ops|secret|clock-*")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.AuthCredentials) != 1 {
		t.Fatalf("expected one credential, got %d", len(cfg.AuthCredentials))
	}
	if cfg.AuthCredentials[0].ID != "ops" {
		t.Fatalf("unexpected id: %s", cfg.AuthCredentials[0].ID)
	}
	if !cfg.AuthCredentials[0].Allows("clock-12") {
		t.Fatal("expected scope to allow clock-12")
	}
}
