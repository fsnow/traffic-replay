package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fsnow/traffic-replay/pkg/reader"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <recording-file> [filter]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nShows detailed packet information.\n")
		fmt.Fprintf(os.Stderr, "\nFilters:\n")
		fmt.Fprintf(os.Stderr, "  all         - Show all packets (default)\n")
		fmt.Fprintf(os.Stderr, "  user        - Show only user operations (insert, find, update, delete, aggregate)\n")
		fmt.Fprintf(os.Stderr, "  command:X   - Show packets containing command X (e.g., command:insert)\n")
		fmt.Fprintf(os.Stderr, "  session:N   - Show packets for session N\n")
		os.Exit(1)
	}

	filePath := os.Args[1]
	filter := "all"
	if len(os.Args) >= 3 {
		filter = os.Args[2]
	}

	rec, err := reader.NewRecordingReader(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening recording: %v\n", err)
		os.Exit(1)
	}
	defer rec.Close()

	packetNum := 0
	shown := 0
	maxPackets := 50 // Limit output

	for {
		packet, err := rec.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading packet %d: %v\n", packetNum, err)
			os.Exit(1)
		}

		packetNum++

		if !shouldShow(packet, filter) {
			continue
		}

		shown++
		if shown > maxPackets {
			fmt.Printf("\n... (showing first %d matching packets, %d total packets in file)\n", maxPackets, packetNum)
			break
		}

		printPacket(packet, packetNum)
	}

	if shown == 0 {
		fmt.Printf("No packets matched filter: %s\n", filter)
	} else {
		fmt.Printf("\nShowed %d packets (out of %d total)\n", shown, packetNum)
	}
}

func shouldShow(packet *reader.Packet, filter string) bool {
	if filter == "all" {
		return true
	}

	if filter == "user" {
		cmd := extractCommandName(packet.Message)
		userCommands := map[string]bool{
			"insert": true, "update": true, "delete": true,
			"find": true, "aggregate": true, "distinct": true,
			"findAndModify": true, "count": true,
			"createIndexes": true, "dropIndexes": true,
			"listIndexes": true, "drop": true, "create": true,
		}
		return userCommands[cmd]
	}

	if strings.HasPrefix(filter, "command:") {
		cmd := strings.TrimPrefix(filter, "command:")
		return extractCommandName(packet.Message) == cmd
	}

	if strings.HasPrefix(filter, "session:") {
		var sessionID uint64
		fmt.Sscanf(filter, "session:%d", &sessionID)
		return packet.SessionID == sessionID
	}

	return false
}

func printPacket(packet *reader.Packet, num int) {
	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("PACKET #%d\n", num)
	fmt.Println(strings.Repeat("=", 100))

	fmt.Printf("Size:             %d bytes\n", packet.Size)
	fmt.Printf("Session ID:       %d\n", packet.SessionID)
	fmt.Printf("Session Metadata: %s\n", packet.SessionMetadata)
	fmt.Printf("Offset:           %d μs (%.3f ms from start)\n", packet.Offset, float64(packet.Offset)/1000.0)
	fmt.Printf("Order:            %d\n", packet.Order)
	fmt.Printf("Message Length:   %d bytes\n", len(packet.Message))

	if len(packet.Message) == 0 {
		fmt.Println("(Empty message)")
		fmt.Println()
		return
	}

	// Parse wire protocol header
	if len(packet.Message) < 16 {
		fmt.Println("(Message too short for wire protocol header)")
		fmt.Println()
		return
	}

	msgLen := binary.LittleEndian.Uint32(packet.Message[0:4])
	requestID := binary.LittleEndian.Uint32(packet.Message[4:8])
	responseTo := binary.LittleEndian.Uint32(packet.Message[8:12])
	opCode := binary.LittleEndian.Uint32(packet.Message[12:16])

	fmt.Println("\n--- Wire Protocol Header ---")
	fmt.Printf("Message Length:   %d\n", msgLen)
	fmt.Printf("Request ID:       %d\n", requestID)
	fmt.Printf("Response To:      %d", responseTo)
	if responseTo == 0 {
		fmt.Printf(" (REQUEST)\n")
	} else {
		fmt.Printf(" (RESPONSE to request %d)\n", responseTo)
	}
	fmt.Printf("OpCode:           %d (%s)\n", opCode, getOpCodeName(opCode))

	// Parse message body based on opcode
	if opCode == 2013 {
		// OP_MSG
		parseOpMsg(packet.Message[16:])
	} else if opCode == 2012 {
		// OP_COMPRESSED
		fmt.Println("\n--- Compressed Message ---")
		if len(packet.Message) >= 25 {
			originalOpCode := binary.LittleEndian.Uint32(packet.Message[16:20])
			uncompressedSize := binary.LittleEndian.Uint32(packet.Message[20:24])
			compressorID := packet.Message[24]
			fmt.Printf("Original OpCode:     %d (%s)\n", originalOpCode, getOpCodeName(originalOpCode))
			fmt.Printf("Uncompressed Size:   %d bytes\n", uncompressedSize)
			fmt.Printf("Compressor ID:       %d (%s)\n", compressorID, getCompressorName(compressorID))
			fmt.Printf("Compressed Data:     %d bytes\n", len(packet.Message)-25)
		}
	} else if opCode == 2004 {
		// OP_QUERY (legacy)
		fmt.Println("\n--- OP_QUERY (Legacy) ---")
		fmt.Println("(Legacy opcode - details not parsed)")
	} else if opCode == 1 {
		// OP_REPLY (legacy)
		fmt.Println("\n--- OP_REPLY (Legacy) ---")
		if len(packet.Message) >= 36 {
			responseFlags := binary.LittleEndian.Uint32(packet.Message[16:20])
			cursorID := binary.LittleEndian.Uint64(packet.Message[20:28])
			startingFrom := binary.LittleEndian.Uint32(packet.Message[28:32])
			numberReturned := binary.LittleEndian.Uint32(packet.Message[32:36])
			fmt.Printf("Response Flags:   0x%08x\n", responseFlags)
			fmt.Printf("Cursor ID:        %d\n", cursorID)
			fmt.Printf("Starting From:    %d\n", startingFrom)
			fmt.Printf("Number Returned:  %d\n", numberReturned)
		}
	}

	// Show hex dump of first part of message
	fmt.Println("\n--- Message Hex Dump (first 128 bytes) ---")
	dumpLen := len(packet.Message)
	if dumpLen > 128 {
		dumpLen = 128
	}
	fmt.Println(hex.Dump(packet.Message[:dumpLen]))

	fmt.Println()
}

