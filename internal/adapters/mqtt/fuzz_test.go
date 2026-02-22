package mqtt

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func FuzzBuildPublishPacket(f *testing.F) {
	f.Add("topic/a", []byte("hello"), byte(0), false, uint16(1))
	f.Add("topic/b", []byte{}, byte(1), true, uint16(7))
	f.Fuzz(func(t *testing.T, topic string, payload []byte, qos byte, retained bool, packetID uint16) {
		packet, err := buildPublishPacket(topic, payload, qos, retained, packetID)
		if err != nil {
			return
		}
		if len(packet) < 4 {
			t.Fatalf("packet too short: %d", len(packet))
		}
	})
}

func FuzzReadPubAck(f *testing.F) {
	buf := make([]byte, 4)
	buf[0] = 0x40
	buf[1] = 0x02
	binary.BigEndian.PutUint16(buf[2:], 10)
	f.Add(buf, uint16(10))
	f.Fuzz(func(t *testing.T, b []byte, expected uint16) {
		if len(b) < 4 {
			return
		}
		_ = readPubAck(bytes.NewReader(b[:4]), expected)
	})
}
