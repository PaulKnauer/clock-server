package domain

import (
	"context"
	"testing"
	"time"
)

func TestSetAlarmCommandValidate(t *testing.T) {
	cmd := SetAlarmCommand{
		DeviceID:  "clock-1",
		AlarmTime: time.Now().Add(10 * time.Minute),
		Label:     "wake-up",
	}
	if err := cmd.Validate(); err != nil {
		t.Fatalf("expected valid command, got error: %v", err)
	}
	if err := cmd.Execute(context.Background()); err != nil {
		t.Fatalf("expected execute success, got error: %v", err)
	}
}

func TestSetAlarmCommandRejectsPastTime(t *testing.T) {
	cmd := SetAlarmCommand{
		DeviceID:  "clock-1",
		AlarmTime: time.Now().Add(-2 * time.Minute),
	}
	if err := cmd.Validate(); err == nil {
		t.Fatal("expected validation error for past time")
	}
}

func TestDisplayMessageCommandValidate(t *testing.T) {
	cmd := DisplayMessageCommand{DeviceID: "clock-1", Message: "hello", DurationSeconds: 5}
	if err := cmd.Validate(); err != nil {
		t.Fatalf("expected valid command, got error: %v", err)
	}
}

func TestSetBrightnessCommandValidate(t *testing.T) {
	valid := SetBrightnessCommand{DeviceID: "clock-1", Level: 100}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid command, got error: %v", err)
	}

	invalid := SetBrightnessCommand{DeviceID: "clock-1", Level: 101}
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected validation error for out-of-range brightness")
	}
}
