package sender

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/description"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/mnet"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/wiremessage"
)

// RawSender sends raw wire protocol messages using the MongoDB driver's
// experimental internal APIs. This provides exact replay of captured traffic.
//
// WARNING: This uses experimental/internal driver packages that may change
// without notice. We pin the driver version to manage this risk.
type RawSender struct {
	client     *mongo.Client
	deployment driver.Deployment
	ctx        context.Context
}

// NewRawSender creates a new RawSender with a connection to MongoDB
// It uses the driver's connection pool for auth, TLS, and connection management
func NewRawSender(ctx context.Context, uri string, opts ...*options.ClientOptions) (*RawSender, error) {
	// Build combined options (v2 API: Connect doesn't take context)
	clientOpts := options.Client().ApplyURI(uri)
	for _, opt := range opts {
		clientOpts = options.MergeClientOptions(clientOpts, opt)
	}

	// Connect to MongoDB (v2: Connect doesn't take ctx as first parameter)
	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping to verify connection
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	// Access the internal deployment field using reflection
	// WARNING: This uses reflection to access private fields and is fragile
	deployment, err := getDeploymentFromClient(client)
	if err != nil {
		client.Disconnect(ctx)
		return nil, fmt.Errorf("failed to access client deployment: %w", err)
	}

	return &RawSender{
		client:     client,
		deployment: deployment,
		ctx:        ctx,
	}, nil
}

// getDeploymentFromClient uses reflection to extract the private deployment field
// from a mongo.Client. This is necessary because the Go driver doesn't expose
// the topology/deployment for raw wire message access.
//
// WARNING: This is fragile and may break if the driver's internal structure changes.
func getDeploymentFromClient(client *mongo.Client) (driver.Deployment, error) {
	// Use reflection to access the private "deployment" field
	clientValue := reflect.ValueOf(client)
	if clientValue.Kind() == reflect.Ptr {
		clientValue = clientValue.Elem()
	}

	deploymentField := clientValue.FieldByName("deployment")
	if !deploymentField.IsValid() {
		return nil, fmt.Errorf("deployment field not found in Client struct (driver API may have changed)")
	}

	// The deployment field is private, so we need to use reflection to access it
	// Create a new reflect.Value that we can interface{} to get the actual value
	deployment, ok := deploymentField.Interface().(driver.Deployment)
	if !ok {
		return nil, fmt.Errorf("deployment field is not of type driver.Deployment")
	}

	return deployment, nil
}

// SendRawWireMessage sends a raw wire protocol message directly to MongoDB
// The message should be the raw bytes from packet.Message (starting with the wire protocol header)
func (s *RawSender) SendRawWireMessage(ctx context.Context, wireMessageBytes []byte) (*RawResult, error) {
	startTime := time.Now()

	// Validate the wire message header
	header, err := s.validateWireMessage(wireMessageBytes)
	if err != nil {
		return &RawResult{
			Success:  false,
			Error:    err,
			Duration: time.Since(startTime),
		}, err
	}

	// Get a connection from the pool
	conn, err := s.getConnection(ctx)
	if err != nil {
		return &RawResult{
			Success:  false,
			Error:    err,
			Duration: time.Since(startTime),
		}, err
	}

	// Write the raw wire message bytes
	err = conn.Write(ctx, wireMessageBytes)
	if err != nil {
		return &RawResult{
			Success:    false,
			Error:      err,
			Duration:   time.Since(startTime),
			OpCode:     header.OpCode,
			RequestID:  header.RequestID,
			ResponseTo: header.ResponseTo,
		}, err
	}

	return &RawResult{
		Success:    true,
		Duration:   time.Since(startTime),
		OpCode:     header.OpCode,
		RequestID:  header.RequestID,
		ResponseTo: header.ResponseTo,
	}, nil
}

