package composite

import (
	"context"
	"errors"
	"fmt"

	"github.com/paul/clock-server/internal/application"
	"github.com/paul/clock-server/internal/domain"
)

// Sender dispatches commands through multiple senders sequentially.
type Sender struct {
	senders []application.ClockCommandSender
}

// NewSender constructs a composite sender.
func NewSender(senders ...application.ClockCommandSender) *Sender {
	return &Sender{senders: senders}
}

// Send forwards command to each configured sender in sequence.
func (s *Sender) Send(ctx context.Context, cmd domain.ClockCommand) error {
	if len(s.senders) == 0 {
		return errors.New("no command senders configured")
	}

	var errs []error
	for idx, sender := range s.senders {
		if sender == nil {
			errs = append(errs, fmt.Errorf("sender at index %d is nil", idx))
			continue
		}
		if err := sender.Send(ctx, cmd); err != nil {
			errs = append(errs, fmt.Errorf("sender %d failed: %w", idx, err))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
