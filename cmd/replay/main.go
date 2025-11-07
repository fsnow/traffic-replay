package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fsnow/traffic-replay/pkg/reader"
	"github.com/fsnow/traffic-replay/pkg/sender"
)

func main() {
	if len(os.Args) < 3 {
		printUsage()
		os.Exit(1)
	}

	filePath := os.Args[1]
	mongoURI := os.Args[2]

	// Parse options
	replayMode := "raw" // default: raw wire protocol mode
	requestsOnly := false
	userOpsOnly := false
	dryRun := false
	limit := 0
	speed := 1.0 // default: 1x speed (preserve original timing)

	for i := 3; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--mode":
			if i+1 < len(os.Args) {
				replayMode = os.Args[i+1]
				i++
			}
		case "--requests-only":
			requestsOnly = true
		case "--user-ops":
			userOpsOnly = true
		case "--dry-run":
			dryRun = true
		case "--limit":
			if i+1 < len(os.Args) {
				fmt.Sscanf(os.Args[i+1], "%d", &limit)
				i++
			}
		case "--speed":
			if i+1 < len(os.Args) {
				fmt.Sscanf(os.Args[i+1], "%f", &speed)
				i++
			}
		}
	}

	// Validate mode
	if replayMode != "raw" && replayMode != "command" {
		fmt.Fprintf(os.Stderr, "Error: Invalid mode '%s'. Must be 'raw' or 'command'\n", replayMode)
		os.Exit(1)
	}

	// Open recording file
	rec, err := reader.NewRecordingReader(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening recording: %v\n", err)
		os.Exit(1)
	}
	defer rec.Close()

	// Print header
	fmt.Printf("Replay Mode: %s\n", strings.ToUpper(replayMode))
	fmt.Printf("Replaying from: %s\n", filePath)
	if requestsOnly {
		fmt.Println("Filter: Requests only")
	}
	if userOpsOnly {
		fmt.Println("Filter: User operations only")
	}
	if limit > 0 {
		fmt.Printf("Limit: %d operations\n", limit)
	}
	if speed == 0 {
		fmt.Println("Speed: Fast-forward (no delays)")
	} else {
		fmt.Printf("Speed: %.1fx\n", speed)
	}

	// Replay based on mode
	if replayMode == "raw" {
		runRawMode(rec, mongoURI, requestsOnly, userOpsOnly, dryRun, limit, speed)
	} else {
		runCommandMode(rec, mongoURI, requestsOnly, userOpsOnly, dryRun, limit, speed)
	}
}

