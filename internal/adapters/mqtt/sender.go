package mqtt

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/paul/clock-server/internal/domain"
)

const (
	defaultConnectTimeout = 10 * time.Second
	defaultPublishTimeout = 5 * time.Second
	defaultKeepAlive      = 60
)

// Config defines MQTT adapter settings.
type Config struct {
	BrokerURL              string
	ClientID               string
	Username               string
	Password               string
	TopicPrefix            string
	QoS                    byte
	Retained               bool
	ConnectRetry           bool
	TLSInsecureSkipVerify  bool
	AllowInsecureTLS       bool
	AllowInsecureTransport bool
}

// Sender publishes smart clock commands to an MQTT broker using a persistent in-process connection.
type Sender struct {
	cfg         Config
	brokerHost  string
	brokerPort  string
	tlsEnabled  bool
	conn        net.Conn
	mu          sync.Mutex
	packetID    uint16
	connTimeout time.Duration
	pubTimeout  time.Duration
}

// NewSender creates and connects an MQTT sender.
func NewSender(cfg Config) (*Sender, error) {
	if strings.TrimSpace(cfg.BrokerURL) == "" {
		return nil, errors.New("mqtt broker url is required")
	}
	if strings.TrimSpace(cfg.TopicPrefix) == "" {
		cfg.TopicPrefix = "clocks/commands"
	}
	if strings.TrimSpace(cfg.ClientID) == "" {
		cfg.ClientID = fmt.Sprintf("clock-dispatcher-%d", time.Now().UnixNano())
	}
	if cfg.QoS > 1 {
		return nil, fmt.Errorf("unsupported qos %d: only 0 and 1 are supported", cfg.QoS)
	}
	if cfg.TLSInsecureSkipVerify && !cfg.AllowInsecureTLS {
		return nil, errors.New("MQTT_TLS_INSECURE_SKIP_VERIFY requires ALLOW_INSECURE_TLS_VERIFY=true")
	}

	host, port, tlsEnabled, err := parseBrokerURL(cfg.BrokerURL, cfg.AllowInsecureTransport)
	if err != nil {
		return nil, err
	}

	s := &Sender{
		cfg:         cfg,
		brokerHost:  host,
		brokerPort:  port,
		tlsEnabled:  tlsEnabled,
		connTimeout: defaultConnectTimeout,
		pubTimeout:  defaultPublishTimeout,
	}
	if err := s.connect(); err != nil {
		return nil, err
	}
	return s, nil
}

// Send maps and publishes a command over MQTT.
func (s *Sender) Send(ctx context.Context, cmd domain.ClockCommand) error {
	if cmd == nil {
		return errors.New("command is required")
	}
	payload, err := buildPayload(cmd)
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal mqtt payload: %w", err)
	}
	topic := buildTopic(s.cfg.TopicPrefix, cmd)

	s.mu.Lock()
	defer s.mu.Unlock()

	attempts := 1
	if s.cfg.ConnectRetry {
		attempts = 3
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if s.conn == nil {
			if err := s.connect(); err != nil {
				lastErr = err
				continue
			}
		}
		if err := s.publish(topic, body); err != nil {
			lastErr = err
			s.closeLocked()
			continue
		}
		return nil
	}
	return fmt.Errorf("mqtt publish failed after %d attempts: %w", attempts, lastErr)
}

// Check verifies adapter readiness.
func (s *Sender) Check(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return errors.New("mqtt not connected")
	}
	return nil
}

// Close disconnects the mqtt client.
func (s *Sender) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked()
}

func (s *Sender) connect() error {
	address := net.JoinHostPort(s.brokerHost, s.brokerPort)
	dialer := net.Dialer{Timeout: s.connTimeout}

	var conn net.Conn
	var err error
	if s.tlsEnabled {
		conn, err = tls.DialWithDialer(&dialer, "tcp", address, &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: s.cfg.TLSInsecureSkipVerify,
		})
	} else {
		conn, err = dialer.Dial("tcp", address)
	}
	if err != nil {
		return fmt.Errorf("dial mqtt broker: %w", err)
	}

	if err := conn.SetDeadline(time.Now().Add(s.connTimeout)); err != nil {
		_ = conn.Close()
		return fmt.Errorf("set connect deadline: %w", err)
	}
	if err := writeConnectPacket(conn, s.cfg); err != nil {
		_ = conn.Close()
		return err
	}
	if err := readConnAck(conn); err != nil {
		_ = conn.Close()
		return err
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		_ = conn.Close()
		return fmt.Errorf("clear deadline: %w", err)
	}

	s.conn = conn
	return nil
}

