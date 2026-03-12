package mqtt

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/paul/clock-server/internal/domain"
)

// ---------------------------------------------------------------------------
// Helpers: mock MQTT broker
// ---------------------------------------------------------------------------

// brokerBehavior controls what a mockBroker does when a client connects.
type brokerBehavior int

const (
	behaviorAccept         brokerBehavior = iota // send valid CONNACK, then send PUBACK for every PUBLISH
	behaviorRejectConnAck                        // send CONNACK with non-zero return code
	behaviorInvalidConnAck                       // send garbage instead of CONNACK
	behaviorCloseOnConnect                       // close connection immediately without sending anything
	behaviorAcceptNoPubAck                       // send valid CONNACK but never send PUBACK (causes timeout on QoS 1)
	behaviorCloseOnPublish                       // accept connection then close on first PUBLISH
)

// mockBroker is a simple in-process TCP listener that behaves like a minimal MQTT broker.
type mockBroker struct {
	t        *testing.T
	ln       net.Listener
	behavior brokerBehavior

	mu      sync.Mutex
	conns   []net.Conn
	pubRecv [][]byte // raw payloads received from PUBLISH packets
}

func newMockBroker(t *testing.T, behavior brokerBehavior) *mockBroker {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	mb := &mockBroker{t: t, ln: ln, behavior: behavior}
	go mb.acceptLoop()
	return mb
}

func (mb *mockBroker) addr() string {
	return mb.ln.Addr().String()
}

func (mb *mockBroker) host() string {
	h, _, _ := net.SplitHostPort(mb.ln.Addr().String())
	return h
}

func (mb *mockBroker) port() string {
	_, p, _ := net.SplitHostPort(mb.ln.Addr().String())
	return p
}

func (mb *mockBroker) mqttURL() string {
	return "mqtt://" + mb.addr()
}

func (mb *mockBroker) close() {
	_ = mb.ln.Close()
	mb.mu.Lock()
	defer mb.mu.Unlock()
	for _, c := range mb.conns {
		_ = c.Close()
	}
}

func (mb *mockBroker) receivedPayloads() [][]byte {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	out := make([][]byte, len(mb.pubRecv))
	copy(out, mb.pubRecv)
	return out
}

func (mb *mockBroker) acceptLoop() {
	for {
		conn, err := mb.ln.Accept()
		if err != nil {
			return
		}
		mb.mu.Lock()
		mb.conns = append(mb.conns, conn)
		mb.mu.Unlock()
		go mb.handleConn(conn)
	}
}

func (mb *mockBroker) handleConn(conn net.Conn) {
	defer conn.Close()

	switch mb.behavior {
	case behaviorCloseOnConnect:
		return

	case behaviorInvalidConnAck:
		// Send garbage bytes
		_, _ = conn.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF})
		return

	case behaviorRejectConnAck:
		// Read CONNECT packet (consume it)
		_ = readMQTTPacket(conn)
		// Send CONNACK with return code 5 (not authorized)
		_, _ = conn.Write([]byte{0x20, 0x02, 0x00, 0x05})
		return

	case behaviorAccept, behaviorAcceptNoPubAck, behaviorCloseOnPublish:
		// Read CONNECT packet
		_ = readMQTTPacket(conn)
		// Send valid CONNACK
		_, _ = conn.Write([]byte{0x20, 0x02, 0x00, 0x00})

		if mb.behavior == behaviorCloseOnPublish {
			// Close connection immediately so the next write by the client fails
			return
		}

		// Loop reading PUBLISH packets and sending PUBACKs (if QoS 1)
		for {
			pkt := readMQTTPacket(conn)
			if pkt == nil {
				return
			}
			if len(pkt) == 0 {
				return
			}
			fixedHeader := pkt[0]
			if fixedHeader>>4 != 0x03 { // not PUBLISH
				continue
			}
			qos := (fixedHeader >> 1) & 0x03

			// Extract topic length + payload for recording
			if len(pkt) < 3 {
				continue
			}
			remainingStart := 1 + varIntLen(pkt[1:])
			if remainingStart >= len(pkt) {
				continue
			}
			remaining := pkt[remainingStart:]

			if len(remaining) < 2 {
				continue
			}
			topicLen := int(binary.BigEndian.Uint16(remaining[0:2]))
			if len(remaining) < 2+topicLen {
				continue
			}
			offset := 2 + topicLen

			var packetID uint16
			if qos > 0 {
				if len(remaining) < offset+2 {
					continue
				}
				packetID = binary.BigEndian.Uint16(remaining[offset : offset+2])
				offset += 2
			}

			payload := remaining[offset:]
			mb.mu.Lock()
			mb.pubRecv = append(mb.pubRecv, payload)
			mb.mu.Unlock()

			if qos == 1 && mb.behavior != behaviorAcceptNoPubAck {
				puback := make([]byte, 4)
				puback[0] = 0x40
				puback[1] = 0x02
				binary.BigEndian.PutUint16(puback[2:], packetID)
				_, _ = conn.Write(puback)
			}
		}
	}
}

