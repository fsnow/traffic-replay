package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/fsnow/traffic-replay/pkg/reader"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <recording-file>\n", os.Args[0])
		os.Exit(1)
	}

	filePath := os.Args[1]

	rec, err := reader.NewRecordingReader(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening recording: %v\n", err)
		os.Exit(1)
	}
	defer rec.Close()

	// Track operation counts
	opCounts := make(map[string]int)

	// Track getMore by collection (database.collection)
	getMoreByCollection := make(map[string]int)

	// Track getMore by database
	getMoreByDatabase := make(map[string]int)

	totalPackets := 0
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

		// Get command name
		cmd := packet.ExtractCommandName()
		if cmd == "" {
			cmd = fmt.Sprintf("(OpCode %d)", packet.GetOpCode())
		}

		opCounts[cmd]++

		// For getMore, track by collection
		if cmd == "getMore" {
			db := packet.ExtractDatabase()
			coll := packet.ExtractCollection()

			if db != "" {
				getMoreByDatabase[db]++
			}

			if db != "" && coll != "" {
				fullName := db + "." + coll
				getMoreByCollection[fullName]++
			} else if db != "" {
				getMoreByCollection[db+".(unknown)"]++
			} else {
				getMoreByCollection["(unknown)"]++
			}
		}
	}

	// Print results
	fmt.Printf("Total packets: %d\n\n", totalPackets)

	// Sort operations by count
	type opCount struct {
		name  string
		count int
	}
	var ops []opCount
	for name, count := range opCounts {
		ops = append(ops, opCount{name, count})
	}
	sort.Slice(ops, func(i, j int) bool {
		return ops[i].count > ops[j].count
	})

	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("OPERATION COUNTS")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()

	totalOps := 0
	for _, op := range ops {
		totalOps += op.count
	}

	for _, op := range ops {
		pct := float64(op.count) / float64(totalOps) * 100
		fmt.Printf("%-30s %6d  (%5.1f%%)\n", op.name, op.count, pct)
	}

	// Print getMore breakdown
	if len(getMoreByDatabase) > 0 {
		fmt.Println()
		fmt.Println(strings.Repeat("=", 80))
		fmt.Println("getMore BREAKDOWN BY DATABASE")
		fmt.Println(strings.Repeat("=", 80))
		fmt.Println()

		type dbCount struct {
			db    string
			count int
		}
		var dbs []dbCount
		for db, count := range getMoreByDatabase {
			dbs = append(dbs, dbCount{db, count})
		}
		sort.Slice(dbs, func(i, j int) bool {
			return dbs[i].count > dbs[j].count
		})

		totalGetMore := 0
		for _, db := range dbs {
			totalGetMore += db.count
		}

		for _, db := range dbs {
			pct := float64(db.count) / float64(totalGetMore) * 100

			isInternal := ""
			if db.db == "local" || db.db == "admin" || db.db == "config" {
				isInternal = " [INTERNAL]"
			}

			fmt.Printf("%-30s %6d  (%5.1f%%)%s\n", db.db, db.count, pct, isInternal)
		}
	}

	if len(getMoreByCollection) > 0 {
		fmt.Println()
		fmt.Println(strings.Repeat("=", 80))
		fmt.Println("getMore BREAKDOWN BY COLLECTION")
		fmt.Println(strings.Repeat("=", 80))
		fmt.Println()

		type collCount struct {
			coll  string
			count int
		}
		var colls []collCount
		for coll, count := range getMoreByCollection {
			colls = append(colls, collCount{coll, count})
		}
		sort.Slice(colls, func(i, j int) bool {
			return colls[i].count > colls[j].count
		})

		totalGetMore := 0
		for _, coll := range colls {
			totalGetMore += coll.count
		}

		for _, coll := range colls {
			pct := float64(coll.count) / float64(totalGetMore) * 100

			isInternal := ""
			if strings.HasPrefix(coll.coll, "local.") {
				isInternal = " [REPLICATION]"
			} else if strings.HasPrefix(coll.coll, "admin.") || strings.HasPrefix(coll.coll, "config.") {
				isInternal = " [INTERNAL]"
			}

			fmt.Printf("%-50s %6d  (%5.1f%%)%s\n", coll.coll, coll.count, pct, isInternal)
		}

		fmt.Println()
		fmt.Println("Note: Collections marked [REPLICATION] are oplog tailing (secondaries replicating).")
		fmt.Println("      Collections marked [INTERNAL] are cluster coordination.")
	}
}
