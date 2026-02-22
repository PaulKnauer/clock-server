package mqtt

import (
	"testing"
	"time"

	"github.com/paul/clock-server/internal/domain"
)

func TestBuildTopic(t *testing.T) {
	cmd := domain.SetBrightnessCommand{DeviceID: "clock-9", Level: 50}
	topic := buildTopic("clocks/commands", cmd)
	if topic != "clocks/commands/clock-9/set_brightness" {
		t.Fatalf("unexpected topic: %s", topic)
	}
}

func TestBuildPayloadAlarm(t *testing.T) {
	cmd := domain.SetAlarmCommand{DeviceID: "clock-1", AlarmTime: time.Date(2030, 1, 1, 6, 0, 0, 0, time.UTC), Label: "wake"}
	payload, err := buildPayload(cmd)
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	if payload["type"] != "set_alarm" {
		t.Fatalf("unexpected type: %v", payload["type"])
	}
	if payload["label"] != "wake" {
		t.Fatalf("unexpected label: %v", payload["label"])
	}
}
