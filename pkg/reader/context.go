package reader

import (
	"encoding/binary"
	"strings"
)

// ExtractDatabase attempts to extract the database name from a packet
// Returns empty string if unable to extract
func (p *Packet) ExtractDatabase() string {
	if len(p.Message) < 21 {
		return ""
	}

	opCode := p.GetOpCode()
	if opCode != 2013 {
		return ""
	}

	// Try to find $db field in BSON document
	// This is a simplified parser - looks for "$db" string followed by string value
	msgStr := string(p.Message)
	idx := strings.Index(msgStr, "$db")
	if idx == -1 {
		return ""
	}

	// Skip "$db\0" and BSON string type (0x02)
	// Then read string length and value
	idx += 4 // Skip "$db\0"
	if idx+4 >= len(p.Message) {
		return ""
	}

	// Read string length (little-endian uint32)
	strLen := binary.LittleEndian.Uint32(p.Message[idx : idx+4])
	idx += 4

	if idx+int(strLen) > len(p.Message) {
		return ""
	}

	// String is null-terminated, so length includes the null byte
	return string(p.Message[idx : idx+int(strLen)-1])
}

// ExtractCollection attempts to extract the collection name from a packet
// Returns empty string if unable to extract
func (p *Packet) ExtractCollection() string {
	cmd := p.ExtractCommandName()
	if cmd == "" {
		return ""
	}

	// The collection name is usually the value of the command field
	// e.g., { insert: "users" } -> collection is "users"
	// But we need to parse BSON to get the value, not just the field name

	// For now, we'll use a simplified approach by looking at the BSON structure
	// This is a heuristic and may not work for all cases

	if len(p.Message) < 30 {
		return ""
	}

	opCode := p.GetOpCode()
	if opCode != 2013 {
		return ""
	}

	offset := 16 + 4 + 1 + 4 + 1 // header + flags + section kind + bson size + element type

	// Skip command name
	for offset < len(p.Message) && p.Message[offset] != 0 {
		offset++
	}
	offset++ // Skip null terminator

	// Now we're at the value of the command field
	// For commands like insert/find/update/delete, this is a BSON string (type 0x02)
	if offset >= len(p.Message) {
		return ""
	}

	// Check if it's a string type (0x02)
	if offset > 0 && p.Message[offset-1-len(cmd)-1] == 0x02 {
		// Read string length
		if offset+4 > len(p.Message) {
			return ""
		}
		strLen := binary.LittleEndian.Uint32(p.Message[offset : offset+4])
		offset += 4

		if offset+int(strLen) > len(p.Message) {
			return ""
		}

		// String is null-terminated
		return string(p.Message[offset : offset+int(strLen)-1])
	}

	return ""
}

// IsInternalDatabase returns true if the database is used for internal MongoDB operations
func IsInternalDatabase(db string) bool {
	internalDatabases := map[string]bool{
		"local":  true, // Replication, oplog
		"admin":  true, // Admin commands (though some user ops go here too)
		"config": true, // Sharding metadata
	}
	return internalDatabases[db]
}

// IsInternalCollection returns true if the collection is used for internal MongoDB operations
func IsInternalCollection(coll string) bool {
	// Collections starting with system. are usually internal
	if strings.HasPrefix(coll, "system.") {
		return true
	}

	internalCollections := map[string]bool{
		"oplog.rs":           true, // Replication oplog
		"startup_log":        true,
		"replset.election":   true,
		"replset.minvalid":   true,
		"replset.oplogTruncateAfterPoint": true,
	}

	return internalCollections[coll]
}

// IsLikelyUserOperation uses heuristics to determine if this is a user operation
// This is smarter than just checking the command name
func (p *Packet) IsLikelyUserOperation() bool {
	cmd := p.ExtractCommandName()
	if cmd == "" {
		return false
	}

	// Check database and collection context for all operations
	db := p.ExtractDatabase()
	coll := p.ExtractCollection()

	// Even write operations can be internal on system collections
	// (e.g., insert/update/delete on system.sessions)
	// So we check context for ALL commands

	// Likely user operations (but check context)
	likelyUserOps := map[string]bool{
		"insert":        true,
		"update":        true,
		"delete":        true,
		"findAndModify": true,
		"create":        true,
		"drop":          true,
		"createIndexes": true,
		"dropIndexes":   true,
	}

	if likelyUserOps[cmd] {
		// Internal database + system collection = internal operation
		if IsInternalDatabase(db) && IsInternalCollection(coll) {
			return false
		}
		// Operations on internal databases are suspicious
		// Even writes to admin/local/config are often internal
		if IsInternalDatabase(db) {
			return false
		}
		return true
	}

	// Definitely internal
	definiteInternalOps := map[string]bool{
		"hello":               true,
		"isMaster":            true,
		"ping":                true,
		"buildInfo":           true,
		"replSetHeartbeat":    true,
		"replSetGetStatus":    true,
		"replSetUpdatePosition": true,
	}

	if definiteInternalOps[cmd] {
		return false
	}

	// Ambiguous commands - use database/collection context
	ambiguousOps := map[string]bool{
		"find":             true, // User queries OR driver/monitoring discovery
		"aggregate":        true, // User pipelines OR Atlas metrics collection
		"count":            true, // User count OR monitoring
		"distinct":         true, // User distinct OR monitoring
		"getMore":          true, // User cursor continuation OR oplog tailing
		"listIndexes":      true, // User query OR driver discovery
		"listCollections":  true, // User query OR driver discovery
		"listDatabases":    true, // User query OR monitoring
	}

	if ambiguousOps[cmd] {
		// Special case: getMore on oplog is ALWAYS replication
		if cmd == "getMore" && db == "local" && coll == "oplog.rs" {
			return false
		}

		// Operations on internal databases are likely internal
		if IsInternalDatabase(db) {
			// Check collection - system collections are definitely internal
			if IsInternalCollection(coll) {
				return false
			}
			// On internal DB but not a system collection - could be either
			// Be conservative: only allow if it's a clear user operation
			// For now, reject (user operations rarely target admin/local/config)
			return false
		}

		// On user database - likely user operation
		return true
	}

	// Unknown command - be conservative and exclude it
	return false
}
