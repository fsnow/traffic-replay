package reader

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// EventType represents the type of event in a packet
// Note: This is inferred from the message content, not stored in the binary format
type EventType uint8

const (
	// EventTypeRegular represents a regular message event with wire protocol data
	EventTypeRegular EventType = 0
	// EventTypeSessionStart represents a session start event (empty message, first packet for session)
	EventTypeSessionStart EventType = 1
	// EventTypeSessionEnd represents a session end event (empty message, last packet for session)
	EventTypeSessionEnd EventType = 2
)

// String returns a human-readable string for the event type
func (e EventType) String() string {
	switch e {
	case EventTypeRegular:
		return "Regular"
	case EventTypeSessionStart:
		return "SessionStart"
	case EventTypeSessionEnd:
		return "SessionEnd"
	default:
		return fmt.Sprintf("Unknown(%d)", e)
	}
}

// Packet represents a single packet from a MongoDB traffic recording file
//
// Binary Format (actual format from MongoDB server):
//   size          : uint32 LE (4 bytes)  - Total packet size including header
//   id            : uint64 LE (8 bytes)  - Session/connection identifier
//   session       : string   (variable)  - Null-terminated string with session metadata (JSON)
//   offset        : uint64 LE (8 bytes)  - Microseconds elapsed since recording started
//   order         : uint64 LE (8 bytes)  - Sequence number for ordering packets
//   message       : []byte   (variable)  - Wire protocol message (may be empty)
//
// Note: There is NO eventType field in the binary format. We infer EventType from context:
//   - Empty message -> SessionStart (if first for session) or SessionEnd (if last)
//   - Non-empty message -> EventTypeRegular
type Packet struct {
	// Size is the total packet size including header (bytes)
	Size uint32

	// EventType indicates the type of event (Regular, SessionStart, SessionEnd)
	// This is inferred, not read from the file
	EventType EventType

	// SessionID is the session/connection identifier
	SessionID uint64

	// SessionMetadata is the null-terminated string with session metadata (JSON format)
	// Example: "{ remote: \"127.0.0.1:51807\", local: \"127.0.0.1:28004\" }"
	SessionMetadata string

	// Offset is the time in microseconds since recording started
	Offset uint64

	// Order is the sequence number for ordering packets
	Order uint64

	// Message contains the raw wire protocol message bytes
	// Empty for SessionStart and SessionEnd events
	Message []byte
}

// IsRequest returns true if this packet is a request (not a response)
// This is determined by checking the wire protocol header's responseTo field
func (p *Packet) IsRequest() bool {
	if len(p.Message) < 16 {
		// Wire protocol header is at least 16 bytes
		// If message is shorter, treat as non-request
		return false
	}

	// Wire protocol header format (little-endian):
	//   messageLength : int32  (bytes 0-3)
	//   requestID     : int32  (bytes 4-7)
	//   responseTo    : int32  (bytes 8-11)
	//   opCode        : int32  (bytes 12-15)
	//
	// A request has responseTo == 0
	// A response has responseTo != 0 (references the request)

	responseTo := binary.LittleEndian.Uint32(p.Message[8:12])
	return responseTo == 0
}

// GetOpCode returns the OpCode from the wire protocol message
// Returns 0 if the message is too short or invalid
func (p *Packet) GetOpCode() uint32 {
	if len(p.Message) < 16 {
		return 0
	}

	// OpCode is at bytes 12-15 in the wire protocol header
	return binary.LittleEndian.Uint32(p.Message[12:16])
}

// ReadPacket reads a single packet from the provided reader
// Returns io.EOF when there are no more packets to read
func ReadPacket(r io.Reader) (*Packet, error) {
	packet := &Packet{}

	// Read size (4 bytes, little-endian)
	if err := binary.Read(r, binary.LittleEndian, &packet.Size); err != nil {
		return nil, err
	}

	// Sanity check: size should be at least the minimum header size
	// Minimum: 4 (size) + 8 (id) + 1 (null terminator for empty session) + 8 (offset) + 8 (order) = 29 bytes
	if packet.Size < 29 {
		return nil, fmt.Errorf("invalid packet size: %d (minimum 29 bytes)", packet.Size)
	}

	// Read session ID (8 bytes, little-endian)
	if err := binary.Read(r, binary.LittleEndian, &packet.SessionID); err != nil {
		return nil, fmt.Errorf("failed to read session ID: %w", err)
	}

	// Read session metadata (null-terminated string)
	sessionBytes := make([]byte, 0, 128) // Most session strings are short
	for {
		var b byte
		if err := binary.Read(r, binary.LittleEndian, &b); err != nil {
			return nil, fmt.Errorf("failed to read session metadata: %w", err)
		}
		if b == 0 {
			// Found null terminator
			break
		}
		sessionBytes = append(sessionBytes, b)

		// Sanity check: session metadata shouldn't be too long
		if len(sessionBytes) > 10000 {
			return nil, fmt.Errorf("session metadata too long (>10KB)")
		}
	}
	packet.SessionMetadata = string(sessionBytes)

	// Read offset (8 bytes, little-endian)
	if err := binary.Read(r, binary.LittleEndian, &packet.Offset); err != nil {
		return nil, fmt.Errorf("failed to read offset: %w", err)
	}

	// Read order (8 bytes, little-endian)
	if err := binary.Read(r, binary.LittleEndian, &packet.Order); err != nil {
		return nil, fmt.Errorf("failed to read order: %w", err)
	}

	// Calculate message size
	// Total size - header size
	// Header size = 4 (size) + 8 (id) + len(session) + 1 (null) + 8 (offset) + 8 (order)
	headerSize := 4 + 8 + len(sessionBytes) + 1 + 8 + 8
	messageSize := int(packet.Size) - headerSize

	if messageSize < 0 {
		return nil, fmt.Errorf("invalid message size: %d (total size: %d, header size: %d)", messageSize, packet.Size, headerSize)
	}

	// Read message data
	if messageSize > 0 {
		packet.Message = make([]byte, messageSize)
		if _, err := io.ReadFull(r, packet.Message); err != nil {
			return nil, fmt.Errorf("failed to read message data: %w", err)
		}
	}

	// Infer event type based on message content
	// For now, we'll treat all packets with messages as Regular
	// Empty messages could be session start/end, but we can't distinguish without tracking state
	if len(packet.Message) > 0 {
		packet.EventType = EventTypeRegular
	} else {
		// Empty message - could be session start or end
		// For simplicity, mark as Regular for now
		// TODO: Track session state to distinguish SessionStart from SessionEnd
		packet.EventType = EventTypeRegular
	}

	return packet, nil
}

// ReadPacketFromBytes is a convenience function that reads a packet from a byte slice
func ReadPacketFromBytes(data []byte) (*Packet, error) {
	return ReadPacket(bytes.NewReader(data))
}
