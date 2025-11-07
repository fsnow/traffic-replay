package sender

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/wiremessage"
)

// RawResult represents the result of sending a raw wire protocol message
type RawResult struct {
	// Success indicates whether the message was sent successfully
	Success bool

	// Error contains the error if the send failed
	Error error

	// Duration is how long the operation took
	Duration time.Duration

	// OpCode is the wire protocol opcode
	OpCode wiremessage.OpCode

	// RequestID is the request ID from the wire protocol header
	RequestID int32

	// ResponseTo is the responseTo field from the wire protocol header
	ResponseTo int32

	// ResponseBytes contains the raw response bytes (if response was read)
	ResponseBytes []byte
}

// String returns a human-readable string representation of the result
func (r *RawResult) String() string {
	if r.Success {
		return fmt.Sprintf("Success: %s (reqID=%d, took %v)",
			r.OpCode.String(), r.RequestID, r.Duration)
	}
	return fmt.Sprintf("Failed: %s (reqID=%d, took %v): %v",
		r.OpCode.String(), r.RequestID, r.Duration, r.Error)
}

// IsRequest returns true if this is a request (responseTo == 0)
func (r *RawResult) IsRequest() bool {
	return r.ResponseTo == 0
}

// IsResponse returns true if this is a response (responseTo != 0)
func (r *RawResult) IsResponse() bool {
	return r.ResponseTo != 0
}

// WireMessageHeader represents a parsed wire protocol message header
type WireMessageHeader struct {
	// Length is the total message length in bytes
	Length int32

	// RequestID is the request identifier
	RequestID int32

	// ResponseTo is the request ID this is responding to (0 for requests)
	ResponseTo int32

	// OpCode is the wire protocol operation code
	OpCode wiremessage.OpCode
}

// String returns a human-readable representation of the header
func (h *WireMessageHeader) String() string {
	return fmt.Sprintf("OpCode=%s, Length=%d, RequestID=%d, ResponseTo=%d",
		h.OpCode.String(), h.Length, h.RequestID, h.ResponseTo)
}
