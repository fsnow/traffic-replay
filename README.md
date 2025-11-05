# Traffic-Replay

A replayer tool for MongoDB's traffic recording feature.

## Project Status

**Phase 1 (MVP) - In Progress**

### Completed
- ✅ Go module setup
- ✅ Project structure
- ✅ Binary packet parser (`pkg/reader/packet.go`)
- ✅ Recording file reader (`pkg/reader/recording.go`)
- ✅ Comprehensive tests
- ✅ Validated against real MongoDB 8.0 recording (5041 packets)

### In Progress
- Wire message sender using MongoDB Go driver
- Basic CLI implementation

## Quick Start

### Prerequisites
- Go 1.21+
- MongoDB 7.0+ (for target replay server)

### Installation

```bash
go install github.com/fsnow/traffic-replay/cmd/traffic-replay@latest
```

### Development

```bash
# Clone the repository
git clone https://github.com/fsnow/traffic-replay.git
cd traffic-replay

# Run tests
go test ./...

# Run tests with verbose output
go test -v ./pkg/reader

# Test against real recording file
go test -v ./pkg/reader -run TestRealRecording
```

## Documentation

See the [`docs/`](docs/) directory for comprehensive documentation:

### Project Documentation
- [`docs/design.md`](docs/design.md) - Main design document and architecture
- [`docs/research-summary.md`](docs/research-summary.md) - Research phase summary
- [`docs/filtering.md`](docs/filtering.md) - Guide to filtering recordings (99%+ reduction!)
- [`docs/command-ambiguities.md`](docs/command-ambiguities.md) - MongoDB command interpretation guide for consulting engineers

### Technical Reference
- [`docs/reference/`](docs/reference/) - Technical reference documentation
  - MongoDB traffic recording format
  - Wire protocol analysis
  - Go driver integration
  - Mongoreplay analysis

## Binary Format

MongoDB traffic recordings use the following binary format:

```
┌──────────────────────────────────────────────────────────┐
│                    Packet Header                          │
├──────────────────────────────────────────────────────────┤
│ size          : uint32 LE (4 bytes)  - Total packet size │
│ id            : uint64 LE (8 bytes)  - Session ID        │
│ session       : string   (variable)  - Session metadata  │
│ offset        : uint64 LE (8 bytes)  - Microseconds      │
│ order         : uint64 LE (8 bytes)  - Sequence number   │
├──────────────────────────────────────────────────────────┤
│                    Message Data                           │
│             (Wire protocol message bytes)                 │
└──────────────────────────────────────────────────────────┘
```

## Available Tools

### Analysis Tools

**analyze** - High-level recording analysis
```bash
go run cmd/analyze/main.go recording.bin
# Shows: packet counts, opcodes, commands, sessions, duration
```

**analyze-detailed** - Detailed operation breakdown
```bash
go run cmd/analyze-detailed/main.go recording.bin
# Shows: operation counts with percentages
# Special: getMore breakdown by database and collection
```

**packets** - Low-level packet inspection
```bash
go run cmd/packets/main.go recording.bin
# Shows: detailed packet structure, hex dumps, BSON parsing
```

### Filtering and Transformation

**filter** - Remove internal operations and reduce file size
```bash
# Smart context-aware filtering (99%+ reduction)
go run cmd/filter/main.go -input recording.bin -output filtered.bin \
  -user-ops-smart -requests-only

# CRUD only
go run cmd/filter/main.go -input recording.bin -output crud.bin \
  -include-commands insert,update,delete,find -requests-only

# Time-based filtering
go run cmd/filter/main.go -input recording.bin -output first-100ms.bin \
  -max-offset 100000
```

**script-gen** - Generate mongosh replay script
```bash
# Generate full replay script (uses db.getSiblingDB() for explicit database names)
go run cmd/script-gen/main.go recording.bin --requests-only > replay.js

# Generate CRUD-only script
go run cmd/script-gen/main.go recording.bin --crud-only --requests-only > crud.js

# Output format:
#   db.getSiblingDB("traffictest").users.insertOne({...});
#   db.getSiblingDB("traffictest").orders.find({...});
#   db.getSiblingDB("admin").runCommand({hello: 1});

# Then manually replay:
mongosh mongodb://localhost:27017 < replay.js
```

See [`docs/filtering.md`](docs/filtering.md) for detailed filtering guide.

## Features (Planned)

### Phase 1 (MVP)
- [x] Parse MongoDB traffic recording files
- [x] Read and validate packet structure
- [x] Command extraction from OP_MSG packets
- [x] Filter recordings (remove 99% of cluster chatter)
- [x] Analysis and inspection tools
- [x] Script generation for manual replay
- [ ] Send wire protocol messages to MongoDB
- [ ] Fast-forward replay mode
- [ ] Basic CLI

### Phase 2 (Production-Ready)
- [ ] Time-scaled replay with configurable speed multiplier
- [ ] Session management with goroutines
- [ ] Statistics and progress reporting
- [ ] Connection lifecycle handling

### Phase 3 (Advanced)
- [ ] Response validation mode
- [ ] OpCode translation (legacy → OP_MSG)
- [ ] Filter and split recordings
- [ ] Parallel replay support

## Contributing

This project is in active development. See [`docs/design.md`](docs/design.md) for architecture and implementation details.

## License

TBD

## Acknowledgments

Built as a replacement for the deprecated [Mongoreplay](https://github.com/mongodb-labs/mongoreplay) tool, designed to work with MongoDB's server-side traffic recording feature.
