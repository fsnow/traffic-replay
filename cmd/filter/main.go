package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnow/traffic-replay/pkg/reader"
)

type FilterConfig struct {
	inputFile          string
	outputFile         string
	requestsOnly       bool
	userOpsOnly        bool
	userOpsOnlySmart   bool // Use context-aware filtering
	excludeInternal    bool
	includeCommands    []string
	excludeCommands    []string
	minOffset          uint64
	maxOffset          uint64
	verbose            bool
}

type FilterStats struct {
	inputPackets       int
	outputPackets      int
	droppedResponses   int
	droppedInternal    int
	droppedByCommand   int
	droppedByTime      int
	inputBytes         uint64
	outputBytes        uint64
}

func main() {
	config := &FilterConfig{}

	flag.StringVar(&config.inputFile, "input", "", "Input recording file (required)")
	flag.StringVar(&config.outputFile, "output", "", "Output recording file (required)")
	flag.BoolVar(&config.requestsOnly, "requests-only", false, "Keep only requests, drop responses")
	flag.BoolVar(&config.userOpsOnly, "user-ops-only", false, "Keep only user operations (simple command-based filter)")
	flag.BoolVar(&config.userOpsOnlySmart, "user-ops-smart", false, "Keep only user operations (context-aware: checks db/collection for getMore, etc.)")
	flag.BoolVar(&config.excludeInternal, "exclude-internal", false, "Exclude internal operations (hello, getMore, replication)")

	var includeCommands string
	var excludeCommands string
	flag.StringVar(&includeCommands, "include-commands", "", "Comma-separated list of commands to include")
	flag.StringVar(&excludeCommands, "exclude-commands", "", "Comma-separated list of commands to exclude")

	flag.Uint64Var(&config.minOffset, "min-offset", 0, "Minimum offset (microseconds) - drop packets before this")
	flag.Uint64Var(&config.maxOffset, "max-offset", 0, "Maximum offset (microseconds) - drop packets after this (0=unlimited)")

	flag.BoolVar(&config.verbose, "verbose", false, "Verbose output")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Filter a MongoDB traffic recording file.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Remove responses (typically reduces size by 50-60%%)\n")
		fmt.Fprintf(os.Stderr, "  %s -input recording.bin -output filtered.bin -requests-only\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Keep only user operations - simple filter\n")
		fmt.Fprintf(os.Stderr, "  %s -input recording.bin -output filtered.bin -user-ops-only\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Keep only user operations - smart filter (checks database/collection context)\n")
		fmt.Fprintf(os.Stderr, "  %s -input recording.bin -output filtered.bin -user-ops-smart\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Combine: requests-only + user-ops-only (maximum reduction)\n")
		fmt.Fprintf(os.Stderr, "  %s -input recording.bin -output filtered.bin -requests-only -user-ops-only\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Keep only insert and update operations\n")
		fmt.Fprintf(os.Stderr, "  %s -input recording.bin -output filtered.bin -include-commands insert,update\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Exclude hello and getMore (remove health checks)\n")
		fmt.Fprintf(os.Stderr, "  %s -input recording.bin -output filtered.bin -exclude-commands hello,getMore\n\n", os.Args[0])
	}

	flag.Parse()

	// Validate required flags
	if config.inputFile == "" || config.outputFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	// Parse command lists
	if includeCommands != "" {
		config.includeCommands = strings.Split(includeCommands, ",")
		for i := range config.includeCommands {
			config.includeCommands[i] = strings.TrimSpace(config.includeCommands[i])
		}
	}
	if excludeCommands != "" {
		config.excludeCommands = strings.Split(excludeCommands, ",")
		for i := range config.excludeCommands {
			config.excludeCommands[i] = strings.TrimSpace(config.excludeCommands[i])
		}
	}

	// Run filter
	stats, err := filterRecording(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Print results
	printStats(stats)
}

func filterRecording(config *FilterConfig) (*FilterStats, error) {
	// Open input
	input, err := reader.NewRecordingReader(config.inputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open input: %w", err)
	}
	defer input.Close()

	// Create output directory if needed
	outputDir := filepath.Dir(config.outputFile)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Open output
	output, err := os.Create(config.outputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create output: %w", err)
	}
	defer output.Close()

	stats := &FilterStats{}

	// Process packets
	for {
		packet, err := input.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read packet: %w", err)
		}

		stats.inputPackets++
		stats.inputBytes += uint64(packet.Size)

		// Apply filters
		keep, reason := shouldKeepPacket(packet, config)

		if config.verbose && !keep {
			fmt.Printf("Dropping packet %d: %s (session=%d, cmd=%s)\n",
				stats.inputPackets, reason, packet.SessionID, packet.ExtractCommandName())
		}

		if !keep {
			// Update drop counters based on reason
			switch reason {
			case "response":
				stats.droppedResponses++
			case "internal-operation":
				stats.droppedInternal++
			case "command-filter":
				stats.droppedByCommand++
			case "time-range":
				stats.droppedByTime++
			}
			continue
		}

		// Write packet to output
		if err := writePacket(output, packet); err != nil {
			return nil, fmt.Errorf("failed to write packet: %w", err)
		}

		stats.outputPackets++
		stats.outputBytes += uint64(packet.Size)
	}

	return stats, nil
}

