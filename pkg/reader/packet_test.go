package reader

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

// buildTestPacket creates a test packet with the given parameters
// Note: eventType is ignored since it's not in the binary format
func buildTestPacket(eventType EventType, sessionID uint64, sessionMetadata string, offset uint64, order uint64, message []byte) []byte {
	buf := new(bytes.Buffer)

	// Calculate total size
	// Header size = 4 (size) + 8 (id) + len(session) + 1 (null) + 8 (offset) + 8 (order)
	headerSize := 4 + 8 + len(sessionMetadata) + 1 + 8 + 8
	totalSize := uint32(headerSize + len(message))

	// Write size
	binary.Write(buf, binary.LittleEndian, totalSize)

	// Write sessionID (no eventType field!)
	binary.Write(buf, binary.LittleEndian, sessionID)

	// Write session metadata (null-terminated)
	buf.WriteString(sessionMetadata)
	buf.WriteByte(0)

	// Write offset
	binary.Write(buf, binary.LittleEndian, offset)

	// Write order
	binary.Write(buf, binary.LittleEndian, order)

	// Write message
	buf.Write(message)

	return buf.Bytes()
}

// buildWireMessage creates a minimal wire protocol message for testing
func buildWireMessage(messageLength int32, requestID int32, responseTo int32, opCode int32) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, messageLength)
	binary.Write(buf, binary.LittleEndian, requestID)
	binary.Write(buf, binary.LittleEndian, responseTo)
	binary.Write(buf, binary.LittleEndian, opCode)
	return buf.Bytes()
}

