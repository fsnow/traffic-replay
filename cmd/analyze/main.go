package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/fsnow/traffic-replay/pkg/reader"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <recording-file>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nAnalyzes a MongoDB traffic recording file and provides detailed statistics.\n")
		os.Exit(1)
	}

	filePath := os.Args[1]

	fmt.Printf("Analyzing recording: %s\n", filePath)
	fmt.Println(strings.Repeat("=", 80))

	// Open recording
	rec, err := reader.NewRecordingReader(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening recording: %v\n", err)
		os.Exit(1)
	}
	defer rec.Close()

	// Collect statistics
	stats := &Statistics{
		sessions:       make(map[uint64]*SessionStats),
		opCodes:        make(map[uint32]int),
		commandCounts:  make(map[string]int),
	}

	packetNum := 0
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
		stats.analyze(packet)
	}

	// Print results
	stats.print()
}

type Statistics struct {
	totalPackets   int
	totalBytes     uint64
	requests       int
	responses      int
	emptyMessages  int

	sessions       map[uint64]*SessionStats
	opCodes        map[uint32]int
	commandCounts  map[string]int

	firstOffset    uint64
	lastOffset     uint64
}

type SessionStats struct {
	sessionID    uint64
	metadata     string
	packetCount  int
	requestCount int
	responseCount int
	bytes        uint64
	firstSeen    uint64
	lastSeen     uint64
}

func (s *Statistics) analyze(packet *reader.Packet) {
	s.totalPackets++
	s.totalBytes += uint64(packet.Size)

	// Track offsets for timing analysis
	if s.totalPackets == 1 {
		s.firstOffset = packet.Offset
	}
	s.lastOffset = packet.Offset

	// Track by session
	session, exists := s.sessions[packet.SessionID]
	if !exists {
		session = &SessionStats{
			sessionID: packet.SessionID,
			metadata:  packet.SessionMetadata,
			firstSeen: packet.Offset,
		}
		s.sessions[packet.SessionID] = session
	}
	session.packetCount++
	session.lastSeen = packet.Offset
	session.bytes += uint64(packet.Size)

	// Analyze message
	if len(packet.Message) == 0 {
		s.emptyMessages++
		return
	}

	// Request vs response
	if packet.IsRequest() {
		s.requests++
		session.requestCount++
	} else {
		s.responses++
		session.responseCount++
	}

	// OpCode
	opCode := packet.GetOpCode()
	s.opCodes[opCode]++

	// Try to extract command name from OP_MSG messages
	if opCode == 2013 && len(packet.Message) > 20 {
		if cmdName := extractCommandName(packet.Message); cmdName != "" {
			s.commandCounts[cmdName]++
		}
	}
}

func (s *Statistics) print() {
	fmt.Println("\n=== OVERALL STATISTICS ===")
	fmt.Printf("Total packets:    %d\n", s.totalPackets)
	fmt.Printf("Total bytes:      %s\n", formatBytes(s.totalBytes))
	fmt.Printf("Requests:         %d\n", s.requests)
	fmt.Printf("Responses:        %d\n", s.responses)
	fmt.Printf("Empty messages:   %d\n", s.emptyMessages)

	duration := time.Duration(s.lastOffset-s.firstOffset) * time.Microsecond
	fmt.Printf("\nRecording duration: %v\n", duration)
	fmt.Printf("First packet offset: %d μs\n", s.firstOffset)
	fmt.Printf("Last packet offset:  %d μs\n", s.lastOffset)

	fmt.Println("\n=== OPCODE DISTRIBUTION ===")
	printOpCodeStats(s.opCodes)

	fmt.Println("\n=== COMMAND DISTRIBUTION (OP_MSG only) ===")
	printCommandStats(s.commandCounts)

	fmt.Println("\n=== SESSION STATISTICS ===")
	fmt.Printf("Total sessions: %d\n", len(s.sessions))
	printSessionStats(s.sessions)
}

