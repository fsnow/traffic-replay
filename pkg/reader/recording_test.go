package reader

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordingReader_Basic(t *testing.T) {
	// Create a temporary recording file for testing
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.bin")

	// Write test packets
	packet1 := buildTestPacket(EventTypeSessionStart, 1, "", 1000, 1, nil)
	wireMsg := buildWireMessage(16, 100, 0, 2013)
	packet2 := buildTestPacket(EventTypeRegular, 1, "", 2000, 2, wireMsg)
	packet3 := buildTestPacket(EventTypeSessionEnd, 1, "", 3000, 3, nil)

	allData := append(packet1, packet2...)
	allData = append(allData, packet3...)

	if err := os.WriteFile(tmpFile, allData, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Open recording reader
	reader, err := NewRecordingReader(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create RecordingReader: %v", err)
	}
	defer reader.Close()

	// Read packets
	packets := []*Packet{}
	for {
		packet, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read packet: %v", err)
		}
		packets = append(packets, packet)
	}

	// Verify
	if len(packets) != 3 {
		t.Fatalf("Expected 3 packets, got %d", len(packets))
	}

	// All packets will have EventTypeRegular (we don't track session state yet)
	for i, p := range packets {
		if p.EventType != EventTypeRegular {
			t.Errorf("Packet %d: EventType = %v, want %v", i, p.EventType, EventTypeRegular)
		}
	}
}

func TestRecordingSet_MultipleFiles(t *testing.T) {
	// Create a temporary directory with multiple recording files
	tmpDir := t.TempDir()

	// Create file1.bin
	packet1 := buildTestPacket(EventTypeSessionStart, 1, "", 1000, 1, nil)
	wireMsg1 := buildWireMessage(16, 100, 0, 2013)
	packet2 := buildTestPacket(EventTypeRegular, 1, "", 2000, 2, wireMsg1)
	file1Data := append(packet1, packet2...)
	if err := os.WriteFile(filepath.Join(tmpDir, "file1.bin"), file1Data, 0644); err != nil {
		t.Fatalf("Failed to write file1.bin: %v", err)
	}

	// Create file2.bin
	wireMsg2 := buildWireMessage(16, 101, 0, 2013)
	packet3 := buildTestPacket(EventTypeRegular, 1, "", 3000, 3, wireMsg2)
	packet4 := buildTestPacket(EventTypeSessionEnd, 1, "", 4000, 4, nil)
	file2Data := append(packet3, packet4...)
	if err := os.WriteFile(filepath.Join(tmpDir, "file2.bin"), file2Data, 0644); err != nil {
		t.Fatalf("Failed to write file2.bin: %v", err)
	}

	// Open recording set
	rs, err := NewRecordingSet(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create RecordingSet: %v", err)
	}
	defer rs.Close()

	// Verify file count
	if rs.FileCount() != 2 {
		t.Errorf("FileCount = %v, want 2", rs.FileCount())
	}

	// Read all packets
	packets := []*Packet{}
	for {
		packet, err := rs.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read packet: %v", err)
		}
		packets = append(packets, packet)
	}

	// Verify total packet count
	if len(packets) != 4 {
		t.Fatalf("Expected 4 packets total, got %d", len(packets))
	}

	// All packets will have EventTypeRegular (we don't track session state yet)
	for i, p := range packets {
		if p.EventType != EventTypeRegular {
			t.Errorf("Packet %d: EventType = %v, want %v", i, p.EventType, EventTypeRegular)
		}
	}

	// Verify order field is sequential
	for i := 0; i < len(packets); i++ {
		if packets[i].Order != uint64(i+1) {
			t.Errorf("Packet %d: Order = %v, want %v", i, packets[i].Order, i+1)
		}
	}
}

func TestRecordingSet_NoFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Try to open empty directory (no .bin files)
	_, err := NewRecordingSet(tmpDir)
	if err == nil {
		t.Error("Expected error for directory with no .bin files, got nil")
	}
}

func TestRecordingSet_NotADirectory(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "notadir.txt")
	os.WriteFile(tmpFile, []byte("test"), 0644)

	// Try to open a file instead of a directory
	_, err := NewRecordingSet(tmpFile)
	if err == nil {
		t.Error("Expected error when path is not a directory, got nil")
	}
}