func TestEventTypeString(t *testing.T) {
	tests := []struct {
		eventType EventType
		expected  string
	}{
		{EventTypeRegular, "Regular"},
		{EventTypeSessionStart, "SessionStart"},
		{EventTypeSessionEnd, "SessionEnd"},
		{EventType(99), "Unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.eventType.String()
			if result != tt.expected {
				t.Errorf("EventType.String() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestReadPacket_EmptyMessage(t *testing.T) {
	// Create a packet with no message data (could be session start/end)
	data := buildTestPacket(EventTypeRegular, 12345, "", 1000000, 1, nil)

	packet, err := ReadPacketFromBytes(data)
	if err != nil {
		t.Fatalf("ReadPacket failed: %v", err)
	}

	// Note: EventType is inferred and will be Regular for empty messages
	if packet.EventType != EventTypeRegular {
		t.Errorf("EventType = %v, want %v", packet.EventType, EventTypeRegular)
	}
	if packet.SessionID != 12345 {
		t.Errorf("SessionID = %v, want %v", packet.SessionID, 12345)
	}
	if packet.Offset != 1000000 {
		t.Errorf("Offset = %v, want %v", packet.Offset, 1000000)
	}
	if packet.Order != 1 {
		t.Errorf("Order = %v, want %v", packet.Order, 1)
	}
	if len(packet.Message) != 0 {
		t.Errorf("Message length = %v, want 0", len(packet.Message))
	}
}

func TestReadPacket_RegularMessage(t *testing.T) {
	// Create a wire protocol message (request)
	wireMsg := buildWireMessage(16, 100, 0, 2013) // OP_MSG = 2013, responseTo=0 means it's a request

	// Create a regular packet with the wire message
	data := buildTestPacket(EventTypeRegular, 67890, "test-session", 2000000, 2, wireMsg)

	packet, err := ReadPacketFromBytes(data)
	if err != nil {
		t.Fatalf("ReadPacket failed: %v", err)
	}

	if packet.EventType != EventTypeRegular {
		t.Errorf("EventType = %v, want %v", packet.EventType, EventTypeRegular)
	}
	if packet.SessionID != 67890 {
		t.Errorf("SessionID = %v, want %v", packet.SessionID, 67890)
	}
	if packet.SessionMetadata != "test-session" {
		t.Errorf("SessionMetadata = %v, want %v", packet.SessionMetadata, "test-session")
	}
	if packet.Offset != 2000000 {
		t.Errorf("Offset = %v, want %v", packet.Offset, 2000000)
	}
	if packet.Order != 2 {
		t.Errorf("Order = %v, want %v", packet.Order, 2)
	}
	if len(packet.Message) != len(wireMsg) {
		t.Errorf("Message length = %v, want %v", len(packet.Message), len(wireMsg))
	}
}

func TestReadPacket_IsRequest(t *testing.T) {
	tests := []struct {
		name       string
		responseTo int32
		expected   bool
	}{
		{"Request", 0, true},
		{"Response", 100, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wireMsg := buildWireMessage(16, 100, tt.responseTo, 2013)
			data := buildTestPacket(EventTypeRegular, 1, "", 1000, 1, wireMsg)

			packet, err := ReadPacketFromBytes(data)
			if err != nil {
				t.Fatalf("ReadPacket failed: %v", err)
			}

			if packet.IsRequest() != tt.expected {
				t.Errorf("IsRequest() = %v, want %v", packet.IsRequest(), tt.expected)
			}
		})
	}
}

func TestReadPacket_GetOpCode(t *testing.T) {
	wireMsg := buildWireMessage(16, 100, 0, 2013) // OP_MSG = 2013
	data := buildTestPacket(EventTypeRegular, 1, "", 1000, 1, wireMsg)

	packet, err := ReadPacketFromBytes(data)
	if err != nil {
		t.Fatalf("ReadPacket failed: %v", err)
	}

	opCode := packet.GetOpCode()
	if opCode != 2013 {
		t.Errorf("GetOpCode() = %v, want 2013", opCode)
	}
}

func TestReadPacket_InvalidSize(t *testing.T) {
	// Create a packet with size too small (minimum is 29 bytes)
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, uint32(10)) // Size = 10 (too small)

	_, err := ReadPacketFromBytes(buf.Bytes())
	if err == nil {
		t.Error("Expected error for invalid packet size, got nil")
	}
}

func TestReadPacket_EOF(t *testing.T) {
	// Try to read from empty buffer
	_, err := ReadPacketFromBytes([]byte{})
	if err != io.EOF {
		t.Errorf("Expected io.EOF, got %v", err)
	}
}

func TestReadPacket_MultiplePackets(t *testing.T) {
	// Create multiple packets in sequence
	packet1 := buildTestPacket(EventTypeRegular, 1, "", 1000, 1, nil)
	wireMsg := buildWireMessage(16, 100, 0, 2013)
	packet2 := buildTestPacket(EventTypeRegular, 1, "", 2000, 2, wireMsg)
	packet3 := buildTestPacket(EventTypeRegular, 1, "", 3000, 3, nil)

	// Concatenate all packets
	allData := append(packet1, packet2...)
	allData = append(allData, packet3...)

	reader := bytes.NewReader(allData)

	// Read first packet (empty message)
	p1, err := ReadPacket(reader)
	if err != nil {
		t.Fatalf("Failed to read packet 1: %v", err)
	}
	if p1.EventType != EventTypeRegular {
		t.Errorf("Packet 1: EventType = %v, want %v", p1.EventType, EventTypeRegular)
	}
	if len(p1.Message) != 0 {
		t.Errorf("Packet 1: Message length = %v, want 0", len(p1.Message))
	}

	// Read second packet (with message)
	p2, err := ReadPacket(reader)
	if err != nil {
		t.Fatalf("Failed to read packet 2: %v", err)
	}
	if p2.EventType != EventTypeRegular {
		t.Errorf("Packet 2: EventType = %v, want %v", p2.EventType, EventTypeRegular)
	}
	if len(p2.Message) == 0 {
		t.Error("Packet 2: Expected non-empty message")
	}

	// Read third packet (empty message)
	p3, err := ReadPacket(reader)
	if err != nil {
		t.Fatalf("Failed to read packet 3: %v", err)
	}
	if p3.EventType != EventTypeRegular {
		t.Errorf("Packet 3: EventType = %v, want %v", p3.EventType, EventTypeRegular)
	}
	if len(p3.Message) != 0 {
		t.Errorf("Packet 3: Message length = %v, want 0", len(p3.Message))
	}

	// Should get EOF on next read
	_, err = ReadPacket(reader)
	if err != io.EOF {
		t.Errorf("Expected io.EOF after all packets, got %v", err)
	}
}
