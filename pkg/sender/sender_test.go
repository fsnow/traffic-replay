package sender

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// TestSenderIntegration tests the Sender against a real MongoDB instance
// Set MONGODB_URI environment variable to run this test
// Example: MONGODB_URI=mongodb://localhost:27017 go test -v ./pkg/sender
func TestSenderIntegration(t *testing.T) {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		t.Skip("Skipping integration test: MONGODB_URI not set")
	}

	ctx := context.Background()

	// Create sender
	sender, err := New(ctx, uri)
	if err != nil {
		t.Fatalf("Failed to create sender: %v", err)
	}
	defer sender.Close()

	// Test database
	testDB := "traffic_replay_test"

	// Clean up before test
	cleanupCmd := bson.M{"dropDatabase": 1}
	sender.SendCommand(testDB, cleanupCmd)

	t.Run("insert command", func(t *testing.T) {
		cmd := bson.M{
			"insert": "users",
			"documents": bson.A{
				bson.M{"name": "Alice", "age": 30},
				bson.M{"name": "Bob", "age": 25},
			},
		}

		result, err := sender.SendCommand(testDB, cmd)
		if err != nil {
			t.Fatalf("Failed to send insert command: %v", err)
		}

		if !result.Success {
			t.Errorf("Insert command failed: %v", result.Error)
		}

		if !result.IsOK() {
			t.Errorf("Insert command returned ok=0: %v", result.Response)
		}

		if result.Duration == 0 {
			t.Error("Duration should be non-zero")
		}

		t.Logf("Insert took %v", result.Duration)
	})

	t.Run("find command", func(t *testing.T) {
		cmd := bson.M{
			"find":   "users",
			"filter": bson.M{"name": "Alice"},
		}

		result, err := sender.SendCommand(testDB, cmd)
		if err != nil {
			t.Fatalf("Failed to send find command: %v", err)
		}

		if !result.Success {
			t.Errorf("Find command failed: %v", result.Error)
		}

		if !result.IsOK() {
			t.Errorf("Find command returned ok=0: %v", result.Response)
		}

		// Check that we got a cursor back
		if _, exists := result.Response["cursor"]; !exists {
			t.Error("Find command should return a cursor")
		}
	})

	t.Run("update command", func(t *testing.T) {
		cmd := bson.M{
			"update": "users",
			"updates": bson.A{
				bson.M{
					"q": bson.M{"name": "Alice"},
					"u": bson.M{"$set": bson.M{"age": 31}},
				},
			},
		}

		result, err := sender.SendCommand(testDB, cmd)
		if err != nil {
			t.Fatalf("Failed to send update command: %v", err)
		}

		if !result.Success {
			t.Errorf("Update command failed: %v", result.Error)
		}

		if !result.IsOK() {
			t.Errorf("Update command returned ok=0: %v", result.Response)
		}
	})

	t.Run("aggregate command", func(t *testing.T) {
		cmd := bson.M{
			"aggregate": "users",
			"pipeline": bson.A{
				bson.M{"$match": bson.M{"age": bson.M{"$gte": 25}}},
				bson.M{"$sort": bson.M{"age": 1}},
			},
			"cursor": bson.M{},
		}

		result, err := sender.SendCommand(testDB, cmd)
		if err != nil {
			t.Fatalf("Failed to send aggregate command: %v", err)
		}

		if !result.Success {
			t.Errorf("Aggregate command failed: %v", result.Error)
		}

		if !result.IsOK() {
			t.Errorf("Aggregate command returned ok=0: %v", result.Response)
		}
	})

	t.Run("command with timeout", func(t *testing.T) {
		cmd := bson.M{
			"find":   "users",
			"filter": bson.M{},
		}

		result, err := sender.SendCommandWithTimeout(testDB, cmd, 5*time.Second)
		if err != nil {
			t.Fatalf("Failed to send command with timeout: %v", err)
		}

		if !result.Success {
			t.Errorf("Command with timeout failed: %v", result.Error)
		}
	})

	// Clean up after test
	t.Run("cleanup", func(t *testing.T) {
		cmd := bson.M{"dropDatabase": 1}
		result, err := sender.SendCommand(testDB, cmd)
		if err != nil {
			t.Logf("Warning: Failed to clean up test database: %v", err)
		} else if !result.IsOK() {
			t.Logf("Warning: Drop database returned ok=0: %v", result.Response)
		}
	})
}

// TestSenderConnectionFailure tests that the sender handles connection failures gracefully
func TestSenderConnectionFailure(t *testing.T) {
	ctx := context.Background()

	// Try to connect to an invalid URI
	_, err := New(ctx, "mongodb://invalid-host:27017")
	if err == nil {
		t.Error("Expected error when connecting to invalid host, got nil")
	}
}
