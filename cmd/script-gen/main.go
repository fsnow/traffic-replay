package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fsnow/traffic-replay/pkg/reader"
	"go.mongodb.org/mongo-driver/bson"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <recording-file> [--crud-only] [--requests-only]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		fmt.Fprintf(os.Stderr, "  --crud-only       Only output CRUD operations (insert/update/delete/find)\n")
		fmt.Fprintf(os.Stderr, "  --requests-only   Only output requests (exclude responses)\n")
		os.Exit(1)
	}

	filePath := os.Args[1]
	crudOnly := false
	requestsOnly := false

	for i := 2; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--crud-only":
			crudOnly = true
		case "--requests-only":
			requestsOnly = true
		}
	}

	rec, err := reader.NewRecordingReader(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening recording: %v\n", err)
		os.Exit(1)
	}
	defer rec.Close()

	fmt.Println("// Generated from:", filePath)
	fmt.Println("// MongoDB operations replay script")
	fmt.Println("// Each operation explicitly specifies the database")
	fmt.Println()

	var operations []string
	var unknownOps []string

	totalPackets := 0
	outputPackets := 0

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
			continue
		}

		cmd := packet.ExtractCommandName()
		if cmd == "" {
			continue
		}

		// Apply CRUD filter
		if crudOnly {
			crudOps := map[string]bool{
				"insert": true, "update": true, "delete": true,
				"find": true, "findAndModify": true,
			}
			if !crudOps[cmd] {
				continue
			}
		}

		// Extract database and generate script
		db := packet.ExtractDatabase()
		if db == "" {
			db = "unknown"
		}

		script, err := generateScript(packet, cmd, db)
		if err != nil {
			// If we can't parse it, just note it
			unknownOps = append(unknownOps, fmt.Sprintf("// Packet %d: %s (parse error: %v)", totalPackets, cmd, err))
			continue
		}

		if script != "" {
			operations = append(operations, script)
			outputPackets++
		}
	}

	// Print all operations
	for _, op := range operations {
		fmt.Println(op)
		fmt.Println()
	}

	// Print operations we couldn't parse
	if len(unknownOps) > 0 {
		fmt.Println("// Operations that could not be parsed:")
		for _, op := range unknownOps {
			fmt.Println(op)
		}
	}

	// Print summary
	fmt.Fprintf(os.Stderr, "\nGenerated script from %d packets (%d operations)\n", totalPackets, outputPackets)
}

func generateScript(packet *reader.Packet, cmd string, db string) (string, error) {
	// Extract the BSON document from the OP_MSG packet
	opCode := packet.GetOpCode()
	if opCode != 2013 { // OP_MSG
		return "", fmt.Errorf("unsupported opcode: %d", opCode)
	}

	// OP_MSG structure:
	// Header: 16 bytes (already part of Message)
	// Flags: 4 bytes
	// Sections: variable
	//   Section kind: 1 byte
	//   For kind 0: BSON document

	if len(packet.Message) < 16+4+1+4 {
		return "", fmt.Errorf("packet too short")
	}

	// Skip to BSON document (after header + flags + section kind)
	offset := 16 + 4 + 1

	// Read BSON document
	bsonDoc := packet.Message[offset:]

	// Parse BSON to map
	var doc bson.M
	err := bson.Unmarshal(bsonDoc, &doc)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	// Generate script based on command type
	switch cmd {
	case "insert":
		return generateInsert(doc, db)
	case "update":
		return generateUpdate(doc, db)
	case "delete":
		return generateDelete(doc, db)
	case "find":
		return generateFind(doc, db)
	case "aggregate":
		return generateAggregate(doc, db)
	case "findAndModify":
		return generateFindAndModify(doc, db)
	case "createIndexes":
		return generateCreateIndexes(doc, db)
	case "dropIndexes":
		return generateDropIndexes(doc, db)
	case "create":
		return generateCreate(doc, db)
	case "drop":
		return generateDrop(doc, db)
	default:
		// For other commands, just output as runCommand
		return generateRunCommand(doc, cmd, db)
	}
}