func runRawMode(rec *reader.RecordingReader, mongoURI string, requestsOnly, userOpsOnly, dryRun bool, limit int, speed float64) {
	ctx := context.Background()

	// Connect to MongoDB (unless dry-run)
	var rawSender *sender.RawSender
	if !dryRun {
		var err error
		rawSender, err = sender.NewRawSender(ctx, mongoURI)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error connecting to MongoDB: %v\n", err)
			os.Exit(1)
		}
		defer rawSender.Close()
		fmt.Printf("Connected to MongoDB at %s (raw mode)\n", mongoURI)
	} else {
		fmt.Println("DRY RUN MODE - Wire messages will be validated but not sent")
	}
	fmt.Println()

	// Statistics
	totalPackets := 0
	skippedPackets := 0
	successfulOps := 0
	failedOps := 0
	startTime := time.Now()

	// Timing state
	var lastOffset uint64
	firstOp := true

	// Replay loop
	for {
		packet, err := rec.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading packet: %v\n", err)
			os.Exit(1)
		}

		totalPackets++

		// Apply filters
		if requestsOnly && !packet.IsRequest() {
			skippedPackets++
			continue
		}

		if userOpsOnly && !packet.IsLikelyUserOperation() {
			skippedPackets++
			continue
		}

		// Check if packet has a wire message
		if len(packet.Message) == 0 {
			skippedPackets++
			continue
		}

		// Check limit
		if limit > 0 && (successfulOps+failedOps) >= limit {
			fmt.Printf("\nReached limit of %d operations\n", limit)
			break
		}

		// Timing logic (unless speed is 0 for fast-forward)
		if speed > 0 {
			if firstOp {
				lastOffset = packet.Offset
				firstOp = false
			} else {
				// Calculate delay based on packet offsets (in microseconds)
				delay := time.Duration(float64(packet.Offset-lastOffset)/speed) * time.Microsecond
				if delay > 0 {
					time.Sleep(delay)
				}
				lastOffset = packet.Offset
			}
		}

		// Send raw wire message (or just validate in dry-run mode)
		if dryRun {
			// Just validate the wire message header
			cmd := packet.ExtractCommandName()
			db := packet.ExtractDatabase()
			fmt.Printf("[DRY RUN] %s.%s (raw wire message, %d bytes)\n", db, cmd, len(packet.Message))
			successfulOps++
		} else {
			result, err := rawSender.SendRawWireMessage(ctx, packet.Message)
			if err != nil {
				cmd := packet.ExtractCommandName()
				db := packet.ExtractDatabase()
				fmt.Printf("❌ FAILED: %s.%s - %v\n", db, cmd, err)
				failedOps++
			} else {
				fmt.Printf("✓ %s (reqID=%d, took %v)\n", result.OpCode.String(), result.RequestID, result.Duration)
				successfulOps++
			}
		}
	}

	printSummary(totalPackets, skippedPackets, successfulOps, failedOps, time.Since(startTime))

	if failedOps > 0 {
		os.Exit(1)
	}
}

func runCommandMode(rec *reader.RecordingReader, mongoURI string, requestsOnly, userOpsOnly, dryRun bool, limit int, speed float64) {
	ctx := context.Background()

	// Connect to MongoDB (unless dry-run)
	var snd *sender.Sender
	if !dryRun {
		var err error
		snd, err = sender.New(ctx, mongoURI)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error connecting to MongoDB: %v\n", err)
			os.Exit(1)
		}
		defer snd.Close()
		fmt.Printf("Connected to MongoDB at %s (command mode)\n", mongoURI)
	} else {
		fmt.Println("DRY RUN MODE - Commands will be parsed but not sent")
	}
	fmt.Println()

	// Statistics
	totalPackets := 0
	skippedPackets := 0
	successfulOps := 0
	failedOps := 0
	startTime := time.Now()

	// Timing state
	var lastOffset uint64
	firstOp := true

	// Replay loop
	for {
		packet, err := rec.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading packet: %v\n", err)
			os.Exit(1)
		}

		totalPackets++

		// Apply filters
		if requestsOnly && !packet.IsRequest() {
			skippedPackets++
			continue
		}

		if userOpsOnly && !packet.IsLikelyUserOperation() {
			skippedPackets++
			continue
		}

		// Extract command
		cmd, err := sender.ExtractCommand(packet)
		if err != nil {
			// Skip packets that can't be parsed
			skippedPackets++
			continue
		}

		// Check limit
		if limit > 0 && (successfulOps+failedOps) >= limit {
			fmt.Printf("\nReached limit of %d operations\n", limit)
			break
		}

		// Timing logic (unless speed is 0 for fast-forward)
		if speed > 0 {
			if firstOp {
				lastOffset = packet.Offset
				firstOp = false
			} else {
				// Calculate delay based on packet offsets (in microseconds)
				delay := time.Duration(float64(packet.Offset-lastOffset)/speed) * time.Microsecond
				if delay > 0 {
					time.Sleep(delay)
				}
				lastOffset = packet.Offset
			}
		}

		// Send command (or just print in dry-run mode)
		if dryRun {
			fmt.Printf("[DRY RUN] %s.%s\n", cmd.Database, cmd.Name)
			successfulOps++
		} else {
			result, err := snd.SendCommand(cmd.Database, cmd.Document)
			if err != nil {
				fmt.Printf("❌ FAILED: %s.%s - %v\n", cmd.Database, cmd.Name, err)
				failedOps++
			} else if !result.IsOK() {
				fmt.Printf("⚠️  WARNING: %s.%s - ok=0 (took %v)\n", cmd.Database, cmd.Name, result.Duration)
				failedOps++
			} else {
				fmt.Printf("✓ %s.%s (took %v)\n", cmd.Database, cmd.Name, result.Duration)
				successfulOps++
			}
		}
	}

	printSummary(totalPackets, skippedPackets, successfulOps, failedOps, time.Since(startTime))

	if failedOps > 0 {
		os.Exit(1)
	}
}

