package application

import (
	"context"

	"github.com/paul/clock-server/internal/domain"
)

// ClockCommandSender is the output port used by application services to deliver commands.
type ClockCommandSender interface {
	Send(ctx context.Context, cmd domain.ClockCommand) error
}