func shouldKeepPacket(packet *reader.Packet, config *FilterConfig) (bool, string) {
	// Time range filter
	if config.minOffset > 0 && packet.Offset < config.minOffset {
		return false, "time-range"
	}
	if config.maxOffset > 0 && packet.Offset > config.maxOffset {
		return false, "time-range"
	}

	// Requests-only filter
	if config.requestsOnly {
		if len(packet.Message) == 0 {
			// Empty message - keep it (might be session event)
		} else if !packet.IsRequest() {
			return false, "response"
		}
	}

	// User operations only (simple)
	if config.userOpsOnly {
		if len(packet.Message) == 0 {
			// Empty message - drop it
			return false, "empty-message"
		}
		if !packet.IsUserOperation() {
			return false, "internal-operation"
		}
	}

	// User operations only (smart - context-aware)
	if config.userOpsOnlySmart {
		if len(packet.Message) == 0 {
			return false, "empty-message"
		}
		if !packet.IsLikelyUserOperation() {
			return false, "internal-operation"
		}
	}

	// Exclude internal operations
	if config.excludeInternal {
		if packet.IsInternalOperation() {
			return false, "internal-operation"
		}
	}

	// Command filters
	cmd := packet.ExtractCommandName()

	// Include commands (whitelist)
	if len(config.includeCommands) > 0 {
		found := false
		for _, includeCmd := range config.includeCommands {
			if cmd == includeCmd {
				found = true
				break
			}
		}
		if !found {
			return false, "command-filter"
		}
	}

	// Exclude commands (blacklist)
	if len(config.excludeCommands) > 0 {
		for _, excludeCmd := range config.excludeCommands {
			if cmd == excludeCmd {
				return false, "command-filter"
			}
		}
	}

	return true, ""
}

func writePacket(w io.Writer, packet *reader.Packet) error {
	// Reconstruct the packet in binary format
	// Format: size(4) + id(8) + session(null-terminated) + offset(8) + order(8) + message

	// Calculate total size
	headerSize := 4 + 8 + len(packet.SessionMetadata) + 1 + 8 + 8
	totalSize := uint32(headerSize + len(packet.Message))

	// Write size
	if err := writeLittleEndianUint32(w, totalSize); err != nil {
		return err
	}

	// Write session ID
	if err := writeLittleEndianUint64(w, packet.SessionID); err != nil {
		return err
	}

	// Write session metadata (null-terminated)
	if _, err := w.Write([]byte(packet.SessionMetadata)); err != nil {
		return err
	}
	if _, err := w.Write([]byte{0}); err != nil {
		return err
	}

	// Write offset
	if err := writeLittleEndianUint64(w, packet.Offset); err != nil {
		return err
	}

	// Write order
	if err := writeLittleEndianUint64(w, packet.Order); err != nil {
		return err
	}

	// Write message
	if len(packet.Message) > 0 {
		if _, err := w.Write(packet.Message); err != nil {
			return err
		}
	}

	return nil
}

func writeLittleEndianUint32(w io.Writer, v uint32) error {
	b := make([]byte, 4)
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
	_, err := w.Write(b)
	return err
}

func writeLittleEndianUint64(w io.Writer, v uint64) error {
	b := make([]byte, 8)
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
	b[4] = byte(v >> 32)
	b[5] = byte(v >> 40)
	b[6] = byte(v >> 48)
	b[7] = byte(v >> 56)
	_, err := w.Write(b)
	return err
}

func printStats(stats *FilterStats) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("FILTER RESULTS")
	fmt.Println(strings.Repeat("=", 80))

	fmt.Printf("\nInput:\n")
	fmt.Printf("  Packets: %d\n", stats.inputPackets)
	fmt.Printf("  Size:    %s\n", formatBytes(stats.inputBytes))

	fmt.Printf("\nOutput:\n")
	fmt.Printf("  Packets: %d\n", stats.outputPackets)
	fmt.Printf("  Size:    %s\n", formatBytes(stats.outputBytes))

	fmt.Printf("\nReduction:\n")
	packetsDropped := stats.inputPackets - stats.outputPackets
	bytesDropped := stats.inputBytes - stats.outputBytes

	if stats.inputPackets > 0 {
		pctPackets := float64(packetsDropped) / float64(stats.inputPackets) * 100
		fmt.Printf("  Packets dropped: %d (%.1f%%)\n", packetsDropped, pctPackets)
	}

	if stats.inputBytes > 0 {
		pctBytes := float64(bytesDropped) / float64(stats.inputBytes) * 100
		fmt.Printf("  Bytes dropped:   %s (%.1f%%)\n", formatBytes(bytesDropped), pctBytes)
	}

	if packetsDropped > 0 {
		fmt.Printf("\nDropped by reason:\n")
		if stats.droppedResponses > 0 {
			fmt.Printf("  Responses:           %d\n", stats.droppedResponses)
		}
		if stats.droppedInternal > 0 {
			fmt.Printf("  Internal operations: %d\n", stats.droppedInternal)
		}
		if stats.droppedByCommand > 0 {
			fmt.Printf("  Command filters:     %d\n", stats.droppedByCommand)
		}
		if stats.droppedByTime > 0 {
			fmt.Printf("  Time range:          %d\n", stats.droppedByTime)
		}
	}

	fmt.Println()
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