// readMQTTPacket reads one complete MQTT packet from r.
// Returns nil on error.
func readMQTTPacket(r io.Reader) []byte {
	_ = r.(*net.TCPConn) // type assertion only used to guard
	return readMQTTPacketFromReader(r)
}

func readMQTTPacketFromReader(r io.Reader) []byte {
	header := make([]byte, 1)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil
	}

	// Decode variable-length remaining-length
	var remaining int
	var multiplier int = 1
	for {
		b := make([]byte, 1)
		if _, err := io.ReadFull(r, b); err != nil {
			return nil
		}
		remaining += int(b[0]&0x7F) * multiplier
		multiplier *= 128
		if b[0]&0x80 == 0 {
			break
		}
		if multiplier > 128*128*128 {
			return nil // malformed
		}
	}

	body := make([]byte, remaining)
	if remaining > 0 {
		if _, err := io.ReadFull(r, body); err != nil {
			return nil
		}
	}

	result := make([]byte, 1+remaining)
	result[0] = header[0]
	copy(result[1:], body)
	return result
}

// varIntLen returns how many bytes are used by the variable-length integer starting at b.
func varIntLen(b []byte) int {
	for i, v := range b {
		if v&0x80 == 0 {
			return i + 1
		}
		if i >= 3 {
			break
		}
	}
	return 1
}

// newSenderWithBroker creates a Sender connected to mb, using insecure transport.
func newSenderWithBroker(t *testing.T, mb *mockBroker, overrides func(*Config)) (*Sender, error) {
	t.Helper()
	cfg := Config{
		BrokerURL:              mb.mqttURL(),
		ClientID:               "test-client",
		TopicPrefix:            "test/commands",
		QoS:                    0,
		AllowInsecureTransport: true,
	}
	if overrides != nil {
		overrides(&cfg)
	}
	return NewSender(cfg)
}

// ---------------------------------------------------------------------------
// Unit tests: buildTopic
// ---------------------------------------------------------------------------

func TestBuildTopic(t *testing.T) {
	cmd := domain.SetBrightnessCommand{DeviceID: "clock-9", Level: 50}
	topic := buildTopic("clocks/commands", cmd)
	if topic != "clocks/commands/clock-9/set_brightness" {
		t.Fatalf("unexpected topic: %s", topic)
	}
}

func TestBuildTopicStripsLeadingTrailingSlashes(t *testing.T) {
	cmd := domain.SetBrightnessCommand{DeviceID: "dev1", Level: 10}
	topic := buildTopic("/clocks/commands/", cmd)
	if topic != "clocks/commands/dev1/set_brightness" {
		t.Fatalf("unexpected topic: %s", topic)
	}
}

func TestBuildTopicAllCommandTypes(t *testing.T) {
	cases := []struct {
		cmd      domain.ClockCommand
		wantSufx string
	}{
		{domain.SetAlarmCommand{DeviceID: "d1", AlarmTime: time.Now().Add(time.Hour)}, "/set_alarm"},
		{domain.DisplayMessageCommand{DeviceID: "d2", Message: "hi", DurationSeconds: 5}, "/display_message"},
		{domain.SetBrightnessCommand{DeviceID: "d3", Level: 75}, "/set_brightness"},
	}
	for _, tc := range cases {
		topic := buildTopic("prefix", tc.cmd)
		if !strings.HasSuffix(topic, tc.wantSufx) {
			t.Errorf("topic %q does not end with %q", topic, tc.wantSufx)
		}
	}
}

// ---------------------------------------------------------------------------
// Unit tests: buildPayload
// ---------------------------------------------------------------------------

func TestBuildPayloadAlarm(t *testing.T) {
	alarmTime := time.Date(2030, 1, 1, 6, 0, 0, 0, time.UTC)
	cmd := domain.SetAlarmCommand{DeviceID: "clock-1", AlarmTime: alarmTime, Label: "wake"}
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
	if payload["deviceId"] != "clock-1" {
		t.Fatalf("unexpected deviceId: %v", payload["deviceId"])
	}
	wantTime := alarmTime.Format(time.RFC3339)
	if payload["alarmTime"] != wantTime {
		t.Fatalf("unexpected alarmTime: %v, want %s", payload["alarmTime"], wantTime)
	}
}

