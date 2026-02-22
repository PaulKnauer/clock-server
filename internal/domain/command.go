package domain

import (
	"context"
	"strings"
	"time"
)

// ClockCommand defines the behavior every command sent to a smart clock must implement.
type ClockCommand interface {
	Execute(ctx context.Context) error
	TargetDeviceID() string
	CommandType() string
	Validate() error
}

// SetAlarmCommand instructs a clock to create a new alarm.
type SetAlarmCommand struct {
	DeviceID  string
	AlarmTime time.Time
	Label     string
}

// Execute validates the command and performs domain-level execution.
func (c SetAlarmCommand) Execute(_ context.Context) error {
	return c.Validate()
}

// TargetDeviceID returns the destination device identifier.
func (c SetAlarmCommand) TargetDeviceID() string {
	return strings.TrimSpace(c.DeviceID)
}

// CommandType returns the stable command name.
func (c SetAlarmCommand) CommandType() string {
	return "set_alarm"
}

// Validate verifies command invariants.
func (c SetAlarmCommand) Validate() error {
	if strings.TrimSpace(c.DeviceID) == "" {
		return NewValidationError("device id is required")
	}
	if c.AlarmTime.IsZero() {
		return NewValidationError("alarm time is required")
	}
	if c.AlarmTime.Before(time.Now().Add(-1 * time.Minute)) {
		return NewValidationErrorf("alarm time %s is in the past", c.AlarmTime.Format(time.RFC3339))
	}
	return nil
}

// DisplayMessageCommand instructs a clock to show a message.
type DisplayMessageCommand struct {
	DeviceID        string
	Message         string
	DurationSeconds int
}

// Execute validates the command and performs domain-level execution.
func (c DisplayMessageCommand) Execute(_ context.Context) error {
	return c.Validate()
}

// TargetDeviceID returns the destination device identifier.
func (c DisplayMessageCommand) TargetDeviceID() string {
	return strings.TrimSpace(c.DeviceID)
}

// CommandType returns the stable command name.
func (c DisplayMessageCommand) CommandType() string {
	return "display_message"
}

// Validate verifies command invariants.
func (c DisplayMessageCommand) Validate() error {
	if strings.TrimSpace(c.DeviceID) == "" {
		return NewValidationError("device id is required")
	}
	if strings.TrimSpace(c.Message) == "" {
		return NewValidationError("message is required")
	}
	if c.DurationSeconds <= 0 {
		return NewValidationError("duration seconds must be greater than zero")
	}
	if c.DurationSeconds > 3600 {
		return NewValidationError("duration seconds must be less than or equal to 3600")
	}
	return nil
}

// SetBrightnessCommand instructs a clock to change screen brightness.
type SetBrightnessCommand struct {
	DeviceID string
	Level    int
}

// Execute validates the command and performs domain-level execution.
func (c SetBrightnessCommand) Execute(_ context.Context) error {
	return c.Validate()
}

// TargetDeviceID returns the destination device identifier.
func (c SetBrightnessCommand) TargetDeviceID() string {
	return strings.TrimSpace(c.DeviceID)
}

// CommandType returns the stable command name.
func (c SetBrightnessCommand) CommandType() string {
	return "set_brightness"
}

// Validate verifies command invariants.
func (c SetBrightnessCommand) Validate() error {
	if strings.TrimSpace(c.DeviceID) == "" {
		return NewValidationError("device id is required")
	}
	if c.Level < 0 || c.Level > 100 {
		return NewValidationError("brightness level must be between 0 and 100")
	}
	return nil
}