func (s *Sender) publish(topic string, payload []byte) error {
	if s.conn == nil {
		return errors.New("mqtt connection is not established")
	}
	if err := s.conn.SetDeadline(time.Now().Add(s.pubTimeout)); err != nil {
		return fmt.Errorf("set publish deadline: %w", err)
	}
	defer func() { _ = s.conn.SetDeadline(time.Time{}) }()

	packetID := uint16(0)
	if s.cfg.QoS > 0 {
		s.packetID++
		if s.packetID == 0 {
			s.packetID = 1
		}
		packetID = s.packetID
	}

	packet, err := buildPublishPacket(topic, payload, s.cfg.QoS, s.cfg.Retained, packetID)
	if err != nil {
		return err
	}
	if _, err := s.conn.Write(packet); err != nil {
		return fmt.Errorf("write publish packet: %w", err)
	}
	if s.cfg.QoS == 1 {
		if err := readPubAck(s.conn, packetID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Sender) closeLocked() {
	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
}

func parseBrokerURL(raw string, allowInsecureTransport bool) (host, port string, tlsEnabled bool, err error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "", false, fmt.Errorf("invalid mqtt broker url: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "mqtt":
		if !allowInsecureTransport {
			return "", "", false, errors.New("insecure mqtt:// is disabled; use mqtts:// or set ALLOW_INSECURE_MQTT=true")
		}
		tlsEnabled = false
	case "mqtts":
		tlsEnabled = true
	default:
		return "", "", false, fmt.Errorf("unsupported mqtt scheme %q", parsed.Scheme)
	}
	host = parsed.Hostname()
	if host == "" {
		return "", "", false, errors.New("mqtt broker host is required")
	}
	port = parsed.Port()
	if port == "" {
		if tlsEnabled {
			port = "8883"
		} else {
			port = "1883"
		}
	}
	return host, port, tlsEnabled, nil
}

func writeConnectPacket(w io.Writer, cfg Config) error {
	flags := byte(0x02) // clean session
	payload := make([]byte, 0, 128)
	payload = append(payload, encodeString(cfg.ClientID)...)
	if cfg.Username != "" {
		flags |= 0x80
	}
	if cfg.Password != "" {
		flags |= 0x40
	}
	if cfg.Username != "" {
		payload = append(payload, encodeString(cfg.Username)...)
	}
	if cfg.Password != "" {
		payload = append(payload, encodeString(cfg.Password)...)
	}

	varHeader := make([]byte, 0, 10)
	varHeader = append(varHeader, encodeString("MQTT")...)
	varHeader = append(varHeader, 0x04, flags)
	keepAlive := make([]byte, 2)
	binary.BigEndian.PutUint16(keepAlive, defaultKeepAlive)
	varHeader = append(varHeader, keepAlive...)

	remaining := len(varHeader) + len(payload)
	packet := make([]byte, 0, 1+4+remaining)
	packet = append(packet, 0x10)
	packet = append(packet, encodeRemainingLength(remaining)...)
	packet = append(packet, varHeader...)
	packet = append(packet, payload...)

	if _, err := w.Write(packet); err != nil {
		return fmt.Errorf("write connect packet: %w", err)
	}
	return nil
}

func readConnAck(r io.Reader) error {
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return fmt.Errorf("read connack: %w", err)
	}
	if header[0] != 0x20 || header[1] != 0x02 {
		return fmt.Errorf("invalid connack header: %v", header[:2])
	}
	if header[3] != 0x00 {
		return fmt.Errorf("connack error code: %d", header[3])
	}
	return nil
}

func buildPublishPacket(topic string, payload []byte, qos byte, retained bool, packetID uint16) ([]byte, error) {
	if strings.TrimSpace(topic) == "" {
		return nil, errors.New("mqtt topic is required")
	}
	if qos > 1 {
		return nil, fmt.Errorf("unsupported qos %d", qos)
	}

	var variable []byte
	variable = append(variable, encodeString(topic)...)
	if qos > 0 {
		pid := make([]byte, 2)
		binary.BigEndian.PutUint16(pid, packetID)
		variable = append(variable, pid...)
	}

	remaining := len(variable) + len(payload)
	fixed := byte(0x30)
	fixed |= qos << 1
	if retained {
		fixed |= 0x01
	}

	packet := make([]byte, 0, 1+4+remaining)
	packet = append(packet, fixed)
	packet = append(packet, encodeRemainingLength(remaining)...)
	packet = append(packet, variable...)
	packet = append(packet, payload...)
	return packet, nil
}

func readPubAck(r io.Reader, expectedPacketID uint16) error {
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return fmt.Errorf("read puback: %w", err)
	}
	if header[0] != 0x40 || header[1] != 0x02 {
		return fmt.Errorf("invalid puback header: %v", header[:2])
	}
	packetID := binary.BigEndian.Uint16(header[2:4])
	if packetID != expectedPacketID {
		return fmt.Errorf("puback packet id mismatch: expected %d got %d", expectedPacketID, packetID)
	}
	return nil
}

func encodeString(value string) []byte {
	b := []byte(value)
	out := make([]byte, 2+len(b))
	binary.BigEndian.PutUint16(out[:2], uint16(len(b)))
	copy(out[2:], b)
	return out
}

func encodeRemainingLength(length int) []byte {
	encoded := make([]byte, 0, 4)
	for {
		digit := byte(length % 128)
		length /= 128
		if length > 0 {
			digit |= 0x80
		}
		encoded = append(encoded, digit)
		if length == 0 {
			break
		}
	}
	return encoded
}

func buildTopic(prefix string, cmd domain.ClockCommand) string {
	cleanPrefix := strings.Trim(strings.TrimSpace(prefix), "/")
	return fmt.Sprintf("%s/%s/%s", cleanPrefix, cmd.TargetDeviceID(), cmd.CommandType())
}

func buildPayload(cmd domain.ClockCommand) (map[string]any, error) {
	base := map[string]any{
		"deviceId": cmd.TargetDeviceID(),
		"type":     cmd.CommandType(),
	}

	switch c := cmd.(type) {
	case domain.SetAlarmCommand:
		base["alarmTime"] = c.AlarmTime.Format(time.RFC3339)
		base["label"] = c.Label
	case domain.DisplayMessageCommand:
		base["message"] = c.Message
		base["durationSeconds"] = c.DurationSeconds
	case domain.SetBrightnessCommand:
		base["level"] = c.Level
	default:
		return nil, fmt.Errorf("unsupported command type %T", cmd)
	}

	return base, nil
}