func TestBuildPayloadDisplayMessage(t *testing.T) {
	cmd := domain.DisplayMessageCommand{DeviceID: "dev-2", Message: "hello world", DurationSeconds: 30}
	payload, err := buildPayload(cmd)
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	if payload["type"] != "display_message" {
		t.Fatalf("unexpected type: %v", payload["type"])
	}
	if payload["message"] != "hello world" {
		t.Fatalf("unexpected message: %v", payload["message"])
	}
	if payload["durationSeconds"] != 30 {
		t.Fatalf("unexpected durationSeconds: %v", payload["durationSeconds"])
	}
	if payload["deviceId"] != "dev-2" {
		t.Fatalf("unexpected deviceId: %v", payload["deviceId"])
	}
}

func TestBuildPayloadSetBrightness(t *testing.T) {
	cmd := domain.SetBrightnessCommand{DeviceID: "dev-3", Level: 80}
	payload, err := buildPayload(cmd)
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	if payload["type"] != "set_brightness" {
		t.Fatalf("unexpected type: %v", payload["type"])
	}
	if payload["level"] != 80 {
		t.Fatalf("unexpected level: %v", payload["level"])
	}
	if payload["deviceId"] != "dev-3" {
		t.Fatalf("unexpected deviceId: %v", payload["deviceId"])
	}
}