func printOpCodeStats(opCodes map[uint32]int) {
	type opStat struct {
		code  uint32
		name  string
		count int
	}

	var stats []opStat
	total := 0
	for code, count := range opCodes {
		name := getOpCodeName(code)
		stats = append(stats, opStat{code, name, count})
		total += count
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].count > stats[j].count
	})

	for _, stat := range stats {
		pct := float64(stat.count) / float64(total) * 100
		fmt.Printf("  %-25s (%4d): %6d packets (%5.1f%%)\n",
			stat.name, stat.code, stat.count, pct)
	}
}

func printCommandStats(commands map[string]int) {
	if len(commands) == 0 {
		fmt.Println("  (No commands extracted)")
		return
	}

	type cmdStat struct {
		name  string
		count int
	}

	var stats []cmdStat
	total := 0
	for name, count := range commands {
		stats = append(stats, cmdStat{name, count})
		total += count
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].count > stats[j].count
	})

	for _, stat := range stats {
		pct := float64(stat.count) / float64(total) * 100
		fmt.Printf("  %-30s: %6d (%5.1f%%)\n", stat.name, stat.count, pct)
	}
}

func printSessionStats(sessions map[uint64]*SessionStats) {
	type sessStat struct {
		id       uint64
		metadata string
		packets  int
		requests int
		responses int
		bytes    uint64
		duration time.Duration
	}

	var stats []sessStat
	for _, s := range sessions {
		duration := time.Duration(s.lastSeen-s.firstSeen) * time.Microsecond
		stats = append(stats, sessStat{
			id:        s.sessionID,
			metadata:  s.metadata,
			packets:   s.packetCount,
			requests:  s.requestCount,
			responses: s.responseCount,
			bytes:     s.bytes,
			duration:  duration,
		})
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].packets > stats[j].packets
	})

	fmt.Println()
	fmt.Printf("%-10s %-45s %8s %8s %8s %10s\n",
		"Session", "Metadata", "Packets", "Req", "Resp", "Bytes")
	fmt.Println(strings.Repeat("-", 100))

	for i, s := range stats {
		if i >= 20 {
			fmt.Printf("\n... and %d more sessions\n", len(stats)-20)
			break
		}

		metadata := s.metadata
		if len(metadata) > 43 {
			metadata = metadata[:40] + "..."
		}

		fmt.Printf("%-10d %-45s %8d %8d %8d %10s\n",
			s.id, metadata, s.packets, s.requests, s.responses, formatBytes(s.bytes))
	}
}

func getOpCodeName(code uint32) string {
	switch code {
	case 1:
		return "OP_REPLY (legacy)"
	case 2001:
		return "OP_UPDATE (legacy)"
	case 2002:
		return "OP_INSERT (legacy)"
	case 2004:
		return "OP_QUERY (legacy)"
	case 2005:
		return "OP_GET_MORE (legacy)"
	case 2006:
		return "OP_DELETE (legacy)"
	case 2007:
		return "OP_KILL_CURSORS (legacy)"
	case 2012:
		return "OP_COMPRESSED"
	case 2013:
		return "OP_MSG"
	default:
		return fmt.Sprintf("UNKNOWN")
	}
}

// extractCommandName attempts to extract the command name from an OP_MSG message
// OP_MSG format: flags(4) + sections...
// Section 0: kind(1) + BSON document (first field is usually the command)
func extractCommandName(message []byte) string {
	if len(message) < 21 {
		return ""
	}

	// Skip wire protocol header (16 bytes) and flags (4 bytes)
	offset := 20

	// Read section kind
	if message[offset] != 0 {
		return "" // Not a section kind 0
	}
	offset++

	// BSON document starts here
	// BSON format: size(4) + elements... + null(1)
	if offset+5 > len(message) {
		return ""
	}

	// Skip BSON size
	offset += 4

	// Read first element type
	if offset >= len(message) {
		return ""
	}
	// elementType := message[offset]
	offset++

	// Read element name (null-terminated)
	nameStart := offset
	for offset < len(message) && message[offset] != 0 {
		offset++
	}

	if offset >= len(message) {
		return ""
	}

	return string(message[nameStart:offset])
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