// SendRawWireMessageWithResponse sends a raw wire message and reads the response
// This is used for validation modes
func (s *RawSender) SendRawWireMessageWithResponse(ctx context.Context, wireMessageBytes []byte) (*RawResult, error) {
	startTime := time.Now()

	// Validate the wire message header
	header, err := s.validateWireMessage(wireMessageBytes)
	if err != nil {
		return &RawResult{
			Success:  false,
			Error:    err,
			Duration: time.Since(startTime),
		}, err
	}

	// Get a connection from the pool
	conn, err := s.getConnection(ctx)
	if err != nil {
		return &RawResult{
			Success:  false,
			Error:    err,
			Duration: time.Since(startTime),
		}, err
	}

	// Write the raw wire message bytes
	err = conn.Write(ctx, wireMessageBytes)
	if err != nil {
		return &RawResult{
			Success:    false,
			Error:      err,
			Duration:   time.Since(startTime),
			OpCode:     header.OpCode,
			RequestID:  header.RequestID,
			ResponseTo: header.ResponseTo,
		}, err
	}

	// Read the response
	responseBytes, err := conn.Read(ctx)
	if err != nil {
		return &RawResult{
			Success:    false,
			Error:      err,
			Duration:   time.Since(startTime),
			OpCode:     header.OpCode,
			RequestID:  header.RequestID,
			ResponseTo: header.ResponseTo,
		}, err
	}

	return &RawResult{
		Success:       true,
		Duration:      time.Since(startTime),
		OpCode:        header.OpCode,
		RequestID:     header.RequestID,
		ResponseTo:    header.ResponseTo,
		ResponseBytes: responseBytes,
	}, nil
}

// validateWireMessage validates and parses a wire protocol message header
func (s *RawSender) validateWireMessage(wireMessageBytes []byte) (*WireMessageHeader, error) {
	// Parse wire protocol header using driver's wiremessage package
	length, requestID, responseTo, opcode, _, ok := wiremessage.ReadHeader(wireMessageBytes)
	if !ok {
		return nil, fmt.Errorf("invalid wire message header")
	}

	// Check for unsupported legacy opcodes
	if isLegacyOpCode(opcode) {
		return nil, fmt.Errorf("unsupported legacy opcode: %s (removed in MongoDB 5.1+)", opcode)
	}

	// Validate message length matches buffer
	if int(length) != len(wireMessageBytes) {
		return nil, fmt.Errorf("wire message length mismatch: header says %d bytes, got %d bytes", length, len(wireMessageBytes))
	}

	return &WireMessageHeader{
		Length:     length,
		RequestID:  requestID,
		ResponseTo: responseTo,
		OpCode:     opcode,
	}, nil
}

// isLegacyOpCode checks if an opcode is a legacy opcode removed in MongoDB 5.1+
func isLegacyOpCode(opcode wiremessage.OpCode) bool {
	switch opcode {
	case wiremessage.OpQuery,
		wiremessage.OpInsert,
		wiremessage.OpUpdate,
		wiremessage.OpDelete,
		wiremessage.OpGetMore,
		wiremessage.OpKillCursors:
		return true
	default:
		return false
	}
}

// getConnection gets a connection from the driver's connection pool
func (s *RawSender) getConnection(ctx context.Context) (*mnet.Connection, error) {
	// Select a server from the deployment
	// Use a simple selector that picks any available writable server
	selector := &writeSelector{}
	server, err := s.deployment.SelectServer(ctx, selector)
	if err != nil {
		return nil, fmt.Errorf("failed to select server: %w", err)
	}

	// Get a connection from the server's pool
	conn, err := server.Connection(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	return conn, nil
}

// writeSelector is a simple server selector that selects writeable servers
type writeSelector struct{}

// SelectServer selects servers that can handle write operations
func (ws *writeSelector) SelectServer(t description.Topology, servers []description.Server) ([]description.Server, error) {
	// Filter for writable servers (primary in replica set, any in standalone/sharded)
	var writeable []description.Server
	for _, server := range servers {
		switch server.Kind {
		case description.ServerKindMongos,
			description.ServerKindStandalone,
			description.ServerKindRSPrimary,
			description.ServerKindLoadBalancer:
			writeable = append(writeable, server)
		}
	}

	if len(writeable) == 0 {
		return nil, fmt.Errorf("no writable servers available")
	}

	return writeable, nil
}

// Close closes the connection to MongoDB
func (s *RawSender) Close() error {
	if s.client != nil {
		return s.client.Disconnect(s.ctx)
	}
	return nil
}

// Client returns the underlying MongoDB client
func (s *RawSender) Client() *mongo.Client {
	return s.client
}
