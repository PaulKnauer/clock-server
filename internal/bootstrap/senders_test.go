package bootstrap

import (
	"strings"
	"testing"

	"github.com/paul/clock-server/internal/adapters/rest"
	"github.com/paul/clock-server/internal/config"
)

func TestBuildCompositeSenderRestOnly(t *testing.T) {
	cfg := config.Config{
		EnabledSenders: []string{"rest"},
		REST:           rest.Config{BaseURL: "http://clock-api.local", AllowInsecureHTTP: true},
	}

	sender, checkers, cleanup, err := BuildCompositeSender(cfg)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if sender == nil {
		t.Fatal("expected sender")
	}
	if cleanup == nil {
		t.Fatal("expected cleanup function")
	}
	if len(checkers) != 1 {
		t.Fatalf("expected one checker, got %d", len(checkers))
	}
	cleanup()
}

func TestBuildCompositeSenderRejectsUnsupportedSender(t *testing.T) {
	cfg := config.Config{EnabledSenders: []string{"not-real"}}

	_, _, cleanup, err := BuildCompositeSender(cfg)
	if cleanup == nil {
		t.Fatal("expected cleanup function")
	}
	if err == nil {
		t.Fatal("expected unsupported sender error")
	}
	if !strings.Contains(err.Error(), "unsupported sender") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildCompositeSenderRestConfigError(t *testing.T) {
	cfg := config.Config{EnabledSenders: []string{"rest"}}

	_, _, _, err := BuildCompositeSender(cfg)
	if err == nil {
		t.Fatal("expected rest config error")
	}
	if !strings.Contains(err.Error(), "build rest sender") {
		t.Fatalf("unexpected error: %v", err)
	}
}