func parseOpMsg(body []byte) {
	if len(body) < 5 {
		fmt.Println("\n--- OP_MSG Body ---")
		fmt.Println("(Too short)")
		return
	}

	flags := binary.LittleEndian.Uint32(body[0:4])
	fmt.Println("\n--- OP_MSG Body ---")
	fmt.Printf("Flags:            0x%08x", flags)
	if flags == 0 {
		fmt.Printf(" (none)")
	}
	if flags&(1<<0) != 0 {
		fmt.Printf(" [checksumPresent]")
	}
	if flags&(1<<1) != 0 {
		fmt.Printf(" [moreToCome]")
	}
	if flags&(1<<16) != 0 {
		fmt.Printf(" [exhaustAllowed]")
	}
	fmt.Println()

	// Parse sections
	offset := 4
	sectionNum := 0
	for offset < len(body) {
		if offset >= len(body) {
			break
		}

		kind := body[offset]
		offset++

		fmt.Printf("\nSection %d (kind %d):", sectionNum, kind)
		if kind == 0 {
			fmt.Println(" Body (BSON document)")
			// Try to extract some info from BSON
			if offset+4 < len(body) {
				bsonSize := binary.LittleEndian.Uint32(body[offset : offset+4])
				fmt.Printf("  BSON Size: %d bytes\n", bsonSize)

				// Try to extract command name
				if offset+5 < len(body) {
					elementType := body[offset+4]
					nameStart := offset + 5
					nameEnd := nameStart
					for nameEnd < len(body) && body[nameEnd] != 0 {
						nameEnd++
					}
					if nameEnd < len(body) {
						elementName := string(body[nameStart:nameEnd])
						fmt.Printf("  First field: %s (type %d)\n", elementName, elementType)
					}
				}

				// Show raw BSON hex
				bsonEnd := offset + int(bsonSize)
				if bsonEnd > len(body) {
					bsonEnd = len(body)
				}
				fmt.Println("  BSON hex:")
				dumpLen := int(bsonSize)
				if dumpLen > 256 {
					dumpLen = 256
					fmt.Printf("  (showing first 256 of %d bytes)\n", bsonSize)
				}
				if offset+dumpLen <= len(body) {
					fmt.Print(indent(hex.Dump(body[offset:offset+dumpLen]), "    "))
				}

				offset = bsonEnd
			}
		} else if kind == 1 {
			fmt.Println(" Document Sequence")
			if offset+4 < len(body) {
				seqSize := binary.LittleEndian.Uint32(body[offset : offset+4])
				fmt.Printf("  Sequence Size: %d bytes\n", seqSize)
				offset += int(seqSize)
			}
		} else {
			fmt.Printf(" (Unknown kind)\n")
			break
		}

		sectionNum++
		if sectionNum > 10 {
			fmt.Println("... (too many sections)")
			break
		}
	}
}

func extractCommandName(message []byte) string {
	if len(message) < 21 {
		return ""
	}

	opCode := binary.LittleEndian.Uint32(message[12:16])
	if opCode != 2013 {
		return ""
	}

	offset := 20 // Skip header (16) + flags (4)
	if message[offset] != 0 {
		return ""
	}
	offset++ // Skip section kind

	if offset+5 > len(message) {
		return ""
	}

	offset += 4 // Skip BSON size
	offset++    // Skip element type

	nameStart := offset
	for offset < len(message) && message[offset] != 0 {
		offset++
	}

	if offset >= len(message) {
		return ""
	}

	return string(message[nameStart:offset])
}

func getOpCodeName(code uint32) string {
	switch code {
	case 1:
		return "OP_REPLY"
	case 2001:
		return "OP_UPDATE"
	case 2002:
		return "OP_INSERT"
	case 2004:
		return "OP_QUERY"
	case 2005:
		return "OP_GET_MORE"
	case 2006:
		return "OP_DELETE"
	case 2007:
		return "OP_KILL_CURSORS"
	case 2012:
		return "OP_COMPRESSED"
	case 2013:
		return "OP_MSG"
	default:
		return "UNKNOWN"
	}
}

func getCompressorName(id byte) string {
	switch id {
	case 0:
		return "noop"
	case 1:
		return "snappy"
	case 2:
		return "zlib"
	case 3:
		return "zstd"
	default:
		return "unknown"
	}
}

func indent(s string, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}
