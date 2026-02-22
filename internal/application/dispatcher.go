package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/paul/clock-server/internal/domain"
)

// CommandDispatcher coordinates command validation and sending through output ports.
type CommandDispatcher struct {
	sender ClockCommandSender
}

// NewCommandDispatcher creates a new application service instance.
func NewCommandDispatcher(sender ClockCommandSender) *CommandDispatcher {
	return &CommandDispatcher{sender: sender}
}

// Dispatch validates and forwards a command through the configured sender.
func (d *CommandDispatcher) Dispatch(ctx context.Context, cmd domain.ClockCommand) error {
	if cmd == nil {
		return fmt.Errorf("%w: command is required", ErrValidation)
	}
	if err := cmd.Execute(ctx); err != nil {
		var validationErr domain.ValidationError
		if errors.As(err, &validationErr) {
			return fmt.Errorf("%w: execute command %s: %w", ErrValidation, cmd.CommandType(), err)
		}
		return fmt.Errorf("%w: execute command %s: %w", ErrDownstream, cmd.CommandType(), err)
	}
	if err := d.sender.Send(ctx, cmd); err != nil {
		return fmt.Errorf("%w: send command %s: %w", ErrDownstream, cmd.CommandType(), err)
	}
	return nil
}
