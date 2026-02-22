package composite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/paul/clock-server/internal/domain"
)

type mockSender struct {
	err   error
	calls int
}

func (m *mockSender) Send(_ context.Context, _ domain.ClockCommand) error {
	m.calls++
	return m.err
}

func TestCompositeSenderCallsAllSenders(t *testing.T) {
	one := &mockSender{}
	two := &mockSender{}
	sut := NewSender(one, two)
	cmd := domain.SetAlarmCommand{DeviceID: "clock-1", AlarmTime: time.Now().Add(time.Hour)}

	if err := sut.Send(context.Background(), cmd); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if one.calls != 1 || two.calls != 1 {
		t.Fatalf("expected each sender to be called once, got one=%d two=%d", one.calls, two.calls)
	}
}

func TestCompositeSenderCollectsErrors(t *testing.T) {
	failA := &mockSender{err: errors.New("a")}
	failB := &mockSender{err: errors.New("b")}
	sut := NewSender(failA, failB)
	cmd := domain.SetBrightnessCommand{DeviceID: "clock-1", Level: 20}

	err := sut.Send(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected joined error")
	}
	if failA.calls != 1 || failB.calls != 1 {
		t.Fatalf("expected each sender to be called once, got a=%d b=%d", failA.calls, failB.calls)
	}
}
