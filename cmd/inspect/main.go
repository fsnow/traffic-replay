package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

// Simple tool to inspect the raw format of a recording file
func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <recording-file>\n", os.Args[0])
		os.Exit(1)
	}

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("File size: %d bytes\n\n", len(data))

	// Read first packet header
	if len(data) < 20 {
		fmt.Fprintf(os.Stderr, "File too small\n")
		os.Exit(1)
	}

	fmt.Println("=== First Packet Analysis ===")
	fmt.Printf("Hex dump (first 128 bytes):\n")
	for i := 0; i < 128 && i < len(data); i += 16 {
		fmt.Printf("%08x  ", i)
		for j := 0; j < 16 && i+j < len(data); j++ {
			fmt.Printf("%02x ", data[i+j])
			if j == 7 {
				fmt.Printf(" ")
			}
		}
		fmt.Printf(" |")
		for j := 0; j < 16 && i+j < len(data); j++ {
			c := data[i+j]
			if c >= 32 && c < 127 {
				fmt.Printf("%c", c)
			} else {
				fmt.Printf(".")
			}
		}
		fmt.Printf("|\n")
	}

	fmt.Println("\n=== Format Interpretation ===")

	// Try interpretation 1: size + eventType (1 byte) + id + session
	size1 := binary.LittleEndian.Uint32(data[0:4])
	eventType1 := data[4]
	id1 := binary.LittleEndian.Uint64(data[5:13])
	fmt.Printf("Interpretation 1 (design doc format):\n")
	fmt.Printf("  size: %d\n", size1)
	fmt.Printf("  eventType: %d (should be 0, 1, or 2)\n", eventType1)
	fmt.Printf("  id: %d\n", id1)
	fmt.Printf("  session starts at byte 13: '%s...'\n", string(data[13:30]))

	// Try interpretation 2: size + id + session (no eventType)
	size2 := binary.LittleEndian.Uint32(data[0:4])
	id2 := binary.LittleEndian.Uint64(data[4:12])
	fmt.Printf("\nInterpretation 2 (no eventType field):\n")
	fmt.Printf("  size: %d\n", size2)
	fmt.Printf("  id: %d\n", id2)
	fmt.Printf("  session starts at byte 12: '%s...'\n", string(data[12:30]))

	// Find null terminator for session string
	nullIdx := -1
	for i := 12; i < len(data) && i < 200; i++ {
		if data[i] == 0 {
			nullIdx = i
			break
		}
	}

	if nullIdx > 0 {
		sessionStr := string(data[12:nullIdx])
		fmt.Printf("\nSession string (interpretation 2): %q\n", sessionStr)
		fmt.Printf("Session string ends at byte %d\n", nullIdx)

		if nullIdx+16 < len(data) {
			offset := binary.LittleEndian.Uint64(data[nullIdx+1 : nullIdx+9])
			order := binary.LittleEndian.Uint64(data[nullIdx+9 : nullIdx+17])
			fmt.Printf("Offset (at byte %d): %d microseconds\n", nullIdx+1, offset)
			fmt.Printf("Order (at byte %d): %d\n", nullIdx+9, order)

			messageStart := nullIdx + 17
			messageSize := int(size2) - (messageStart - 0)
			fmt.Printf("Message starts at byte %d, size: %d bytes\n", messageStart, messageSize)

			if messageStart+16 <= len(data) {
				// Try to read wire protocol header
				msgLen := binary.LittleEndian.Uint32(data[messageStart:messageStart+4])
				requestID := binary.LittleEndian.Uint32(data[messageStart+4:messageStart+8])
				responseTo := binary.LittleEndian.Uint32(data[messageStart+8:messageStart+12])
				opCode := binary.LittleEndian.Uint32(data[messageStart+12:messageStart+16])

				fmt.Printf("\nWire protocol header:\n")
				fmt.Printf("  messageLength: %d\n", msgLen)
				fmt.Printf("  requestID: %d\n", requestID)
				fmt.Printf("  responseTo: %d (0=request, else response)\n", responseTo)
				fmt.Printf("  opCode: %d (2013=OP_MSG, 2012=OP_COMPRESSED)\n", opCode)
			}
		}
	}
}
