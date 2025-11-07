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
		fmt.Fprintf(os.Stderr, "Usage: %s <recording-file> <mongodb-uri> [options]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nReplays recorded MongoDB traffic against a target MongoDB instance.\n")
		fmt.Fprintf(os.Stderr, "\nArguments:\n")
		fmt.Fprintf(os.Stderr, "  recording-file  Path to the traffic recording file\n")
		fmt.Fprintf(os.Stderr, "  mongodb-uri     MongoDB connection URI (e.g., mongodb://localhost:27017)\n")
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		fmt.Fprintf(os.Stderr, "  --requests-only Only replay requests (skip responses)\n")
		fmt.Fprintf(os.Stderr, "  --user-ops      Only replay user operations (skip internal ops)\n")
		fmt.Fprintf(os.Stderr, "  --dry-run       Parse and validate commands without sending them\n")
		fmt.Fprintf(os.Stderr, "  --limit N       Limit replay to first N operations\n")
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s recording.bin mongodb://localhost:27017 --requests-only --user-ops\n", os.Args[0])
		os.Exit(1)
	}

	filePath := os.Args[1]
	mongoURI := os.Args[2]

	// Parse options
	requestsOnly := false
	userOpsOnly := false
	dryRun := false
	limit := 0

	for i := 3; i < len(os.Args); i++ {
		switch os.Args[i] {
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
		}
	}

	// Open recording file
	rec, err := reader.NewRecordingReader(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening recording: %v\n", err)
		os.Exit(1)
	}
	defer rec.Close()

	// Connect to MongoDB (unless dry-run)
	var snd *sender.Sender
	if !dryRun {
		ctx := context.Background()
		snd, err = sender.New(ctx, mongoURI)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error connecting to MongoDB: %v\n", err)
			os.Exit(1)
		}
		defer snd.Close()
		fmt.Printf("Connected to MongoDB at %s\n", mongoURI)
	} else {
		fmt.Println("DRY RUN MODE - Commands will be parsed but not sent")
	}

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
	fmt.Println()

	// Statistics
	totalPackets := 0
	skippedPackets := 0
	successfulOps := 0
	failedOps := 0
	startTime := time.Now()

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

	duration := time.Since(startTime)

	// Print summary
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

	if failedOps > 0 {
		os.Exit(1)
	}
}
