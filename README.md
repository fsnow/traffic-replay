# Traffic-Replay

A replayer tool for MongoDB's traffic recording feature.

Captures, analyzes, filters, and replays MongoDB traffic recordings created with the `startRecordingTraffic` server command. Designed as a modern replacement for the deprecated [Mongoreplay](https://github.com/mongodb-labs/mongoreplay) tool.

## Project Status

**Phase 1 (MVP) - In Progress**

### Completed
- ✅ Go module setup and project structure
- ✅ Binary packet parser (`pkg/reader/packet.go`)
- ✅ Recording file reader (`pkg/reader/recording.go`)
- ✅ Command extraction from OP_MSG packets (`pkg/reader/commands.go`)
- ✅ Context-aware filtering (`pkg/reader/context.go`)
- ✅ Comprehensive analysis tools (`cmd/analyze/`, `cmd/analyze-detailed/`, `cmd/packets/`)
- ✅ Smart filtering tool (`cmd/filter/`)
- ✅ Script generator for manual replay (`cmd/script-gen/`)
- ✅ Wire message sender (`pkg/sender/`)
- ✅ Automated replay engine (`cmd/replay/`)
- ✅ Validated against MongoDB 8.0 with 56 operation types
- ✅ Test coverage: 70+ operation types including advanced features

### In Progress
- Advanced replay features (time-scaled replay, session management)
- Statistics and progress reporting

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

## Example Workflow

### 1. Record MongoDB Traffic

```bash
# Start recording (on MongoDB server)
mongosh mongodb://localhost:27017/admin --eval "
  db.runCommand({
    startRecordingTraffic: 1,
    filename: 'myapp-traffic.txt',
    bufferSize: NumberLong('100000000')
  })
"

# Run your application or test workload...

# Stop recording
mongosh mongodb://localhost:27017/admin --eval "
  db.adminCommand({ stopRecordingTraffic: 1 })
"
```

### 2. Analyze and Filter

```bash
# Quick analysis
go run cmd/analyze/main.go ~/mongodb-data/myapp-traffic.txt

# Detailed breakdown
go run cmd/analyze-detailed/main.go ~/mongodb-data/myapp-traffic.txt

# Filter to user operations only (99%+ reduction)
go run cmd/filter/main.go \
  -input ~/mongodb-data/myapp-traffic.txt \
  -output filtered-ops.bin \
  -user-ops-smart -requests-only
```

### 3. Replay Traffic

**Option A: Automated Replay (Raw Mode - Default)**

Raw mode sends exact wire protocol bytes for precise replay:

```bash
# Raw mode: exact wire protocol replay with original timing (default)
go run cmd/replay/main.go filtered-ops.bin mongodb://test-cluster:27017 \
  --requests-only --user-ops

# Fast-forward mode (no timing delays)
go run cmd/replay/main.go filtered-ops.bin mongodb://test-cluster:27017 \
  --speed 0 --requests-only --user-ops

# 2x speed replay
go run cmd/replay/main.go filtered-ops.bin mongodb://test-cluster:27017 \
  --speed 2.0 --requests-only --user-ops

# Dry run to validate wire messages
go run cmd/replay/main.go filtered-ops.bin mongodb://test-cluster:27017 \
  --dry-run --requests-only

# Replay with limit
go run cmd/replay/main.go filtered-ops.bin mongodb://test-cluster:27017 \
  --requests-only --limit 100
```

**Option A (Alternative): Automated Replay (Command Mode)**

Command mode parses and re-executes operations via RunCommand:

```bash
# Command mode: semantic replay via RunCommand
go run cmd/replay/main.go filtered-ops.bin mongodb://test-cluster:27017 \
  --mode command --requests-only --user-ops

# Dry run in command mode
go run cmd/replay/main.go filtered-ops.bin mongodb://test-cluster:27017 \
  --mode command --dry-run --requests-only
```

**Option B: Manual Replay with Script**

```bash
# Generate executable mongosh script
go run cmd/script-gen/main.go filtered-ops.bin --requests-only > replay.js

# Verify the script
head -50 replay.js

# Replay against test environment
mongosh mongodb://test-cluster:27017 < replay.js
```

See [`recordings/README.md`](recordings/README.md) for comprehensive workflow documentation.

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

### Replay Tools

**replay** - Automated replay of recorded traffic

Two replay modes available:
- **Raw mode** (default): Sends exact wire protocol bytes for precise replay
- **Command mode**: Parses and re-executes operations via RunCommand

Speed control:
- **1.0x** (default): Preserves original timing between operations
- **0**: Fast-forward mode (no delays)
- **2.0**: 2x speed, **0.5**: half speed, etc.

```bash
# Raw mode with original timing (default)
go run cmd/replay/main.go recording.bin mongodb://localhost:27017 \
  --requests-only --user-ops

# Fast-forward mode (no delays)
go run cmd/replay/main.go recording.bin mongodb://localhost:27017 \
  --speed 0 --requests-only --user-ops

# Command mode: semantic replay
go run cmd/replay/main.go recording.bin mongodb://localhost:27017 \
  --mode command --requests-only --user-ops

# Dry run mode (validate without sending)
go run cmd/replay/main.go recording.bin mongodb://localhost:27017 \
  --dry-run --requests-only

# Limit number of operations
go run cmd/replay/main.go recording.bin mongodb://localhost:27017 \
  --requests-only --limit 100
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

## Test Recording

A comprehensive test recording is included in [`recordings/`](recordings/):

- **56 validated operations** covering 70+ operation types
- **Comprehensive coverage**: Updates ($inc, $mul, $min, $max, $unset, $rename, array ops), finds (projection, sort, skip, complex queries), aggregation ($lookup, $bucket, $facet), indexes (unique, compound, partial, TTL, text), findAndModify, bulk operations, and more
- **Generated script**: [`recordings/expected-operations.js`](recordings/expected-operations.js) - validated replay script
- **Test generator**: [`recordings/generate-test-operations.js`](recordings/generate-test-operations.js) - rerun to create new recordings

See [`recordings/README.md`](recordings/README.md) for complete test coverage details.

## Features

### Phase 1 (MVP) - Current Status
- [x] Parse MongoDB traffic recording files
- [x] Read and validate packet structure
- [x] Command extraction from OP_MSG packets
- [x] Context-aware operation classification (distinguish user vs internal operations)
- [x] Smart filtering (remove 99% of cluster chatter)
- [x] Analysis tools (summary, detailed breakdown, packet inspection)
- [x] Script generation for manual replay (with insertMany/insertOne, replaceOne/updateOne detection)
- [x] Internal field cleaning ($clusterTime, lsid, txnNumber)
- [x] Raw wire protocol sender using MongoDB Go driver (`pkg/sender/raw_sender.go`)
- [x] Command-based sender using RunCommand (`pkg/sender/sender.go`)
- [x] Dual-mode automated replay engine (`cmd/replay/`) with raw (default) and command modes
- [x] Basic CLI with filtering and replay options
- [x] Timing-based replay with speed multiplier (1x default, 0 for fast-forward)
- [x] Speed control (--speed flag: 0 = fast-forward, 1.0 = original timing, 2.0 = 2x speed, etc.)

### Phase 2 (Production-Ready)
- [ ] Session management with goroutines (parallel replay per session)
- [ ] Enhanced statistics and progress reporting
- [ ] Connection lifecycle handling
- [ ] Response validation mode

### Phase 3 (Advanced)
- [ ] OpCode translation (legacy → OP_MSG)
- [ ] Filter and split recordings
- [ ] Multi-connection parallel replay
- [ ] Real-time recording analysis

## Contributing

This project is in active development. See [`docs/design.md`](docs/design.md) for architecture and implementation details.

## License

TBD

## Acknowledgments

Built as a replacement for the deprecated [Mongoreplay](https://github.com/mongodb-labs/mongoreplay) tool, designed to work with MongoDB's server-side traffic recording feature.