func generateInsert(doc bson.M, database string) (string, error) {
	coll, ok := doc["insert"].(string)
	if !ok {
		return "", fmt.Errorf("missing collection name")
	}

	documents, ok := doc["documents"].(bson.A)
	if !ok {
		return "", fmt.Errorf("missing documents array")
	}

	var lines []string
	for _, d := range documents {
		jsonBytes, err := json.MarshalIndent(d, "", "  ")
		if err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("db.getSiblingDB(\"%s\").%s.insertOne(%s);", database, coll, string(jsonBytes)))
	}

	return strings.Join(lines, "\n"), nil
}

func generateUpdate(doc bson.M, database string) (string, error) {
	coll, ok := doc["update"].(string)
	if !ok {
		return "", fmt.Errorf("missing collection name")
	}

	updates, ok := doc["updates"].(bson.A)
	if !ok {
		return "", fmt.Errorf("missing updates array")
	}

	var lines []string
	for _, u := range updates {
		update, ok := u.(bson.M)
		if !ok {
			continue
		}

		filter := update["q"]
		updateDoc := update["u"]
		multi := update["multi"]

		filterJSON, _ := json.MarshalIndent(filter, "", "  ")
		updateJSON, _ := json.MarshalIndent(updateDoc, "", "  ")

		if multi == true {
			lines = append(lines, fmt.Sprintf("db.getSiblingDB(\"%s\").%s.updateMany(\n  %s,\n  %s\n);",
				database, coll, string(filterJSON), string(updateJSON)))
		} else {
			lines = append(lines, fmt.Sprintf("db.getSiblingDB(\"%s\").%s.updateOne(\n  %s,\n  %s\n);",
				database, coll, string(filterJSON), string(updateJSON)))
		}
	}

	return strings.Join(lines, "\n"), nil
}

func generateDelete(doc bson.M, database string) (string, error) {
	coll, ok := doc["delete"].(string)
	if !ok {
		return "", fmt.Errorf("missing collection name")
	}

	deletes, ok := doc["deletes"].(bson.A)
	if !ok {
		return "", fmt.Errorf("missing deletes array")
	}

	var lines []string
	for _, d := range deletes {
		del, ok := d.(bson.M)
		if !ok {
			continue
		}

		filter := del["q"]
		limit := del["limit"]

		filterJSON, _ := json.MarshalIndent(filter, "", "  ")

		// limit: 0 = deleteMany, limit: 1 = deleteOne
		if limit == int32(1) || limit == int64(1) {
			lines = append(lines, fmt.Sprintf("db.getSiblingDB(\"%s\").%s.deleteOne(%s);", database, coll, string(filterJSON)))
		} else {
			lines = append(lines, fmt.Sprintf("db.getSiblingDB(\"%s\").%s.deleteMany(%s);", database, coll, string(filterJSON)))
		}
	}

	return strings.Join(lines, "\n"), nil
}

func generateFind(doc bson.M, database string) (string, error) {
	coll, ok := doc["find"].(string)
	if !ok {
		return "", fmt.Errorf("missing collection name")
	}

	filter := doc["filter"]
	if filter == nil {
		filter = bson.M{}
	}

	filterJSON, _ := json.MarshalIndent(filter, "", "  ")

	// Add projection if present
	if projection, ok := doc["projection"].(bson.M); ok && len(projection) > 0 {
		projJSON, _ := json.MarshalIndent(projection, "", "  ")
		return fmt.Sprintf("db.getSiblingDB(\"%s\").%s.find(\n  %s\n).project(%s);", database, coll, string(filterJSON), string(projJSON)), nil
	}

	// Add sort if present
	if sort, ok := doc["sort"].(bson.M); ok && len(sort) > 0 {
		sortJSON, _ := json.MarshalIndent(sort, "", "  ")
		return fmt.Sprintf("db.getSiblingDB(\"%s\").%s.find(\n  %s\n).sort(%s);", database, coll, string(filterJSON), string(sortJSON)), nil
	}

	// Add limit if present
	if limit, ok := doc["limit"]; ok {
		return fmt.Sprintf("db.getSiblingDB(\"%s\").%s.find(\n  %s\n).limit(%v);", database, coll, string(filterJSON), limit), nil
	}

	return fmt.Sprintf("db.getSiblingDB(\"%s\").%s.find(%s);", database, coll, string(filterJSON)), nil
}

