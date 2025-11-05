package reader

import (
	"io"
	"path/filepath"
	"testing"
)

// TestRealRecording tests reading from the actual recording file created yesterday
// This recording was created by sending various operations to a local MongoDB 8.0 instance
func TestRealRecording(t *testing.T) {
	// Path to the real recording file
	recordingPath := filepath.Join("..", "..", "recordings", "recording1.txt")

	reader, err := NewRecordingReader(recordingPath)
	if err != nil {
		t.Skipf("Skipping real recording test: %v", err)
		return
	}
	defer reader.Close()

	stats := struct {
		totalPackets   int
		sessionStarts  int
		sessionEnds    int
		regularPackets int
		requests       int
		responses      int
		opCodes        map[uint32]int
	}{
		opCodes: make(map[uint32]int),
	}

	for {
		packet, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read packet %d: %v", stats.totalPackets, err)
		}

		stats.totalPackets++

		switch packet.EventType {
		case EventTypeSessionStart:
			stats.sessionStarts++
		case EventTypeSessionEnd:
			stats.sessionEnds++
		case EventTypeRegular:
			stats.regularPackets++
			if packet.IsRequest() {
				stats.requests++
			} else {
				stats.responses++
			}

			if opCode := packet.GetOpCode(); opCode != 0 {
				stats.opCodes[opCode]++
			}
		}
	}

	// Log statistics
	t.Logf("Recording Statistics:")
	t.Logf("  Total packets: %d", stats.totalPackets)
	t.Logf("  Session starts: %d", stats.sessionStarts)
	t.Logf("  Session ends: %d", stats.sessionEnds)
	t.Logf("  Regular packets: %d", stats.regularPackets)
	t.Logf("  Requests: %d", stats.requests)
	t.Logf("  Responses: %d", stats.responses)
	t.Logf("  OpCode distribution:")
	for opCode, count := range stats.opCodes {
		opName := "UNKNOWN"
		switch opCode {
		case 2013:
			opName = "OP_MSG"
		case 2012:
			opName = "OP_COMPRESSED"
		}
		t.Logf("    %s (%d): %d packets", opName, opCode, count)
	}

	// Basic sanity checks
	if stats.totalPackets == 0 {
		t.Fatal("Expected at least one packet in recording")
	}

	if stats.regularPackets == 0 {
		t.Fatal("Expected at least one regular packet")
	}

	// Check for modern opcodes (OP_MSG=2013, OP_COMPRESSED=2012)
	modernOpcodes := 0
	legacyOpcodes := 0
	for opCode, count := range stats.opCodes {
		if opCode == 2013 || opCode == 2012 {
			modernOpcodes += count
		} else {
			legacyOpcodes += count
			t.Logf("  Found legacy OpCode %d: %d packets", opCode, count)
		}
	}

	if modernOpcodes == 0 {
		t.Error("Expected at least some modern opcodes (OP_MSG or OP_COMPRESSED)")
	}

	if legacyOpcodes > 0 {
		t.Logf("  Note: Recording contains %d packets with legacy opcodes", legacyOpcodes)
	}

	t.Logf("✓ Successfully read and validated %d packets from recording", stats.totalPackets)
}

// TestRealRecording_FirstPacket examines the first packet in detail
func TestRealRecording_FirstPacket(t *testing.T) {
	recordingPath := filepath.Join("..", "..", "recordings", "recording1.txt")

	reader, err := NewRecordingReader(recordingPath)
	if err != nil {
		t.Skipf("Skipping real recording test: %v", err)
		return
	}
	defer reader.Close()

	packet, err := reader.Next()
	if err != nil {
		t.Fatalf("Failed to read first packet: %v", err)
	}

	t.Logf("First Packet Details:")
	t.Logf("  Size: %d bytes", packet.Size)
	t.Logf("  Event Type: %s", packet.EventType)
	t.Logf("  Session ID: %d", packet.SessionID)
	t.Logf("  Session Metadata: %q", packet.SessionMetadata)
	t.Logf("  Offset: %d microseconds", packet.Offset)
	t.Logf("  Order: %d", packet.Order)
	t.Logf("  Message Length: %d bytes", len(packet.Message))

	if packet.EventType == EventTypeRegular && len(packet.Message) >= 16 {
		t.Logf("  Is Request: %v", packet.IsRequest())
		opCode := packet.GetOpCode()
		opName := "UNKNOWN"
		if opCode == 2013 {
			opName = "OP_MSG"
		} else if opCode == 2012 {
			opName = "OP_COMPRESSED"
		}
		t.Logf("  OpCode: %d (%s)", opCode, opName)
	}
}
