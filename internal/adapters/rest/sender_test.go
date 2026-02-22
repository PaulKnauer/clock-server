package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/paul/clock-server/internal/domain"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRESTSenderMapsAlarmCommand(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotBody map[string]any

	sender, err := NewSender(Config{BaseURL: "http://clock-api.local", AllowInsecureHTTP: true})
	if err != nil {
		t.Fatalf("new sender: %v", err)
	}
	sender.client = &http.Client{
		Timeout: 2 * time.Second,
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusAccepted,
				Body:       io.NopCloser(bytes.NewBufferString(`{"ok":true}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	cmd := domain.SetAlarmCommand{DeviceID: "clock-22", AlarmTime: time.Date(2030, 1, 1, 8, 0, 0, 0, time.UTC), Label: "run"}
	if err := sender.Send(context.Background(), cmd); err != nil {
		t.Fatalf("send command: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/clocks/clock-22/alarms" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotBody["label"] != "run" {
		t.Fatalf("expected label run, got %v", gotBody["label"])
	}
}