func generateAggregate(doc bson.M, database string) (string, error) {
	coll, ok := doc["aggregate"].(string)
	if !ok {
		return "", fmt.Errorf("missing collection name")
	}

	pipeline, ok := doc["pipeline"].(bson.A)
	if !ok {
		return "", fmt.Errorf("missing pipeline")
	}

	pipelineJSON, _ := json.MarshalIndent(pipeline, "", "  ")

	return fmt.Sprintf("db.getSiblingDB(\"%s\").%s.aggregate(%s);", database, coll, string(pipelineJSON)), nil
}

func generateFindAndModify(doc bson.M, database string) (string, error) {
	coll, ok := doc["findAndModify"].(string)
	if !ok {
		return "", fmt.Errorf("missing collection name")
	}

	// Remove the collection name from the document for runCommand
	delete(doc, "findAndModify")

	argsJSON, _ := json.MarshalIndent(doc, "", "  ")

	return fmt.Sprintf("db.getSiblingDB(\"%s\").%s.findAndModify(%s);", database, coll, string(argsJSON)), nil
}

func generateCreateIndexes(doc bson.M, database string) (string, error) {
	coll, ok := doc["createIndexes"].(string)
	if !ok {
		return "", fmt.Errorf("missing collection name")
	}

	indexes, ok := doc["indexes"].(bson.A)
	if !ok {
		return "", fmt.Errorf("missing indexes array")
	}

	var lines []string
	for _, idx := range indexes {
		indexDoc, ok := idx.(bson.M)
		if !ok {
			continue
		}

		key := indexDoc["key"]
		keyJSON, _ := json.MarshalIndent(key, "", "  ")

		// Build options
		options := bson.M{}
		if name, ok := indexDoc["name"].(string); ok {
			options["name"] = name
		}
		if unique, ok := indexDoc["unique"].(bool); ok && unique {
			options["unique"] = true
		}

		if len(options) > 0 {
			optJSON, _ := json.MarshalIndent(options, "", "  ")
			lines = append(lines, fmt.Sprintf("db.getSiblingDB(\"%s\").%s.createIndex(%s, %s);", database, coll, string(keyJSON), string(optJSON)))
		} else {
			lines = append(lines, fmt.Sprintf("db.getSiblingDB(\"%s\").%s.createIndex(%s);", database, coll, string(keyJSON)))
		}
	}

	return strings.Join(lines, "\n"), nil
}

func generateDropIndexes(doc bson.M, database string) (string, error) {
	coll, ok := doc["dropIndexes"].(string)
	if !ok {
		return "", fmt.Errorf("missing collection name")
	}

	index := doc["index"]
	indexJSON, _ := json.Marshal(index)

	return fmt.Sprintf("db.getSiblingDB(\"%s\").%s.dropIndex(%s);", database, coll, string(indexJSON)), nil
}

func generateCreate(doc bson.M, database string) (string, error) {
	coll, ok := doc["create"].(string)
	if !ok {
		return "", fmt.Errorf("missing collection name")
	}

	return fmt.Sprintf("db.getSiblingDB(\"%s\").createCollection(\"%s\");", database, coll), nil
}

func generateDrop(doc bson.M, database string) (string, error) {
	coll, ok := doc["drop"].(string)
	if !ok {
		return "", fmt.Errorf("missing collection name")
	}

	return fmt.Sprintf("db.getSiblingDB(\"%s\").%s.drop();", database, coll), nil
}

func generateRunCommand(doc bson.M, cmd string, database string) (string, error) {
	docJSON, _ := json.MarshalIndent(doc, "", "  ")
	return fmt.Sprintf("db.getSiblingDB(\"%s\").runCommand(%s);", database, string(docJSON)), nil
}
