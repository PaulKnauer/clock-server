package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/paul/clock-server/internal/domain"
)

type testSender struct {
	err   error
	calls int
}

func (s *testSender) Send(_ context.Context, _ domain.ClockCommand) error {
	s.calls++
	return s.err
}

type testCommand struct {
	typeName   string
	executeErr error
}

func (c testCommand) Execute(_ context.Context) error {
	return c.executeErr
}

func (c testCommand) TargetDeviceID() string {
	return "clock-1"
}

func (c testCommand) CommandType() string {
	if c.typeName == "" {
		return "test_command"
	}
	return c.typeName
}

func (c testCommand) Validate() error {
	return nil
}

func TestDispatchRejectsNilCommand(t *testing.T) {
	sender := &testSender{}
	dispatcher := NewCommandDispatcher(sender)

	err := dispatcher.Dispatch(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil command")
	}
	if !strings.Contains(err.Error(), "command is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if sender.calls != 0 {
		t.Fatalf("expected sender not to be called, got %d calls", sender.calls)
	}
}

func TestDispatchReturnsExecuteError(t *testing.T) {
	sender := &testSender{}
	dispatcher := NewCommandDispatcher(sender)

	err := dispatcher.Dispatch(context.Background(), testCommand{typeName: "failing", executeErr: errors.New("validation failed")})
	if err == nil {
		t.Fatal("expected execute error")
	}
	if !strings.Contains(err.Error(), "execute command failing") {
		t.Fatalf("unexpected error: %v", err)
	}
	if sender.calls != 0 {
		t.Fatalf("expected sender not to be called, got %d calls", sender.calls)
	}
}

func TestDispatchReturnsSendError(t *testing.T) {
	sender := &testSender{err: errors.New("transport down")}
	dispatcher := NewCommandDispatcher(sender)

	err := dispatcher.Dispatch(context.Background(), testCommand{typeName: "send_test"})
	if err == nil {
		t.Fatal("expected send error")
	}
	if !strings.Contains(err.Error(), "send command send_test") {
		t.Fatalf("unexpected error: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("expected sender called once, got %d", sender.calls)
	}
}

func TestDispatchSuccess(t *testing.T) {
	sender := &testSender{}
	dispatcher := NewCommandDispatcher(sender)

	err := dispatcher.Dispatch(context.Background(), testCommand{typeName: "ok"})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("expected sender called once, got %d", sender.calls)
	}
}