func TestBuildPayloadUnsupportedCommand(t *testing.T) {
	_, err := buildPayload(unknownCmd{})
	if err == nil {
		t.Fatal("expected error for unsupported command type")
	}
	if !strings.Contains(err.Error(), "unsupported command type") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// unknownCmd is a fake command type not handled by buildPayload.
type unknownCmd struct{}

func (u unknownCmd) Execute(_ context.Context) error { return nil }
func (u unknownCmd) TargetDeviceID() string          { return "dev-x" }
func (u unknownCmd) CommandType() string             { return "unknown" }
func (u unknownCmd) Validate() error                 { return nil }

// ---------------------------------------------------------------------------
// Unit tests: parseBrokerURL
// ---------------------------------------------------------------------------

func TestParseBrokerURL_InsecureMQTTBlocked(t *testing.T) {
	_, _, _, err := parseBrokerURL("mqtt://localhost:1883", false)
	if err == nil {
		t.Fatal("expected error for insecure mqtt:// without AllowInsecureTransport")
	}
}

func TestParseBrokerURL_InsecureMQTTAllowed(t *testing.T) {
	host, port, tls, err := parseBrokerURL("mqtt://broker.example.com:1883", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "broker.example.com" {
		t.Errorf("host = %q, want %q", host, "broker.example.com")
	}
	if port != "1883" {
		t.Errorf("port = %q, want %q", port, "1883")
	}
	if tls {
		t.Error("expected tlsEnabled=false for mqtt://")
	}
}

func TestParseBrokerURL_MQTTs(t *testing.T) {
	host, port, tlsEnabled, err := parseBrokerURL("mqtts://secure.broker.com:8883", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "secure.broker.com" {
		t.Errorf("host = %q", host)
	}
	if port != "8883" {
		t.Errorf("port = %q", port)
	}
	if !tlsEnabled {
		t.Error("expected tlsEnabled=true for mqtts://")
	}
}

func TestParseBrokerURL_DefaultPort_MQTT(t *testing.T) {
	_, port, _, err := parseBrokerURL("mqtt://localhost", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port != "1883" {
		t.Errorf("expected default port 1883, got %q", port)
	}
}

func TestParseBrokerURL_DefaultPort_MQTTs(t *testing.T) {
	_, port, _, err := parseBrokerURL("mqtts://localhost", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port != "8883" {
		t.Errorf("expected default port 8883, got %q", port)
	}
}

func TestParseBrokerURL_UnsupportedScheme(t *testing.T) {
	_, _, _, err := parseBrokerURL("ws://localhost:9001", false)
	if err == nil {
		t.Fatal("expected error for unsupported scheme")
	}
	if !strings.Contains(err.Error(), "unsupported mqtt scheme") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseBrokerURL_EmptyHost(t *testing.T) {
	_, _, _, err := parseBrokerURL("mqtts://:8883", false)
	if err == nil {
		t.Fatal("expected error for empty host")
	}
}

func TestParseBrokerURL_InvalidURL(t *testing.T) {
	_, _, _, err := parseBrokerURL("://not-valid", false)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

// ---------------------------------------------------------------------------
// Unit tests: NewSender config validation
// ---------------------------------------------------------------------------

func TestNewSender_EmptyBrokerURL(t *testing.T) {
	_, err := NewSender(Config{BrokerURL: "", AllowInsecureTransport: true})
	if err == nil {
		t.Fatal("expected error for empty broker URL")
	}
	if !strings.Contains(err.Error(), "broker url is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewSender_WhitespaceBrokerURL(t *testing.T) {
	_, err := NewSender(Config{BrokerURL: "   ", AllowInsecureTransport: true})
	if err == nil {
		t.Fatal("expected error for whitespace-only broker URL")
	}
}

func TestNewSender_QoS2Rejected(t *testing.T) {
	_, err := NewSender(Config{
		BrokerURL:              "mqtts://localhost:8883",
		QoS:                    2,
		AllowInsecureTransport: false,
	})
	if err == nil {
		t.Fatal("expected error for QoS 2")
	}
	if !strings.Contains(err.Error(), "unsupported qos 2") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewSender_TLSInsecureSkipVerifyWithoutAllowFlag(t *testing.T) {
	_, err := NewSender(Config{
		BrokerURL:             "mqtts://localhost:8883",
		TLSInsecureSkipVerify: true,
		AllowInsecureTLS:      false,
	})
	if err == nil {
		t.Fatal("expected error: TLSInsecureSkipVerify requires AllowInsecureTLS")
	}
	if !strings.Contains(err.Error(), "ALLOW_INSECURE_TLS_VERIFY=true") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewSender_TLSInsecureSkipVerifyWithAllowFlag(t *testing.T) {
	// This should fail at the connection stage, not at config validation
	_, err := NewSender(Config{
		BrokerURL:             "mqtts://127.0.0.1:19999", // nothing listening
		TLSInsecureSkipVerify: true,
		AllowInsecureTLS:      true,
	})
	// Should fail with a dial error, not a config validation error
	if err == nil {
		t.Fatal("expected a dial error")
	}
	if strings.Contains(err.Error(), "ALLOW_INSECURE_TLS_VERIFY") {
		t.Fatalf("got config error instead of dial error: %v", err)
	}
}

func TestNewSender_DefaultTopicPrefix(t *testing.T) {
	mb := newMockBroker(t, behaviorAccept)
	defer mb.close()
	s, err := NewSender(Config{
		BrokerURL:              mb.mqttURL(),
		ClientID:               "test",
		TopicPrefix:            "",  // should get default
		AllowInsecureTransport: true,
	})
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer s.Close()
	if s.cfg.TopicPrefix != "clocks/commands" {
		t.Errorf("expected default topic prefix 'clocks/commands', got %q", s.cfg.TopicPrefix)
	}
}

func TestNewSender_DefaultClientID(t *testing.T) {
	mb := newMockBroker(t, behaviorAccept)
	defer mb.close()
	s, err := NewSender(Config{
		BrokerURL:              mb.mqttURL(),
		ClientID:               "", // should get auto-generated
		TopicPrefix:            "t",
		AllowInsecureTransport: true,
	})
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer s.Close()
	if s.cfg.ClientID == "" {
		t.Error("expected auto-generated client ID, got empty string")
	}
}

// ---------------------------------------------------------------------------
// Integration-style tests: connection lifecycle
// ---------------------------------------------------------------------------

func TestNewSender_ConnectSuccess(t *testing.T) {
	mb := newMockBroker(t, behaviorAccept)
	defer mb.close()

	s, err := newSenderWithBroker(t, mb, nil)
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer s.Close()

	if err := s.Check(context.Background()); err != nil {
		t.Fatalf("Check after connect: %v", err)
	}
}

func TestNewSender_BrokerRefusesConnection(t *testing.T) {
	mb := newMockBroker(t, behaviorCloseOnConnect)
	defer mb.close()

	_, err := newSenderWithBroker(t, mb, nil)
	if err == nil {
		t.Fatal("expected error when broker closes connection immediately")
	}
}

func TestNewSender_BrokerSendsInvalidConnAck(t *testing.T) {
	mb := newMockBroker(t, behaviorInvalidConnAck)
	defer mb.close()

	_, err := newSenderWithBroker(t, mb, nil)
	if err == nil {
		t.Fatal("expected error for invalid CONNACK")
	}
}

func TestNewSender_BrokerRejectsConnAck(t *testing.T) {
	mb := newMockBroker(t, behaviorRejectConnAck)
	defer mb.close()

	_, err := newSenderWithBroker(t, mb, nil)
	if err == nil {
		t.Fatal("expected error for rejected CONNACK")
	}
	if !strings.Contains(err.Error(), "connack error code") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewSender_NoListenerAtAddress(t *testing.T) {
	_, err := NewSender(Config{
		BrokerURL:              "mqtt://127.0.0.1:19998",
		ClientID:               "test",
		TopicPrefix:            "t",
		AllowInsecureTransport: true,
	})
	if err == nil {
		t.Fatal("expected dial error")
	}
	if !strings.Contains(err.Error(), "dial mqtt broker") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClose_DisconnectsClient(t *testing.T) {
	mb := newMockBroker(t, behaviorAccept)
	defer mb.close()

	s, err := newSenderWithBroker(t, mb, nil)
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}

	s.Close()

	if err := s.Check(context.Background()); err == nil {
		t.Fatal("expected Check to fail after Close")
	}
}

func TestClose_IdempotentDoubleClose(t *testing.T) {
	mb := newMockBroker(t, behaviorAccept)
	defer mb.close()

	s, err := newSenderWithBroker(t, mb, nil)
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}

	s.Close()
	s.Close() // should not panic
}

// ---------------------------------------------------------------------------
// Integration-style tests: Send — publish messages
// ---------------------------------------------------------------------------

func TestSend_NilCommand(t *testing.T) {
	mb := newMockBroker(t, behaviorAccept)
	defer mb.close()

	s, err := newSenderWithBroker(t, mb, nil)
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer s.Close()

	if err := s.Send(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil command")
	}
}

func TestSend_SetAlarm_QoS0(t *testing.T) {
	mb := newMockBroker(t, behaviorAccept)
	defer mb.close()

	s, err := newSenderWithBroker(t, mb, func(c *Config) { c.QoS = 0 })
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer s.Close()

	cmd := domain.SetAlarmCommand{
		DeviceID:  "clock-1",
		AlarmTime: time.Now().Add(time.Hour),
		Label:     "morning",
	}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Give broker goroutine time to record the payload
	time.Sleep(20 * time.Millisecond)
	payloads := mb.receivedPayloads()
	if len(payloads) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(payloads))
	}
	if !bytes.Contains(payloads[0], []byte("set_alarm")) {
		t.Errorf("payload does not contain 'set_alarm': %s", payloads[0])
	}
}

func TestSend_SetAlarm_QoS1(t *testing.T) {
	mb := newMockBroker(t, behaviorAccept)
	defer mb.close()

	s, err := newSenderWithBroker(t, mb, func(c *Config) { c.QoS = 1 })
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer s.Close()

	cmd := domain.SetAlarmCommand{
		DeviceID:  "clock-1",
		AlarmTime: time.Now().Add(time.Hour),
		Label:     "morning",
	}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("Send QoS 1: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	payloads := mb.receivedPayloads()
	if len(payloads) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(payloads))
	}
}

func TestSend_DisplayMessage(t *testing.T) {
	mb := newMockBroker(t, behaviorAccept)
	defer mb.close()

	s, err := newSenderWithBroker(t, mb, nil)
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer s.Close()

	cmd := domain.DisplayMessageCommand{DeviceID: "clock-2", Message: "hello", DurationSeconds: 10}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("Send: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	payloads := mb.receivedPayloads()
	if len(payloads) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(payloads))
	}
	if !bytes.Contains(payloads[0], []byte("display_message")) {
		t.Errorf("payload missing 'display_message': %s", payloads[0])
	}
	if !bytes.Contains(payloads[0], []byte("hello")) {
		t.Errorf("payload missing message text: %s", payloads[0])
	}
}

func TestSend_SetBrightness(t *testing.T) {
	mb := newMockBroker(t, behaviorAccept)
	defer mb.close()

	s, err := newSenderWithBroker(t, mb, nil)
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer s.Close()

	cmd := domain.SetBrightnessCommand{DeviceID: "clock-3", Level: 75}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("Send: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	payloads := mb.receivedPayloads()
	if len(payloads) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(payloads))
	}
	if !bytes.Contains(payloads[0], []byte("set_brightness")) {
		t.Errorf("payload missing 'set_brightness': %s", payloads[0])
	}
}

func TestSend_MultipleCommands(t *testing.T) {
	mb := newMockBroker(t, behaviorAccept)
	defer mb.close()

	s, err := newSenderWithBroker(t, mb, nil)
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer s.Close()

	cmds := []domain.ClockCommand{
		domain.SetBrightnessCommand{DeviceID: "d1", Level: 10},
		domain.DisplayMessageCommand{DeviceID: "d2", Message: "hi", DurationSeconds: 5},
		domain.SetAlarmCommand{DeviceID: "d3", AlarmTime: time.Now().Add(time.Hour), Label: "test"},
	}
	for i, cmd := range cmds {
		if err := s.Send(context.Background(), cmd); err != nil {
			t.Fatalf("Send[%d]: %v", i, err)
		}
	}

	time.Sleep(50 * time.Millisecond)
	payloads := mb.receivedPayloads()
	if len(payloads) != 3 {
		t.Fatalf("expected 3 payloads, got %d", len(payloads))
	}
}

func TestSend_RetainedFlag(t *testing.T) {
	mb := newMockBroker(t, behaviorAccept)
	defer mb.close()

	s, err := newSenderWithBroker(t, mb, func(c *Config) { c.Retained = true })
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer s.Close()

	cmd := domain.SetBrightnessCommand{DeviceID: "dev-r", Level: 50}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

func TestSend_WithUsernameAndPassword(t *testing.T) {
	mb := newMockBroker(t, behaviorAccept)
	defer mb.close()

	s, err := newSenderWithBroker(t, mb, func(c *Config) {
		c.Username = "user1"
		c.Password = "s3cr3t"
	})
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer s.Close()

	cmd := domain.SetBrightnessCommand{DeviceID: "dev-auth", Level: 30}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Integration-style tests: error paths
// ---------------------------------------------------------------------------

func TestSend_CancelledContext(t *testing.T) {
	mb := newMockBroker(t, behaviorAccept)
	defer mb.close()

	s, err := newSenderWithBroker(t, mb, nil)
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	cmd := domain.SetBrightnessCommand{DeviceID: "dev-1", Level: 50}
	err = s.Send(ctx, cmd)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestSend_PublishFailureThenReconnectWithRetry(t *testing.T) {
	// Broker closes connection after CONNACK — simulates dropped connection
	mb := newMockBroker(t, behaviorCloseOnPublish)
	defer mb.close()

	s, err := newSenderWithBroker(t, mb, func(c *Config) {
		c.ConnectRetry = true
	})
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer s.Close()

	cmd := domain.SetBrightnessCommand{DeviceID: "dev-1", Level: 50}
	// With ConnectRetry=true the sender will try 3 times; all should fail since
	// the broker always closes on publish.
	err = s.Send(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error after all retry attempts")
	}
	if !strings.Contains(err.Error(), "mqtt publish failed after 3 attempts") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSend_PublishFailureNoRetry(t *testing.T) {
	mb := newMockBroker(t, behaviorCloseOnPublish)
	defer mb.close()

	s, err := newSenderWithBroker(t, mb, func(c *Config) {
		c.ConnectRetry = false
	})
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer s.Close()

	cmd := domain.SetBrightnessCommand{DeviceID: "dev-1", Level: 50}
	err = s.Send(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error when publish fails")
	}
	if !strings.Contains(err.Error(), "mqtt publish failed after 1 attempts") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSend_QoS1_PubAckTimeout(t *testing.T) {
	// Broker accepts but never sends PUBACK
	mb := newMockBroker(t, behaviorAcceptNoPubAck)
	defer mb.close()

	s, err := newSenderWithBroker(t, mb, func(c *Config) { c.QoS = 1 })
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	// Shorten publish timeout to make the test fast
	s.pubTimeout = 100 * time.Millisecond
	defer s.Close()

	cmd := domain.SetBrightnessCommand{DeviceID: "dev-1", Level: 50}
	start := time.Now()
	err = s.Send(context.Background(), cmd)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error waiting for PUBACK")
	}
	if elapsed < 80*time.Millisecond {
		t.Errorf("timeout fired too early: %v", elapsed)
	}
}

func TestSend_QoS1_PubAckPacketIDMismatch(t *testing.T) {
	// Use a broker that sends a PUBACK with wrong packet ID.
	mb := newMockBrokerWithCustomPubAck(t)
	defer mb.close()

	s, err := newSenderWithBroker(t, mb, func(c *Config) { c.QoS = 1 })
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer s.Close()

	cmd := domain.SetBrightnessCommand{DeviceID: "dev-1", Level: 50}
	err = s.Send(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected packet ID mismatch error")
	}
	if !strings.Contains(err.Error(), "puback packet id mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// newMockBrokerWithCustomPubAck returns a broker that sends PUBACK with wrong ID.
func newMockBrokerWithCustomPubAck(t *testing.T) *mockBroker {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	mb := &mockBroker{t: t, ln: ln, behavior: behaviorAccept}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		mb.mu.Lock()
		mb.conns = append(mb.conns, conn)
		mb.mu.Unlock()
		defer conn.Close()

		// Read CONNECT, send CONNACK
		_ = readMQTTPacketFromReader(conn)
		_, _ = conn.Write([]byte{0x20, 0x02, 0x00, 0x00})

		// Read PUBLISH, send PUBACK with wrong packet ID (0xFFFF)
		_ = readMQTTPacketFromReader(conn)
		puback := make([]byte, 4)
		puback[0] = 0x40
		puback[1] = 0x02
		binary.BigEndian.PutUint16(puback[2:], 0xFFFF) // wrong ID
		_, _ = conn.Write(puback)
	}()
	return mb
}

// ---------------------------------------------------------------------------
// Unit tests: Check
// ---------------------------------------------------------------------------

func TestCheck_NotConnected(t *testing.T) {
	s := &Sender{}
	if err := s.Check(context.Background()); err == nil {
		t.Fatal("expected error when not connected")
	}
}

func TestCheck_Connected(t *testing.T) {
	mb := newMockBroker(t, behaviorAccept)
	defer mb.close()

	s, err := newSenderWithBroker(t, mb, nil)
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer s.Close()

	if err := s.Check(context.Background()); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Unit tests: packet encoding helpers
// ---------------------------------------------------------------------------

func TestEncodeString(t *testing.T) {
	cases := []struct {
		input string
		want  []byte
	}{
		{"", []byte{0x00, 0x00}},
		{"A", []byte{0x00, 0x01, 0x41}},
		{"MQTT", []byte{0x00, 0x04, 'M', 'Q', 'T', 'T'}},
	}
	for _, tc := range cases {
		got := encodeString(tc.input)
		if !bytes.Equal(got, tc.want) {
			t.Errorf("encodeString(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestEncodeRemainingLength(t *testing.T) {
	cases := []struct {
		length int
		want   []byte
	}{
		{0, []byte{0x00}},
		{127, []byte{0x7F}},
		{128, []byte{0x80, 0x01}},
		{16383, []byte{0xFF, 0x7F}},
		{16384, []byte{0x80, 0x80, 0x01}},
	}
	for _, tc := range cases {
		got := encodeRemainingLength(tc.length)
		if !bytes.Equal(got, tc.want) {
			t.Errorf("encodeRemainingLength(%d) = %v, want %v", tc.length, got, tc.want)
		}
	}
}

func TestBuildPublishPacket_QoS0(t *testing.T) {
	pkt, err := buildPublishPacket("test/topic", []byte("hello"), 0, false, 0)
	if err != nil {
		t.Fatalf("buildPublishPacket: %v", err)
	}
	// Fixed header: 0x30 (PUBLISH, QoS 0, not retained)
	if pkt[0] != 0x30 {
		t.Errorf("fixed header = 0x%02x, want 0x30", pkt[0])
	}
}

func TestBuildPublishPacket_QoS1(t *testing.T) {
	pkt, err := buildPublishPacket("test/topic", []byte("hello"), 1, false, 42)
	if err != nil {
		t.Fatalf("buildPublishPacket: %v", err)
	}
	// Fixed header: 0x32 (PUBLISH, QoS 1)
	if pkt[0] != 0x32 {
		t.Errorf("fixed header = 0x%02x, want 0x32", pkt[0])
	}
}

func TestBuildPublishPacket_Retained(t *testing.T) {
	pkt, err := buildPublishPacket("test/topic", []byte("hello"), 0, true, 0)
	if err != nil {
		t.Fatalf("buildPublishPacket: %v", err)
	}
	// Fixed header: 0x31 (PUBLISH, QoS 0, retained)
	if pkt[0] != 0x31 {
		t.Errorf("fixed header = 0x%02x, want 0x31", pkt[0])
	}
}

func TestBuildPublishPacket_EmptyTopic(t *testing.T) {
	_, err := buildPublishPacket("", []byte("hello"), 0, false, 0)
	if err == nil {
		t.Fatal("expected error for empty topic")
	}
	if !strings.Contains(err.Error(), "mqtt topic is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildPublishPacket_WhitespaceTopic(t *testing.T) {
	_, err := buildPublishPacket("   ", []byte("hello"), 0, false, 0)
	if err == nil {
		t.Fatal("expected error for whitespace-only topic")
	}
}

func TestBuildPublishPacket_UnsupportedQoS(t *testing.T) {
	_, err := buildPublishPacket("t", []byte("p"), 2, false, 0)
	if err == nil {
		t.Fatal("expected error for QoS 2")
	}
	if !strings.Contains(err.Error(), "unsupported qos 2") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildPublishPacket_EmptyPayload(t *testing.T) {
	pkt, err := buildPublishPacket("t/empty", []byte{}, 0, false, 0)
	if err != nil {
		t.Fatalf("buildPublishPacket empty payload: %v", err)
	}
	if len(pkt) == 0 {
		t.Error("expected non-empty packet even with empty payload")
	}
}

func TestReadConnAck_ValidResponse(t *testing.T) {
	buf := bytes.NewReader([]byte{0x20, 0x02, 0x00, 0x00})
	if err := readConnAck(buf); err != nil {
		t.Fatalf("readConnAck: %v", err)
	}
}

func TestReadConnAck_ErrorCode(t *testing.T) {
	codes := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	for _, code := range codes {
		buf := bytes.NewReader([]byte{0x20, 0x02, 0x00, code})
		err := readConnAck(buf)
		if err == nil {
			t.Errorf("expected error for CONNACK return code %d", code)
		}
		if !strings.Contains(err.Error(), "connack error code") {
			t.Errorf("unexpected error for code %d: %v", code, err)
		}
	}
}

func TestReadConnAck_WrongFixedHeader(t *testing.T) {
	buf := bytes.NewReader([]byte{0x10, 0x02, 0x00, 0x00})
	err := readConnAck(buf)
	if err == nil {
		t.Fatal("expected error for wrong fixed header")
	}
	if !strings.Contains(err.Error(), "invalid connack header") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadConnAck_TruncatedResponse(t *testing.T) {
	buf := bytes.NewReader([]byte{0x20, 0x02})
	err := readConnAck(buf)
	if err == nil {
		t.Fatal("expected error for truncated CONNACK")
	}
}

func TestReadPubAck_Valid(t *testing.T) {
	buf := make([]byte, 4)
	buf[0] = 0x40
	buf[1] = 0x02
	binary.BigEndian.PutUint16(buf[2:], 7)
	if err := readPubAck(bytes.NewReader(buf), 7); err != nil {
		t.Fatalf("readPubAck: %v", err)
	}
}

func TestReadPubAck_WrongFixedHeader(t *testing.T) {
	buf := []byte{0x30, 0x02, 0x00, 0x01}
	err := readPubAck(bytes.NewReader(buf), 1)
	if err == nil {
		t.Fatal("expected error for wrong fixed header")
	}
	if !strings.Contains(err.Error(), "invalid puback header") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadPubAck_PacketIDMismatch(t *testing.T) {
	buf := make([]byte, 4)
	buf[0] = 0x40
	buf[1] = 0x02
	binary.BigEndian.PutUint16(buf[2:], 5) // actual ID = 5
	err := readPubAck(bytes.NewReader(buf), 99) // expected ID = 99
	if err == nil {
		t.Fatal("expected packet ID mismatch error")
	}
	if !strings.Contains(err.Error(), "puback packet id mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadPubAck_Truncated(t *testing.T) {
	err := readPubAck(bytes.NewReader([]byte{0x40, 0x02}), 1)
	if err == nil {
		t.Fatal("expected error for truncated PUBACK")
	}
}

func TestWriteConnectPacket_Basic(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{ClientID: "test-client"}
	if err := writeConnectPacket(&buf, cfg); err != nil {
		t.Fatalf("writeConnectPacket: %v", err)
	}
	data := buf.Bytes()
	// Fixed header = 0x10 (CONNECT)
	if data[0] != 0x10 {
		t.Errorf("expected CONNECT fixed header 0x10, got 0x%02x", data[0])
	}
}

func TestWriteConnectPacket_WithCredentials(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{ClientID: "client-x", Username: "user1", Password: "pass1"}
	if err := writeConnectPacket(&buf, cfg); err != nil {
		t.Fatalf("writeConnectPacket: %v", err)
	}
	data := buf.Bytes()
	if data[0] != 0x10 {
		t.Errorf("expected CONNECT fixed header 0x10, got 0x%02x", data[0])
	}
	// The packet should contain the username string somewhere
	if !bytes.Contains(data, []byte("user1")) {
		t.Error("expected packet to contain username")
	}
	if !bytes.Contains(data, []byte("pass1")) {
		t.Error("expected packet to contain password")
	}
}

func TestWriteConnectPacket_WriteError(t *testing.T) {
	// Use an io.Writer that always returns an error
	w := &errWriter{err: errors.New("write failed")}
	cfg := Config{ClientID: "test"}
	err := writeConnectPacket(w, cfg)
	if err == nil {
		t.Fatal("expected error from failing writer")
	}
}

// errWriter always returns an error on Write.
type errWriter struct {
	err error
}

func (e *errWriter) Write(_ []byte) (int, error) {
	return 0, e.err
}

// ---------------------------------------------------------------------------
// Unit tests: packetID wrapping
// ---------------------------------------------------------------------------

func TestPacketID_Wrapping(t *testing.T) {
	mb := newMockBroker(t, behaviorAccept)
	defer mb.close()

	s, err := newSenderWithBroker(t, mb, func(c *Config) { c.QoS = 1 })
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer s.Close()

	// Force packetID to wrap around
	s.mu.Lock()
	s.packetID = 0xFFFE
	s.mu.Unlock()

	cmd := domain.SetBrightnessCommand{DeviceID: "d1", Level: 50}
	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("Send at 0xFFFF: %v", err)
	}

	// Next increment should wrap to 1 (skipping 0)
	s.mu.Lock()
	s.packetID = 0xFFFF
	s.mu.Unlock()

	if err := s.Send(context.Background(), cmd); err != nil {
		t.Fatalf("Send after wrap: %v", err)
	}
}
