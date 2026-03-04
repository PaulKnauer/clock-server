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

func TestValidateDeviceID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid alphanumeric", "clock-1", false},
		{"valid with underscore", "my_clock_99", false},
		{"valid max length", "abcdefghijklmnopqrstuvwxyz01234567890123456789ABCDEFGHIJKLMNOPQR", false},
		{"empty", "", true},
		{"whitespace only", "   ", true},
		{"path traversal", "../../admin", true},
		{"mqtt wildcard plus", "clock+1", true},
		{"mqtt wildcard hash", "clock#1", true},
		{"slash injection", "clock/../../admin", true},
		{"spaces", "clock 1", true},
		{"too long", "a123456789012345678901234567890123456789012345678901234567890123X", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDeviceID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDeviceID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestSetAlarmRejectsTopicInjection(t *testing.T) {
	cmd := SetAlarmCommand{
		DeviceID:  "../../admin",
		AlarmTime: time.Now().Add(10 * time.Minute),
	}
	if err := cmd.Validate(); err == nil {
		t.Fatal("expected validation error for path traversal device id")
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
