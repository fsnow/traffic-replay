package reader

// ExtractCommandName extracts the MongoDB command name from a packet's message
// Works for OP_MSG (2013) messages by reading the first BSON field name
// Returns empty string if unable to extract
func (p *Packet) ExtractCommandName() string {
	if len(p.Message) < 21 {
		return ""
	}

	opCode := p.GetOpCode()
	if opCode != 2013 { // Only works for OP_MSG
		return ""
	}

	// OP_MSG format:
	// - Wire header: 16 bytes (already parsed)
	// - Flags: 4 bytes
	// - Section kind: 1 byte (kind 0 = body)
	// - BSON document: size(4) + type(1) + name(null-terminated) + ...

	offset := 16 + 4 // Skip wire header + flags

	// Check section kind (must be 0 for body)
	if p.Message[offset] != 0 {
		return ""
	}
	offset++

	// Skip BSON document size
	if offset+4 > len(p.Message) {
		return ""
	}
	offset += 4

	// Read element type (we don't validate it)
	if offset >= len(p.Message) {
		return ""
	}
	offset++

	// Read element name (null-terminated string)
	nameStart := offset
	for offset < len(p.Message) && p.Message[offset] != 0 {
		offset++
	}

	if offset >= len(p.Message) {
		return ""
	}

	return string(p.Message[nameStart:offset])
}

// IsUserOperation returns true if this packet contains a user-initiated operation
// (as opposed to internal cluster operations)
func (p *Packet) IsUserOperation() bool {
	cmd := p.ExtractCommandName()

	// User data operations
	userDataOps := map[string]bool{
		"insert":        true,
		"update":        true,
		"delete":        true,
		"find":          true,
		"findAndModify": true,
		"aggregate":     true,
		"count":         true,
		"distinct":      true,
	}

	// DDL operations
	ddlOps := map[string]bool{
		"create":        true,
		"drop":          true,
		"createIndexes": true,
		"dropIndexes":   true,
		"listIndexes":   true,
		"collMod":       true,
		"renameCollection": true,
	}

	// Admin operations that users might issue
	adminOps := map[string]bool{
		"explain":       true,
		"validate":      true,
		"compact":       true,
		"reIndex":       true,
	}

	return userDataOps[cmd] || ddlOps[cmd] || adminOps[cmd]
}

// IsInternalOperation returns true if this is internal cluster chatter
func (p *Packet) IsInternalOperation() bool {
	cmd := p.ExtractCommandName()

	// Replication operations
	replicationOps := map[string]bool{
		"replSetHeartbeat":    true,
		"replSetGetStatus":    true,
		"replSetGetConfig":    true,
		"replSetUpdatePosition": true,
		"getMore":             true, // Usually oplog tailing
	}

	// Health checks and monitoring
	monitoringOps := map[string]bool{
		"hello":            true,
		"isMaster":         true, // Deprecated name for hello
		"ping":             true,
		"buildInfo":        true,
		"serverStatus":     true,
		"replSetGetStatus": true,
	}

	// Internal coordination
	internalOps := map[string]bool{
		"_configsvrCommitChunkMigration": true,
		"_configsvrCommitChunkSplit":     true,
		"_shardsvrCloneCatalogData":      true,
		"_flushRoutingTableCacheUpdates": true,
	}

	return replicationOps[cmd] || monitoringOps[cmd] || internalOps[cmd]
}

// GetCommandCategory returns a human-readable category for the command
func (p *Packet) GetCommandCategory() string {
	cmd := p.ExtractCommandName()

	if cmd == "" {
		if p.GetOpCode() == 2004 {
			return "legacy-query"
		} else if p.GetOpCode() == 1 {
			return "legacy-reply"
		}
		return "unknown"
	}

	// Categorize commands
	categories := map[string]string{
		// Data operations
		"insert":        "crud",
		"update":        "crud",
		"delete":        "crud",
		"find":          "read",
		"findAndModify": "crud",
		"aggregate":     "read",
		"count":         "read",
		"distinct":      "read",
		"getMore":       "read-continuation",

		// DDL
		"create":           "ddl",
		"drop":             "ddl",
		"createIndexes":    "ddl",
		"dropIndexes":      "ddl",
		"listIndexes":      "ddl",
		"collMod":          "ddl",
		"renameCollection": "ddl",

		// Health/monitoring
		"hello":       "health-check",
		"isMaster":    "health-check",
		"ping":        "health-check",
		"buildInfo":   "info",

		// Replication
		"replSetHeartbeat":       "replication",
		"replSetGetStatus":       "replication",
		"replSetUpdatePosition":  "replication",

		// Admin
		"getParameter":   "admin",
		"setParameter":   "admin",
		"shutdown":       "admin",
		"killOp":         "admin",
		"currentOp":      "admin",

		// Recording control
		"startRecordingTraffic": "recording-control",
		"stopRecordingTraffic":  "recording-control",
	}

	if category, ok := categories[cmd]; ok {
		return category
	}

	return "other"
}