func printSummary(totalPackets, skippedPackets, successfulOps, failedOps int, duration time.Duration) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("REPLAY SUMMARY")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Total packets:       %d\n", totalPackets)
	fmt.Printf("Skipped packets:     %d\n", skippedPackets)
	fmt.Printf("Successful ops:      %d\n", successfulOps)
	fmt.Printf("Failed ops:          %d\n", failedOps)
	fmt.Printf("Duration:            %v\n", duration)
	if successfulOps+failedOps > 0 {
		fmt.Printf("Average per op:      %v\n", duration/time.Duration(successfulOps+failedOps))
	}
	fmt.Println(strings.Repeat("=", 60))
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s <recording-file> <mongodb-uri> [options]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nReplays recorded MongoDB traffic against a target MongoDB instance.\n")
	fmt.Fprintf(os.Stderr, "\nArguments:\n")
	fmt.Fprintf(os.Stderr, "  recording-file  Path to the traffic recording file\n")
	fmt.Fprintf(os.Stderr, "  mongodb-uri     MongoDB connection URI (e.g., mongodb://localhost:27017)\n")
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	fmt.Fprintf(os.Stderr, "  --mode MODE        Replay mode: 'raw' or 'command' (default: raw)\n")
	fmt.Fprintf(os.Stderr, "                     raw:     Send exact wire protocol bytes (exact replay)\n")
	fmt.Fprintf(os.Stderr, "                     command: Parse and re-execute via RunCommand (semantic replay)\n")
	fmt.Fprintf(os.Stderr, "  --speed MULTIPLIER Replay speed multiplier (default: 1.0 for original timing)\n")
	fmt.Fprintf(os.Stderr, "                     1.0:     Original timing (default)\n")
	fmt.Fprintf(os.Stderr, "                     2.0:     2x faster\n")
	fmt.Fprintf(os.Stderr, "                     0.5:     Half speed\n")
	fmt.Fprintf(os.Stderr, "                     0:       Fast-forward (no delays)\n")
	fmt.Fprintf(os.Stderr, "  --requests-only    Only replay requests (skip responses)\n")
	fmt.Fprintf(os.Stderr, "  --user-ops         Only replay user operations (skip internal ops)\n")
	fmt.Fprintf(os.Stderr, "  --dry-run          Parse and validate without sending\n")
	fmt.Fprintf(os.Stderr, "  --limit N          Limit replay to first N operations\n")
	fmt.Fprintf(os.Stderr, "\nExamples:\n")
	fmt.Fprintf(os.Stderr, "  # Raw mode with original timing (default)\n")
	fmt.Fprintf(os.Stderr, "  %s recording.bin mongodb://localhost:27017 --requests-only\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\n  # Fast-forward mode (no delays)\n")
	fmt.Fprintf(os.Stderr, "  %s recording.bin mongodb://localhost:27017 --speed 0 --requests-only\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\n  # Command mode at 2x speed\n")
	fmt.Fprintf(os.Stderr, "  %s recording.bin mongodb://localhost:27017 --mode command --speed 2.0 --user-ops\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\n  # Dry run to validate\n")
	fmt.Fprintf(os.Stderr, "  %s recording.bin mongodb://localhost:27017 --dry-run --limit 100\n", os.Args[0])
}
