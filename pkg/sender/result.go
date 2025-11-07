package sender

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Result represents the result of sending a command to MongoDB
type Result struct {
	// Success indicates whether the command succeeded
	Success bool

	// Response contains the BSON response document from MongoDB (if successful)
	Response bson.M

	// Error contains the error if the command failed
	Error error

	// Duration is how long the command took to execute
	Duration time.Duration
}

// String returns a human-readable string representation of the result
func (r *Result) String() string {
	if r.Success {
		return fmt.Sprintf("Success (took %v)", r.Duration)
	}
	return fmt.Sprintf("Failed: %v (took %v)", r.Error, r.Duration)
}

// IsOK returns true if the MongoDB response indicates success (ok: 1)
// Some operations may complete without error but have ok: 0 in the response
func (r *Result) IsOK() bool {
	if !r.Success {
		return false
	}

	if r.Response == nil {
		return false
	}

	// Check for "ok" field
	if ok, exists := r.Response["ok"]; exists {
		// MongoDB returns ok as a number (1 for success, 0 for failure)
		switch v := ok.(type) {
		case int:
			return v == 1
		case int32:
			return v == 1
		case int64:
			return v == 1
		case float64:
			return v == 1.0
		}
	}

	// If no "ok" field, assume success since the command didn't error
	return true
}
