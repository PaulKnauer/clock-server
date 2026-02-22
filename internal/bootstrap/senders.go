package bootstrap

import (
	"fmt"

	"github.com/paul/clock-server/internal/adapters/composite"
	"github.com/paul/clock-server/internal/adapters/mqtt"
	"github.com/paul/clock-server/internal/adapters/rest"
	"github.com/paul/clock-server/internal/application"
	"github.com/paul/clock-server/internal/config"
)

// BuildCompositeSender wires sender adapters based on configuration.
func BuildCompositeSender(cfg config.Config) (application.ClockCommandSender, []application.ReadinessChecker, func(), error) {
	senders := make([]application.ClockCommandSender, 0, len(cfg.EnabledSenders))
	checkers := make([]application.ReadinessChecker, 0, len(cfg.EnabledSenders))
	cleanup := func() {}

	for _, enabled := range cfg.EnabledSenders {
		switch enabled {
		case "mqtt":
			sender, err := mqtt.NewSender(cfg.MQTT)
			if err != nil {
				return nil, nil, cleanup, fmt.Errorf("build mqtt sender: %w", err)
			}
			senders = append(senders, sender)
			checkers = append(checkers, sender)
			prevCleanup := cleanup
			cleanup = func() {
				sender.Close()
				prevCleanup()
			}
		case "rest":
			sender, err := rest.NewSender(cfg.REST)
			if err != nil {
				return nil, nil, cleanup, fmt.Errorf("build rest sender: %w", err)
			}
			senders = append(senders, sender)
			checkers = append(checkers, sender)
		default:
			return nil, nil, cleanup, fmt.Errorf("unsupported sender %q", enabled)
		}
	}

	return composite.NewSender(senders...), checkers, cleanup, nil
}
