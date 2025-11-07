package sender

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Sender manages connections to MongoDB and sends commands from recorded traffic
type Sender struct {
	client *mongo.Client
	ctx    context.Context
}

// New creates a new Sender with a connection to MongoDB
func New(ctx context.Context, uri string, opts ...*options.ClientOptions) (*Sender, error) {
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

	return &Sender{
		client: client,
		ctx:    ctx,
	}, nil
}

// Close closes the connection to MongoDB
func (s *Sender) Close() error {
	if s.client != nil {
		return s.client.Disconnect(s.ctx)
	}
	return nil
}

// SendCommand sends a BSON command to the specified database
// The command document should already have internal fields cleaned
func (s *Sender) SendCommand(database string, command bson.M) (*Result, error) {
	startTime := time.Now()

	// Get database handle
	db := s.client.Database(database)

	// Execute the command
	var result bson.M
	err := db.RunCommand(s.ctx, command).Decode(&result)

	duration := time.Since(startTime)

	if err != nil {
		return &Result{
			Success:  false,
			Error:    err,
			Duration: duration,
		}, err
	}

	return &Result{
		Success:  true,
		Response: result,
		Duration: duration,
	}, nil
}

// SendCommandWithTimeout sends a command with a specific timeout
func (s *Sender) SendCommandWithTimeout(database string, command bson.M, timeout time.Duration) (*Result, error) {
	ctx, cancel := context.WithTimeout(s.ctx, timeout)
	defer cancel()

	startTime := time.Now()

	db := s.client.Database(database)
	var result bson.M
	err := db.RunCommand(ctx, command).Decode(&result)

	duration := time.Since(startTime)

	if err != nil {
		return &Result{
			Success:  false,
			Error:    err,
			Duration: duration,
		}, err
	}

	return &Result{
		Success:  true,
		Response: result,
		Duration: duration,
	}, nil
}

// Client returns the underlying MongoDB client
// This allows advanced usage if needed
func (s *Sender) Client() *mongo.Client {
	return s.client
}
