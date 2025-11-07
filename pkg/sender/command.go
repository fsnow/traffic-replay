package sender

import (
	"fmt"

	"github.com/fsnow/traffic-replay/pkg/reader"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Command represents a MongoDB command extracted from a recorded packet
type Command struct {
	// Database is the target database name
	Database string

	// Name is the command name (e.g., "insert", "find", "update")
	Name string

	// Document is the BSON command document (with internal fields cleaned)
	Document bson.M

	// OriginalPacket is a reference to the original packet (optional)
	OriginalPacket *reader.Packet
}

// ExtractCommand extracts a Command from a recorded packet
// It parses the BSON document and cleans internal fields
func ExtractCommand(packet *reader.Packet) (*Command, error) {
	// Only support OP_MSG (2013)
	opCode := packet.GetOpCode()
	if opCode != 2013 {
		return nil, fmt.Errorf("unsupported opcode: %d (only OP_MSG/2013 is supported)", opCode)
	}

	// Extract command name
	cmdName := packet.ExtractCommandName()
	if cmdName == "" {
		return nil, fmt.Errorf("failed to extract command name")
	}

	// Extract database name
	database := packet.ExtractDatabase()
	if database == "" {
		return nil, fmt.Errorf("failed to extract database name")
	}

	// Extract BSON document from OP_MSG
	// Structure: Header (16) + Flags (4) + Section kind (1) + BSON document
	if len(packet.Message) < 21 {
		return nil, fmt.Errorf("packet too short to contain BSON document")
	}

	offset := 16 + 4 + 1 // Skip header, flags, section kind
	bsonData := packet.Message[offset:]

	// Parse BSON document
	var doc bson.M
	if err := bson.Unmarshal(bsonData, &doc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	// Clean internal fields
	doc = cleanInternalFields(doc)

	return &Command{
		Database:       database,
		Name:           cmdName,
		Document:       doc,
		OriginalPacket: packet,
	}, nil
}

// cleanInternalFields removes driver/server internal fields from BSON documents
// This is the same logic used in script-gen
func cleanInternalFields(doc bson.M) bson.M {
	cleaned := bson.M{}

	// List of internal driver/server fields to remove
	internalFields := map[string]bool{
		"$clusterTime":      true,
		"$db":               true,
		"$readPreference":   true,
		"lsid":              true,
		"txnNumber":         true,
		"autocommit":        true,
		"startTransaction":  true,
		"readConcern":       true, // Usually set by driver
		"writeConcern":      true, // Usually set by driver
	}

	for key, value := range doc {
		// Skip internal fields
		if internalFields[key] {
			continue
		}

		// Recursively clean nested documents
		switch v := value.(type) {
		case bson.M:
			cleaned[key] = cleanInternalFields(v)
		case bson.A:
			cleaned[key] = cleanInternalFieldsArray(v)
		default:
			cleaned[key] = value
		}
	}

	return cleaned
}

// cleanInternalFieldsArray recursively cleans internal fields from BSON arrays
func cleanInternalFieldsArray(arr bson.A) bson.A {
	cleaned := make(bson.A, len(arr))

	for i, item := range arr {
		switch v := item.(type) {
		case bson.M:
			cleaned[i] = cleanInternalFields(v)
		case bson.A:
			cleaned[i] = cleanInternalFieldsArray(v)
		default:
			cleaned[i] = item
		}
	}

	return cleaned
}
