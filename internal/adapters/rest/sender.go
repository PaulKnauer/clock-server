package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/paul/clock-server/internal/domain"
)

// Config defines REST adapter settings.
type Config struct {
	BaseURL           string
	AuthToken         string
	Timeout           time.Duration
	HealthPath        string
	AllowInsecureHTTP bool
}

// Sender sends clock commands to a downstream REST service.
type Sender struct {
	client     *http.Client
	baseURL    string
	token      string
	healthPath string
}

// NewSender creates a REST sender.
func NewSender(cfg Config) (*Sender, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		return nil, errors.New("rest base url is required")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("invalid rest base url: %w", err)
	}
	if parsed.Scheme != "https" && !cfg.AllowInsecureHTTP {
		return nil, errors.New("insecure downstream http is disabled; use https:// or set ALLOW_INSECURE_DOWNSTREAM_HTTP=true")
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return &Sender{
		client:     &http.Client{Timeout: timeout},
		baseURL:    base,
		token:      cfg.AuthToken,
		healthPath: strings.TrimSpace(cfg.HealthPath),
	}, nil
}

// Send maps a domain command into an HTTP request.
func (s *Sender) Send(ctx context.Context, cmd domain.ClockCommand) error {
	if cmd == nil {
		return errors.New("command is required")
	}

	method, path, payload, err := mapRequest(cmd)
	if err != nil {
		return err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal rest payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(s.token) != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send rest request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("rest request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	return nil
}

func mapRequest(cmd domain.ClockCommand) (method string, path string, payload map[string]any, err error) {
	deviceID := url.PathEscape(cmd.TargetDeviceID())
	switch c := cmd.(type) {
	case domain.SetAlarmCommand:
		return http.MethodPost, fmt.Sprintf("/clocks/%s/alarms", deviceID), map[string]any{
			"alarmTime": c.AlarmTime.Format(time.RFC3339),
			"label":     c.Label,
		}, nil
	case domain.DisplayMessageCommand:
		return http.MethodPost, fmt.Sprintf("/clocks/%s/messages", deviceID), map[string]any{
			"message":         c.Message,
			"durationSeconds": c.DurationSeconds,
		}, nil
	case domain.SetBrightnessCommand:
		return http.MethodPut, fmt.Sprintf("/clocks/%s/brightness", deviceID), map[string]any{
			"level": c.Level,
		}, nil
	default:
		return "", "", nil, fmt.Errorf("unsupported command type %T", cmd)
	}
}

// Check verifies downstream readiness.
func (s *Sender) Check(ctx context.Context) error {
	if s.healthPath == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+s.healthPath, nil)
	if err != nil {
		return fmt.Errorf("build readiness request: %w", err)
	}
	if strings.TrimSpace(s.token) != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("readiness request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("readiness status=%d", resp.StatusCode)
	}
	return nil
}
