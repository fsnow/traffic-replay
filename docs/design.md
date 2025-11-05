# Traffic-Replay Design Document

## Project Overview

**Goal:** Build a replayer tool for MongoDB's traffic recording feature.

**Status:** Design Phase

**Last Updated:** 2025-10-31

## Table of Contents

1. [Background and Context](#background-and-context)
2. [Requirements](#requirements)
   - [Functional Requirements](#functional-requirements)
   - [Non-Functional Requirements](#non-functional-requirements)
3. [Architecture](#architecture)
   - [Component Structure](#component-structure)
   - [Data Flow](#data-flow)
4. [Binary File Format Reference](#binary-file-format-reference)
5. [Design Decisions](#design-decisions)
   - [Programming Language](#1-programming-language)
   - [Timing Control](#2-timing-control)
   - [Connection Strategy](#3-connection-strategy)
   - [Wire Protocol Handling](#4-wire-protocol-handling)
   - [Authentication Handling](#5-authentication-handling)
   - [Database/Collection Targeting](#6-databasecollection-targeting)
   - [Response Handling](#7-response-handling)
6. [Connection Management Architecture](#connection-management-architecture)
7. [CLI Interface Design](#cli-interface-design)
8. [Open Questions](#open-questions)
9. [Implementation Phases](#implementation-phases)
10. [Implementation Roadmap](#implementation-roadmap)
11. [MongoDB Version Compatibility & Test Matrix](#mongodb-version-compatibility--test-matrix)
12. [References](#references)
13. [Notes and Ideas](#notes-and-ideas)
14. [Decision Log](#decision-log)
15. [UPDATE: Mongoreplay Analysis (2025-10-31)](#update-mongoreplay-analysis-2025-10-31)
16. [FINAL DESIGN DECISIONS (2025-11-04)](#final-design-decisions-2025-11-04)

---

## Background and Context

### History

1. **Mongoreplay** (Original Tool)
   - GitHub: https://github.com/mongodb-labs/mongoreplay
   - Built using MGo driver (before official MongoDB Go driver)
   - Had its own network packet capture feature
   - Could also replay externally recorded packet data
   - **Fatal Flaw:** Used network packet capture, which doesn't work with TLS encryption
   - Became unmaintained and incompatible with newer MongoDB versions
   - Deprecated as TLS became default for both on-prem and Atlas deployments

2. **Traffic Recording**
   - Implemented in MongoDB Server to replace Mongoreplay
   - Captures traffic at the transport layer **inside** the server
   - **Key Advantage:** Works with TLS because it captures after decryption
   - Fully functional for capture (see `mongodb-traffic-capture-guide.md`)
   - **Missing Piece:** Replayer was never implemented

3. **This Project: Traffic-Replay**
   - Complete the traffic recording feature by building the replayer component
   - Leverage lessons learned from Mongoreplay
   - Use modern Go driver and best practices

---

## Requirements

### Functional Requirements

1. **Recording File Support**
   - Read traffic recording binary recording files (`.bin` format)
   - Parse packet headers and wire protocol messages
   - Support multiple files in a recording directory
   - Verify CRC32C checksums

2. **Replay Capabilities**
   - Replay wire protocol messages to target MongoDB instance
   - Handle session lifecycle (start/end events)
   - Support two replay timing modes:
     - **Fast-forward**: Replay as fast as possible (ignore timing)
     - **Time-scaled**: Respect original timing with configurable speed multiplier (default 1.0x)
       - 1.0x = real-time (original speed)
       - 0.5x = half speed (slower)
       - 2.0x, 10.0x = faster than original

3. **Connection Management** (Hybrid Approach)
   - Use driver's connection pooling for connection lifecycle (auth, TLS, health checks)
   - Access driver's internal APIs to write raw wire protocol bytes
   - **NOT using high-level driver APIs** (no `collection.InsertOne()`, etc.)
   - Send raw wire message bytes from recordings via `connection.Write()`
   - Handle multiple concurrent sessions (one connection per recorded session)
   - Benefits:
     - ✅ Driver handles: auth (SCRAM/x.509/LDAP), TLS, compression, reconnection
     - ✅ We control: exact wire message replay from recordings

4. **Response Handling**
   - **Note**: Traffic recordings include BOTH requests and responses (captured at transport layer)
   - Distinguish via wire protocol header: `responseTo == 0` = request, `responseTo != 0` = response
   - **Primary mode**: Fire-and-forget (send requests only, ignore responses in recording and from target)
   - **Optional modes**:
     - **Validate**: Send requests, read target responses, check for errors (ignore recorded responses)
     - **Verify**: Send requests, read target responses, semantically compare to recorded responses
       - Not byte-for-byte comparison (timestamps, IDs, etc. will differ)
       - Compare document structure, counts, and non-temporal values

5. **Monitoring and Statistics**
   - Progress reporting
   - Statistics (packets replayed, errors, timing, etc.)
   - Configurable stats output interval

### Non-Functional Requirements

1. **Performance**
   - Handle high-throughput recordings
   - Minimal memory footprint
   - Efficient binary file parsing

2. **Usability**
   - Clear CLI interface
   - Helpful error messages
   - Configurable via flags and/or config file

3. **Reliability**
   - Graceful error handling
   - Resume capability (optional)
   - Proper cleanup on shutdown

---

## Architecture

### Component Structure

```
traffic-replay/
├── cmd/
│   └── traffic-replay/
│       └── main.go              # CLI entry point
├── pkg/
│   ├── reader/
│   │   ├── packet.go            # Packet struct and binary parsing
│   │   ├── recording.go         # RecordingReader (single file)
│   │   └── iterator.go          # RecordingSetIterator (directory)
│   ├── session/
│   │   ├── manager.go           # Session lifecycle management
│   │   ├── pool.go              # Connection pool/strategy
│   │   └── mapper.go            # Map recorded session IDs to connections
│   ├── replay/
│   │   ├── replayer.go          # Main replay orchestration
│   │   ├── scheduler.go         # Timing control (fast-forward, time-scaled)
│   │   └── stats.go             # Statistics and progress tracking
│   ├── wire/
│   │   ├── message.go           # Wire protocol message handling
│   │   └── sender.go            # Send messages via Go driver
│   └── config/
│       └── config.go            # Configuration structures
├── go.mod
├── go.sum
├── README.md
├── DESIGN.md                    # This file
└── mongodb-traffic-capture-guide.md  # traffic recording feature documentation
```

### Data Flow

```
┌──────────────────────────────────────────────────────────────┐
│  Recording Files (*.bin)                                      │
│  /data/recordings/capture-001/                               │
└────────────────┬─────────────────────────────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────────────────────────────┐
│  RecordingSetIterator                                         │
│  • Reads binary packets from files                           │
│  • Parses packet headers                                     │
│  • Yields packets in order                                   │
└────────────────┬─────────────────────────────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────────────────────────────┐
│  Scheduler                                                    │
│  • Controls timing (fast-forward, time-scaled)               │
│  • Sleeps until appropriate replay time                      │
└────────────────┬─────────────────────────────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────────────────────────────┐
│  SessionManager                                               │
│  • Maps recorded session IDs to connections                  │
│  • Manages connection lifecycle                              │
│  • Handles session start/end events                          │
└────────────────┬─────────────────────────────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────────────────────────────┐
│  WireProtocolSender                                           │
│  • Sends wire protocol messages to MongoDB                   │
│  • Handles responses (fire-and-forget, verify, validate)     │
└────────────────┬─────────────────────────────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────────────────────────────┐
│  Target MongoDB Instance                                      │
│  mongodb://localhost:27017                                    │
└──────────────────────────────────────────────────────────────┘
```

---

## Binary File Format Reference

From `mongodb-traffic-capture-guide.md`:

### Packet Structure

```
┌──────────────────────────────────────────────────────────┐
│                    Packet Header                          │
├──────────────────────────────────────────────────────────┤
│ size          : uint32_t (LE)    - Total packet size     │
│ eventType     : uint8_t          - Event type (0/1/2)    │
│ id            : uint64_t (LE)    - Session ID            │
│ session       : null-terminated  - Session metadata BSON │
│ offset        : uint64_t (LE)    - Microseconds from start│
│ order         : uint64_t (LE)    - Sequence number       │
├──────────────────────────────────────────────────────────┤
│                    Message Data                           │
│             (Wire protocol message bytes)                 │
└──────────────────────────────────────────────────────────┘
```

### Event Types

```go
type EventType uint8

const (
    EventTypeRegular      EventType = 0  // Regular message event
    EventTypeSessionStart EventType = 1  // Session started (no message data)
    EventTypeSessionEnd   EventType = 2  // Session ended (no message data)
)
```

### Field Details

| Field | Type | Size | Description |
|-------|------|------|-------------|
| `size` | uint32 LE | 4 bytes | Total packet size including header |
| `eventType` | uint8 | 1 byte | Event type (0/1/2) |
| `id` | uint64 LE | 8 bytes | Session/connection identifier |
| `session` | string | variable | Null-terminated BSON string with session metadata |
| `offset` | uint64 LE | 8 bytes | Microseconds elapsed since recording started |
| `order` | uint64 LE | 8 bytes | Sequence number for ordering packets |
| `message` | bytes | variable | Wire protocol message (may be empty for session events) |

---

## Design Decisions

### 1. Programming Language

**Decision:** Go

**Rationale:**
- Original Mongoreplay was written in Go
- Official MongoDB Go driver is well-maintained
- Good performance for I/O-bound workloads
- Excellent binary parsing libraries
- Easy cross-platform deployment
- Strong concurrency primitives for handling multiple sessions

### 2. Timing Control

**Options:**

```go
type TimingMode int

const (
    TimingFastForward TimingMode = iota  // Send as fast as possible
    TimingScaled                         // Multiply timing by factor (default 1.0)
)

type ReplayConfig struct {
    TimingMode   TimingMode
    TimeScale    float64      // Multiplier for TimingScaled (default 1.0 = real-time, 2.0 = 2x speed, etc.)
}
```

**Implementation approach:**
- Track first packet's offset as baseline
- For each packet, calculate target replay time based on mode
- Sleep until target time before sending

### 3. Connection Strategy

**Three options considered:**

**Option A: One-to-One Session Mapping**
- Create one MongoDB connection per recorded session ID
- Pros: Most accurate replay, preserves session semantics
- Cons: Could be hundreds/thousands of connections, resource intensive

**Option B: Fixed Connection Pool**
- Fixed pool size, round-robin assignment
- Pros: Bounded resource usage
- Cons: Loses session affinity, may affect server-side session state

**Option C: Hybrid - Pool with Session Affinity** (RECOMMENDED)
- Connection pool with session affinity (reuse same connection for same session ID)
- Configurable pool size with spillover behavior
- Pros: Balance between accuracy and resource usage
- Cons: More complex implementation

**Status:** TBD - Need to decide

### 4. Wire Protocol Handling

**Two approaches:**

**Option A: Use Official Go Driver**
- Leverage `mongo-go-driver` for connections and message sending
- Pros: Easier, handles auth/TLS/connection management
- Cons: Less control over exact wire protocol replay

**Option B: Raw Wire Protocol Sender**
- Write raw TCP/wire protocol sender
- Pros: Exact replay of captured messages
- Cons: Much more complex, need to handle connection management, auth, etc.

**Status:** TBD - Need to decide

**Considerations:**
- The Go driver works at a higher level (BSON commands)
- Recorded messages are raw wire protocol bytes
- May need hybrid: use driver for connections, but send raw bytes

### 5. Authentication Handling

**Challenge:** Recorded traffic includes authentication handshakes

**Options:**

1. **Skip auth packets, use provided credentials**
   - Parse and skip auth-related messages
   - Establish connections with user-provided credentials
   - Pros: Works with different credentials than recording
   - Cons: Need to identify auth packets

2. **Replay auth as-is**
   - Send auth packets exactly as recorded
   - Pros: Simple, exact replay
   - Cons: Only works with same credentials

3. **Configurable**
   - Support both modes via flag
   - Pros: Maximum flexibility
   - Cons: More complex

**Status:** TBD - Need to decide

### 6. Database/Collection Targeting

**Options:**

1. **Exact replay** - Use database/collection names from recording
2. **Configurable remapping** - Allow replaying to different database
   - Useful for testing: replay prod traffic to test database
   - Requires parsing and modifying wire protocol messages

**Status:** TBD - Start with exact replay, add remapping later

### 7. Response Handling

**Important**: Traffic recordings capture BOTH requests and responses from the transport layer.

**How to identify:**
- Wire protocol header contains `requestID` and `responseTo` fields
- **Request**: `responseTo == 0` (initiating message)
- **Response**: `responseTo != 0` (references the request it's replying to)

**Modes:**

```go
type ResponseMode int

const (
    ResponseFireAndForget ResponseMode = iota  // Send requests only, ignore all responses
    ResponseValidate                           // Send requests, read target responses, check for errors
    ResponseVerify                             // Send requests, compare target responses to recorded responses
)
```

**Implementation:**

**Fire-and-forget** (Phase 1):
- Filter recording: only send packets where `responseTo == 0` (requests)
- Skip packets where `responseTo != 0` (responses in recording)
- Don't read responses from target server
- Maximum throughput, simplest implementation

**Validate** (Phase 2):
- Send requests only (skip responses in recording)
- Read responses from target server
- Check response for errors (look for error codes)
- Don't compare to recorded responses
- Useful for detecting replay issues

**Verify** (Phase 3):
- Send requests only
- Read responses from target server
- Compare target response to recorded response (find matching `responseTo` in recording)
- **Note**: NOT exact byte-for-byte comparison - many fields will legitimately differ:
  - Server timestamps (e.g., `$currentDate`, `operationTime`, `clusterTime`)
  - Server-generated IDs (e.g., ObjectIDs with timestamp component, UUIDs)
  - Cursor IDs (server-generated handles)
  - Session/transaction IDs
  - System metadata (hostnames, ports, build info)
  - Performance counters, statistics
- **Semantic comparison** needed:
  - Compare document counts
  - Compare document structures (field names, types)
  - Compare non-temporal field values
  - Allow configurable tolerance for numeric fields
  - Configurable field exclusions (e.g., `--ignore-field clusterTime`)
- Report meaningful differences (for debugging, behavior validation)
- Most complex implementation

**Status:** Start with fire-and-forget, add others incrementally

---

## Connection Management Architecture

### Hybrid Approach: Driver Pooling + Raw Wire Messages

Traffic-Replay uses a **hybrid approach** combining the MongoDB driver's connection management with raw wire protocol message sending:

```
┌─────────────────────────────────────────────────────────┐
│  Traffic-Replay Application                             │
│                                                          │
│  ┌────────────────────────────────────────────────┐    │
│  │  Recording Iterator                             │    │
│  │  • Reads .bin files                            │    │
│  │  • Returns raw wire message bytes              │    │
│  └──────────────┬─────────────────────────────────┘    │
│                 │                                        │
│                 │ []byte (raw wire message)             │
│                 ▼                                        │
│  ┌────────────────────────────────────────────────┐    │
│  │  Wire Message Sender                            │    │
│  │  • Validates message (OpCode, header)         │    │
│  │  • Gets connection from driver pool            │    │
│  │  • Writes raw bytes via connection.Write()    │    │
│  └──────────────┬─────────────────────────────────┘    │
│                 │                                        │
└─────────────────┼────────────────────────────────────────┘
                  │ connection.Write(ctx, []byte)
                  ▼
┌─────────────────────────────────────────────────────────┐
│  MongoDB Go Driver (Experimental APIs)                   │
│                                                          │
│  ┌────────────────────────────────────────────────┐    │
│  │  x/mongo/driver/topology                        │    │
│  │  • Connection pool management                  │    │
│  │  • Health checks                               │    │
│  │  • Load balancing                              │    │
│  └──────────────┬─────────────────────────────────┘    │
│                 │                                        │
│                 ▼                                        │
│  ┌────────────────────────────────────────────────┐    │
│  │  x/mongo/driver/mnet.Connection                 │    │
│  │  • Write(ctx, []byte) - Send wire message     │    │
│  │  • Read(ctx) ([]byte, error) - Read response  │    │
│  │  • Handles: Auth, TLS, Compression             │    │
│  └──────────────┬─────────────────────────────────┘    │
└─────────────────┼────────────────────────────────────────┘
                  │ TCP with TLS
                  ▼
         ┌──────────────────┐
         │  MongoDB Server   │
         └──────────────────┘
```

### What We're NOT Doing

❌ **NOT using high-level driver APIs:**
```go
// We DON'T do this:
client.Database("test").Collection("users").InsertOne(ctx, doc)
client.Database("test").Collection("users").Find(ctx, filter)

// These APIs require us to have BSON documents,
// but we only have raw wire protocol bytes from recordings
```

### What We ARE Doing

✅ **Using driver's internal APIs for raw wire message access:**
```go
import (
    "go.mongodb.org/mongo-driver/v2/mongo"
    "go.mongodb.org/mongo-driver/v2/x/mongo/driver/wiremessage"
    "go.mongodb.org/mongo-driver/v2/x/mongo/driver/topology"
)

// Step 1: Use driver for connection establishment
client, _ := mongo.Connect(ctx, options.Client().ApplyURI(uri))

// Step 2: Access internal topology to get raw connections
// (This requires using experimental/internal APIs)
topology := getTopologyFromClient(client)
conn := topology.Connection(ctx, description)

// Step 3: Write raw wire message bytes from recording
rawWireMessage := recording.NextMessage() // []byte from .bin file
err := conn.Write(ctx, rawWireMessage)

// Step 4: Optionally read response (for validation modes)
response, err := conn.Read(ctx)
```

### Benefits of This Approach

| Aspect | What Driver Provides | What We Control |
|--------|---------------------|-----------------|
| **Authentication** | ✅ SCRAM-SHA-1/256, x.509, LDAP, Kerberos, OIDC | - |
| **TLS/SSL** | ✅ Certificate handling, encryption | - |
| **Compression** | ✅ Snappy, Zlib, Zstd automatic handling | - |
| **Connection Health** | ✅ Heartbeats, reconnection, failover | - |
| **Load Balancing** | ✅ Replica set / sharded cluster routing | - |
| **Wire Messages** | - | ✅ Send exact bytes from recording |
| **Timing Control** | - | ✅ Fast-forward, time-scaled replay |
| **Session Mapping** | - | ✅ Map recorded sessions to connections |

### Why This Works

1. **Traffic recordings contain raw wire protocol bytes** - We can't parse them back into high-level operations
2. **Driver handles the hard stuff** - Auth, TLS, compression are complex and production-tested
3. **We get exact replay** - Send the same bytes that were captured, maintaining fidelity
4. **Best of both worlds** - Production-grade connection management + precise replay control

### The Challenge: Accessing Internal APIs

The main challenge is that these APIs are **internal/experimental**:
- `x/mongo/driver/topology` - Not part of public API
- `x/mongo/driver/mnet` - Marked as experimental
- APIs may change without notice

**Mitigation:**
- Pin specific driver version in `go.mod`
- Create abstraction layer around driver APIs
- Vendor dependencies
- Apache 2.0 license allows forking if needed

### Alternative Approach (Not Chosen)

We could have implemented everything ourselves:
```go
// Raw TCP connection approach
conn, _ := net.Dial("tcp", "mongodb://localhost:27017")
conn.Write(wireMessageBytes)
```

**Why we rejected this:**
- ❌ Would need to implement auth ourselves (SCRAM, x.509, LDAP, etc.)
- ❌ Would need to implement TLS ourselves
- ❌ Would need to implement compression ourselves
- ❌ Would need to handle connection failures/reconnection
- ❌ Much more code to write and maintain
- ❌ Higher risk of bugs and security issues

### Session Affinity

For replaying multiple sessions, we maintain a mapping:
```go
// Map recorded session IDs to driver connections
sessionConnections := make(map[SessionID]*topology.Connection)

for packet := range recording {
    // Get or create connection for this session
    conn := sessionConnections[packet.SessionID]
    if conn == nil {
        conn = createNewConnection()
        sessionConnections[packet.SessionID] = conn
    }

    // Send packet on its session's connection
    conn.Write(ctx, packet.Message)
}
```

This ensures operations from the same recorded session go to the same connection, maintaining session semantics.

---

## CLI Interface Design

### Proposed Command Structure

```bash
traffic-replay [flags]
```

### Flags

```bash
# Required
--recording-dir <path>        # Path to recording directory with .bin files

# Target
--target <uri>                # MongoDB connection URI (default: mongodb://localhost:27017)

# Timing
--timing <mode>               # Timing mode: fast-forward, time-scaled (default: time-scaled)
--time-scale <float>          # Speed multiplier for time-scaled mode (default: 1.0)

# Connection Management
--max-connections <int>       # Maximum concurrent connections (default: 100)
--connection-strategy <str>   # Strategy: one-to-one, pool, affinity (default: affinity)

# Response Handling
--response-mode <mode>        # Response handling: fire-and-forget, validate, verify (default: fire-and-forget)

# Authentication
--auth-mode <mode>            # Auth mode: skip, replay, provided (default: skip)
--username <str>              # Username for 'provided' auth mode
--password <str>              # Password for 'provided' auth mode

# Output and Monitoring
--stats-interval <duration>   # Stats output interval (default: 5s)
--verbose                     # Verbose logging
--quiet                       # Suppress output except errors

# Advanced
--verify-checksums            # Verify CRC32C checksums before replay (default: true)
--skip-session-events         # Skip session start/end events (default: false)
--dry-run                     # Parse recording but don't replay
```

### Example Usage

```bash
# Basic replay at original speed (default: time-scaled with 1.0x multiplier)
traffic-replay --recording-dir /data/recordings/prod-capture-001 --target mongodb://localhost:27017

# Fast-forward replay (no timing delays)
traffic-replay --recording-dir /data/recordings/prod-capture-001 --timing fast-forward

# Replay at 10x speed
traffic-replay --recording-dir /data/recordings/prod-capture-001 --time-scale 10.0

# Replay at half speed (0.5x)
traffic-replay --recording-dir /data/recordings/prod-capture-001 --time-scale 0.5

# Replay with custom credentials
traffic-replay --recording-dir /data/recordings/prod-capture-001 \
  --auth-mode provided \
  --username admin \
  --password secret

# Dry run to analyze recording
traffic-replay --recording-dir /data/recordings/prod-capture-001 --dry-run

# Filter: Compress recording by removing responses (Phase 3)
traffic-replay filter \
  --recording-dir /data/recordings/prod-capture-001 \
  --output-dir /data/recordings/prod-capture-001-compressed \
  --requests-only
# Result: 50-90% smaller recording, still usable for replay

# Filter: Split recording for parallel replay (Phase 3)
traffic-replay filter \
  --recording-dir /data/recordings/prod-capture-001 \
  --output-dir /data/split \
  --split 4 \
  --by connections
```

---

## Open Questions

### Resolved (See Decision Log & Design Sections)

1. ✅ **Connection Strategy**: Hybrid approach - use driver pooling + raw wire message APIs
2. ✅ **Wire Protocol**: Use Go driver experimental packages (`x/mongo/driver`)
3. ✅ **Authentication**: Use driver's built-in auth (skip recorded auth packets)
4. ✅ **Mongoreplay Analysis**: Completed - see `docs/reference/mongoreplay-analysis.md`
5. ✅ **Timing Modes**: Two modes - fast-forward and time-scaled (with configurable multiplier)
6. ✅ **OpCode Support**: OP_MSG and OP_COMPRESSED only (MongoDB 7.0+)
7. ✅ **Response Handling**: Traffic recordings include both requests and responses

### Open Questions for Future Implementation

1. **Response Comparison Strategy (Phase 3 - Verify Mode)**
   - **Challenge**: Cannot do exact byte-for-byte comparison of responses
   - **Reasons**:
     - Server timestamps will differ (`$currentDate`, `operationTime`, `clusterTime`)
     - Server-generated IDs will differ (ObjectIDs, UUIDs, cursor IDs)
     - System metadata differs (hostnames, ports, build versions)
     - Performance counters and statistics
   - **Questions**:
     - Which fields should be compared semantically vs. ignored?
     - How to handle nested documents and arrays?
     - Should we parse BSON for comparison or use heuristics?
     - What tolerance for numeric values (floating point)?
     - How to make field exclusions configurable?
   - **Potential Approaches**:
     - Parse BSON responses, compare recursively with field exclusion list
     - Use JSON diff-like algorithm on BSON structures
     - Allow user-provided comparison rules/scripts
     - Pre-defined comparison profiles (e.g., "strict", "relaxed", "structure-only")
   - **Defer to**: Phase 3 implementation (after validate mode is working)

2. **Cursor ID Mapping (Optional Feature)**
   - **Challenge**: Cursor IDs in recording won't match replay server's cursor IDs
   - **Impact**: Multi-batch queries (getMore operations) may fail
   - **Options**:
     - Accept failures (document limitation)
     - Implement cursor ID rewriting (complex, like Mongoreplay did)
     - Make it an optional advanced feature
   - **Decision**: Start without cursor mapping, assess need based on user feedback

---

## Implementation Phases

### Phase 1: Foundation (MVP)
- [ ] Set up Go module and project structure
- [ ] Implement binary packet parser
- [ ] Implement recording file reader/iterator
- [ ] Create basic CLI with minimal flags
- [ ] Parse and display recording contents (dry-run mode)

### Phase 2: Basic Replay
- [ ] Implement simple MongoDB connection
- [ ] Fast-forward timing mode (no delays)
- [ ] Fire-and-forget response mode
- [ ] Handle regular message events only (skip session events initially)
- [ ] Basic error handling

### Phase 3: Timing Control
- [ ] Implement time-scaled mode with configurable multiplier
- [ ] Add statistics and progress reporting

### Phase 4: Connection Management
- [ ] Implement connection pooling
- [ ] Implement session mapping strategy
- [ ] Handle session start/end events properly

### Phase 5: Advanced Features
- [ ] Response validation mode
- [ ] Response verification mode
- [ ] Authentication handling options
- [ ] Database/collection remapping
- [ ] Resume capability

### Phase 6: Polish
- [ ] Comprehensive error handling
- [ ] Documentation
- [ ] Tests
- [ ] Performance optimization

---

## References

- **Mongoreplay GitHub**: https://github.com/mongodb-labs/mongoreplay
- **MongoDB Go Driver**: https://github.com/mongodb/mongo-go-driver
- **Traffic Recording Feature Guide**: See `mongodb-traffic-capture-guide.md` in this repository
- **Wire Protocol Spec**: https://www.mongodb.com/docs/manual/reference/mongodb-wire-protocol/

---

## Notes and Ideas

- Consider adding a "analyze" subcommand to inspect recordings without replaying
- Could add support for filtering (replay only certain sessions, time ranges, etc.)
- Might want to support multiple recording directories (merge multiple captures)
- Consider metrics export (Prometheus, etc.) for production use

---

## Decision Log

| Date | Decision | Rationale |
|------|----------|-----------|
| 2025-10-31 | Use Go as implementation language | Consistency with Mongoreplay, good driver support |
| 2025-10-31 | Support two timing modes (fast-forward, time-scaled with configurable multiplier) | Flexibility; time-scaled with 1.0x = real-time |
| 2025-10-31 | **UPDATED:** Analyzed Mongoreplay source code | See mongoreplay-analysis.md for full findings |
| 2025-10-31 | **UPDATED:** Documented deprecated opcodes | See deprecated-opcodes.md for OpCode handling strategy |
| 2025-10-31 | **UPDATED:** Subcommand-based CLI structure | play, analyze, filter, validate commands |
| TBD | Connection strategy - Decision pending | Researching Go driver wire protocol capabilities |
| TBD | Wire protocol approach - Decision pending | Awaiting Go driver research results |

---

## UPDATE: Mongoreplay Analysis (2025-10-31)

**Status:** Completed comprehensive analysis of original Mongoreplay implementation

### Key Findings

See `docs/reference/mongoreplay-analysis.md` for full analysis. Key takeaways:

#### 1. Wire Protocol Handling
- Mongoreplay uses **MGo driver's socket API** (`mgo.MongoSocket` and `ExecOpWithReply()`)
- Official Go driver does NOT expose equivalent low-level APIs
- **Critical Decision Point:** We likely need to implement raw wire protocol sender

#### 2. Timing Implementation
- Excellent, proven approach that we should adopt
- Baseline from first operation, scale delta by speed factor
- Queue management to prevent excessive buffering
- Simple and effective

#### 3. Connection Strategy
- **One goroutine per recorded connection**
- Channel-based architecture (10k buffer per connection)
- Maintains session affinity naturally
- Works well but no connection limits (resource concern)

#### 4. Deprecated OpCodes Issue

**IMPORTANT:** Many opcodes Mongoreplay supports are **removed** in MongoDB 5.1+:

| OpCode | Status | Removed In |
|--------|--------|-----------|
| OP_QUERY | ❌ Removed | 5.1+ |
| OP_INSERT | ❌ Removed | 5.1+ |
| OP_UPDATE | ❌ Removed | 5.1+ |
| OP_DELETE | ❌ Removed | 5.1+ |
| OP_GET_MORE | ❌ Removed | 5.1+ |
| OP_KILL_CURSORS | ❌ Removed | 5.1+ |
| OP_REPLY | ❌ Deprecated | 3.6+ |
| OP_MSG | ✅ Current | 3.6+ |
| OP_COMPRESSED | ✅ Current | 3.4+ |

**Implication:** recordings may contain legacy opcodes if captured from older MongoDB versions or drivers. Modern MongoDB servers (5.1+) will reject these.

**Strategy:** See `docs/reference/deprecated-opcodes.md` for detailed handling plan:
- **Phase 1 (MVP):** Support OP_MSG and OP_COMPRESSED only; detect and warn about legacy opcodes
- **Phase 2:** Optionally implement opcode translation (legacy → OP_MSG)

#### 5. File Splitting and Filtering

Mongoreplay has excellent `filter` subcommand:
- Split recordings by connection (modulo distribution)
- Filter by time range
- Remove driver operations
- Handles empty files gracefully

**Use Case:** Parallel replay - split large recording into N files, replay each from separate process

See `docs/reference/mongoreplay-cli.md` for full CLI analysis.

### Updated Recommendations

#### CLI Structure: Subcommand-Based

```bash
traffic-replay <command> [options]
```

**Commands:**
1. **play** - Replay traffic (primary command)
2. **analyze** - Inspect recording contents
3. **filter** - Filter and split recordings for parallel replay
4. **validate** - Validate format and checksums

#### Connection Strategy: Hybrid Approach (Updated)

**Phase 1 (MVP):**
- One goroutine per session (like Mongoreplay)
- Simple and maintains session affinity
- Works for typical recordings

**Phase 2:**
- Add `--max-connections` limit
- When limit reached, multiplex sessions over connections
- Warn user about loss of session affinity

#### Wire Protocol: Raw Socket Implementation (Updated)

**Recommendation:** Implement raw wire protocol sender

**Rationale:**
- Official Go driver doesn't expose wire protocol APIs
- Traffic recording gives us raw wire protocol bytes - send them as-is
- Exact replay is the goal
- MGo's approach requires maintaining forked driver (not sustainable)

**Implementation:**
- Parse wire protocol message header from recording packets
- Open `net.Conn` TCP connection to MongoDB
- Write raw bytes to socket
- Optionally read reply for validation mode

#### OpCode Handling Strategy (New)

**Phase 1 (MVP):**
```go
func (r *Replayer) handlePacket(packet *Packet) error {
    opCode := detectOpCode(packet.Message)

    if isLegacyOpCode(opCode) {
        if r.config.SkipUnsupported {
            r.stats.SkippedOps++
            return nil
        }
        return fmt.Errorf("unsupported opcode %s (removed in MongoDB 5.1)", opCode)
    }

    return r.sendToMongoDB(packet.Message)
}
```

**Phase 2:** Implement opcode translation (OP_QUERY → OP_MSG, etc.)

#### File Splitting Strategy (New Feature)

**Design:**

```bash
traffic-replay filter \
  --recording-dir /data/recordings/large-capture \
  --output-dir /data/split \
  --split 4 \
  --by connections
```

**Algorithm:**
```go
outputFile := sessionID % splitCount
```

**Priority:** Medium (not MVP, valuable for Phase 3)

**Use Cases:**
- Parallel replay from multiple processes/machines
- Selective replay of specific connections

#### Recording Compression (Remove Responses)

**Motivation:**
- Responses are typically much larger than requests (often 10-100x per query results)
- For fire-and-forget replay, responses are not needed
- Recordings can be 1-2x the size of actual data transferred
- Compressing by removing responses can reduce recording size by 50-90%
- **Performance bottleneck**: Reading/parsing large responses from disk can prevent replay from "keeping up" during high-speed load testing
  - Fast-forward mode or high time-scale multipliers (10x, 50x, 100x) need maximum I/O throughput
  - Even though we skip sending responses, we still must read and parse them from disk
  - Large responses create memory pressure and parsing overhead
  - Removing responses beforehand eliminates this bottleneck

**Design:**

```bash
# Remove responses, keep only requests
traffic-replay filter \
  --recording-dir /data/recordings/large-capture \
  --output-dir /data/compressed \
  --requests-only

# Alternative: keep both, but compress separately
traffic-replay filter \
  --recording-dir /data/recordings/large-capture \
  --output-dir /data/split \
  --split-by-direction \
  --output-requests requests/ \
  --output-responses responses/
```

**Implementation:**
```go
func filterRequestsOnly(inputDir, outputDir string) error {
    for packet := range readRecording(inputDir) {
        // Parse wire protocol header
        header := parseWireMessageHeader(packet.Message)

        // Keep only requests (responseTo == 0)
        if header.ResponseTo == 0 {
            writePacket(outputDir, packet)
        }
        // Skip responses (responseTo != 0)
    }
}
```

**Output:**
- New recording directory with only request packets
- Maintains session events (start/end)
- Preserves timing information (offsets)
- Updates checksums for new files

**Benefits:**
- Significant disk space savings (50-90% reduction)
- Faster transfer/upload of recordings
- **Critical for high-speed load testing**: Eliminates I/O bottleneck
  - Recording reader can keep up with fast-forward or high time-scale replay
  - Smaller files = faster sequential reads = higher throughput
  - Less parsing overhead = more CPU available for actual replay
  - Better memory cache locality
- Reduces replay preparation time (less data to scan)
- Still fully usable for fire-and-forget and validate modes

**Trade-offs:**
- ❌ Cannot use for verify mode (no recorded responses to compare)
- ✅ Sufficient for most replay use cases (load testing, timing analysis)

**Priority:** Medium (Phase 3 - after filter subcommand basics)

**Use Cases:**
- Compress large recordings before archiving or transfer
- Share recordings without exposing sensitive response data
- Prepare recordings specifically for load testing
- Breaking up multi-GB recordings

### Updated Architecture

```
┌──────────────────────────────────────────────────────────────┐
│  Command: play / analyze / filter / validate                 │
└────────────────┬─────────────────────────────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────────────────────────────┐
│  RecordingSetIterator                                         │
│  • Reads *.bin files from directory                          │
│  • Parses recording binary packet format                          │
│  • Detects OpCodes and filters if needed                     │
│  • Yields packets in order                                   │
└────────────────┬─────────────────────────────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────────────────────────────┐
│  Scheduler (Timing Controller)                                │
│  • Time-scaled: sleep until packet.offset / timeScale       │
│    (timeScale default = 1.0 for real-time replay)           │
│  • Fast-forward: no sleeping, maximum throughput             │
│  • Baseline: first packet offset = replay start time        │
└────────────────┬─────────────────────────────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────────────────────────────┐
│  SessionManager                                               │
│  • One goroutine per session ID                              │
│  • Buffered channels (10k ops per session)                   │
│  • Handles session start/end events                          │
│  • Optional: connection limit enforcement                    │
└────────────────┬─────────────────────────────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────────────────────────────┐
│  WireProtocolSender (Raw Socket Implementation)               │
│  • Opens net.Conn to MongoDB                                 │
│  • Writes raw wire protocol bytes                            │
│  • Optionally reads replies (validation mode)                │
│  • Handles connection lifecycle                              │
└────────────────┬─────────────────────────────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────────────────────────────┐
│  Target MongoDB Instance (5.1+)                               │
│  • Expects OP_MSG / OP_COMPRESSED only                       │
│  • Rejects legacy opcodes                                    │
└──────────────────────────────────────────────────────────────┘
```

### Updated Implementation Phases

**Phase 1: Foundation & Basic Replay (MVP)**
- Set up project structure
- Binary packet parser for recording format
- Recording file iterator
- OpCode detection and filtering
- CLI with `play` subcommand only
- Raw wire protocol sender (basic)
- Two timing modes: fast-forward and time-scaled (with configurable multiplier)
- Fire-and-forget response mode
- Basic statistics

**Phase 2: Production-Ready**
- Time-scaled mode with various multipliers (0.5x, 1.0x, 2.0x, 10.0x)
- Session management with goroutines
- Connection lifecycle handling
- Comprehensive error handling
- `validate` subcommand
- Documentation

**Phase 3: Advanced Features**
- `analyze` subcommand (like Mongoreplay monitor)
- `filter` subcommand with splitting
- Response validation/verification modes
- OpCode translation (legacy → OP_MSG)
- Authentication handling
- Resume capability

**Phase 4: Scale & Performance**
- Parallel replay coordination
- Performance optimization
- Metrics export (Prometheus)
- Distributed replay support

### Go Driver Wire Protocol Research (2025-11-04)

**Status:** ✅ COMPLETE

**Key Discovery:** The MongoDB Go driver DOES have wire protocol APIs in experimental packages!

See `docs/reference/go-driver-wire-protocol.md` for complete analysis.

**Summary:**
- `x/mongo/driver/wiremessage` - OpCode constants, header parsing, message construction
- `x/mongo/driver/mnet.Connection` - Read/Write raw wire message bytes
- Marked as experimental/internal, but usable
- Apache 2.0 license allows us to use or copy

**Decision:** Use experimental packages with version pinning (Approach 1)

---

## FINAL DESIGN DECISIONS (2025-11-04)

All major design decisions have been made based on our research. Ready to begin implementation.

### Decision Summary

| Decision Area | Choice | Rationale |
|---------------|--------|-----------|
| **Wire Protocol** | Use Go driver experimental packages | Tested code, full features, acceptable risk |
| **Connection Strategy** | One goroutine per session (Phase 1) | Simple, maintains affinity, proven by Mongoreplay |
| **Timing Control** | Mongoreplay's algorithm | Proven, simple, works perfectly with recording offsets |
| **OpCode Support** | OP_MSG + OP_COMPRESSED only (Phase 1) | Modern MongoDB, detect/skip legacy |
| **Response Handling** | Fire-and-forget (Phase 1) | Simplest, sufficient for load testing |
| **Authentication** | Use driver's built-in auth | Full support, production-tested |
| **CLI Structure** | Subcommands: play, analyze, filter, validate | Extensible, clear separation of concerns |
| **File Format** | recording binary (.bin files) | Defined by MongoDB traffic recording feature |

### Next Steps

1. **✅ All research complete**
   - Mongoreplay analysis done
   - OpCode compatibility documented
   - Go driver capabilities confirmed

2. **Begin implementation** (Next phase)
   - Set up Go module and project structure
   - Implement recording binary packet parser
   - Create recording file iterator
   - Build basic wire message sender
   - Implement `play` command MVP

3. **MVP Scope** (Phase 1)
   - Parse recording .bin files
   - Detect and skip unsupported opcodes
   - Fast-forward mode (no timing delays)
   - Fire-and-forget (no response handling)
   - Basic statistics
   - Error reporting

---

## Implementation Roadmap

### Phase 1: MVP (Weeks 1-2)
**Goal:** Basic replay functionality

- [x] Complete design and research
- [ ] Project setup (Go module, directory structure)
- [ ] Recording packet parser (`pkg/reader/`)
  - Binary format parsing
  - Header extraction
  - OpCode detection
- [ ] Recording iterator (`pkg/reader/`)
  - Read *.bin files from directory
  - Iterate in order
  - Handle multiple files
- [ ] Wire message sender (`pkg/sender/`)
  - Use Go driver experimental packages
  - Connect to MongoDB
  - Send raw wire messages
- [ ] Basic replayer (`pkg/replay/`)
  - Fast-forward mode (time-scaled comes in Phase 2)
  - Single-threaded
  - Fire-and-forget
- [ ] CLI (`cmd/traffic-replay/`)
  - `play` command with minimal flags
  - Basic error handling
- [ ] Testing
  - Unit tests for packet parser
  - Integration test with test recording

**Deliverable:** Working replayer that can replay OP_MSG traffic at maximum speed

### Phase 2: Production-Ready (Weeks 3-4)
**Goal:** Production-quality features

- [ ] Timing control
  - Time-scaled mode with configurable multiplier
  - Queue management to prevent buffer bloat
- [ ] Session management
  - Goroutine per session
  - Channel-based architecture
  - Session start/end events
- [ ] Connection lifecycle
  - Proper cleanup
  - Error recovery
  - Reconnection logic
- [ ] Statistics
  - Operation counts
  - Throughput metrics
  - Error tracking
  - Progress reporting
- [ ] CLI enhancements
  - All timing modes and multipliers tested
  - Connection options
  - Output formatting
- [ ] `validate` subcommand
  - Verify file format
  - Check checksums
  - Detect OpCode issues
- [ ] Documentation
  - User guide
  - Example usage
  - Troubleshooting

**Deliverable:** Production-ready replayer for real-world use

### Phase 3: Advanced Features (Weeks 5-6)
**Goal:** Power user features

- [ ] `analyze` subcommand
  - Inspect recording contents
  - Statistics without replay
  - OpCode distribution
  - Session analysis
- [ ] `filter` subcommand
  - Split by connection
  - Time-based filtering
  - OpCode filtering
  - **Recording compression (remove responses)**
    - `--requests-only` flag to keep only requests
    - Can reduce recording size by 50-90%
    - Still usable for fire-and-forget and validate modes
  - Parallel replay support
- [ ] Response validation
  - Read replies
  - Check for errors
  - Report failures
- [ ] OpCode translation (optional)
  - OP_QUERY → OP_MSG
  - OP_INSERT → OP_MSG
  - etc.
- [ ] Advanced connection management
  - Connection limits
  - Session multiplexing
  - Pool configuration

**Deliverable:** Full-featured replayer with all planned capabilities

### Phase 4: Polish & Optimization (Week 7+)
**Goal:** Production polish

- [ ] Performance optimization
  - Profiling
  - Memory optimization
  - Concurrent replay
- [ ] Comprehensive testing
  - Large recordings
  - Edge cases
  - Error scenarios
- [ ] Production features
  - Metrics export (Prometheus)
  - Structured logging
  - Signal handling
- [ ] Documentation polish
  - API documentation
  - Architecture guide
  - Contributing guide

**Deliverable:** Battle-tested, optimized replayer

---

## MongoDB Version Compatibility & Test Matrix

### Supported MongoDB Versions

Traffic-Replay targets **officially supported MongoDB versions only**. As of November 2025:

| Version | Status | EOL Date | Support |
|---------|--------|----------|---------|
| **MongoDB 8.x** | Current | October 31, 2029 | ✅ Full Support |
| **MongoDB 7.0** | Stable | August 31, 2027 | ✅ Full Support |
| MongoDB 6.0 | EOL | July 31, 2025 | ❌ Not Supported |
| MongoDB 5.0 | EOL | October 31, 2024 | ❌ Not Supported |

**Minimum Supported Version:** MongoDB 7.0+

### Minor Release Support Model

Starting with MongoDB 8.0, MongoDB introduced a new **minor release model**:

**Key Changes:**
- Minor releases (e.g., 8.0, 8.2, 8.4) are now supported in self-managed deployments
- Previously, point releases were "rapid releases" available only on MongoDB Atlas
- Each minor release is supported independently with its own lifecycle

**Support Window:**
- When a new minor release is published, there's a **60-day overlap period**
- During this period, both the previous and new minor release are officially supported
- After 60 days, only the latest minor release receives support
- This applies to the 8.x series (e.g., 8.0, 8.2, 8.4)

**Example Timeline:**
```
MongoDB 8.0 released:    ━━━━━━━━━━━━━━━━━━━━━━━━━▶ (supported)
MongoDB 8.2 released:              ▲
                                   │
                         ┌─────────┴─────────┐
                         │   60-day overlap  │
                         │  Both supported   │
                         └───────────────────┘
                                             ▲
                                             │
                         After 60 days: only 8.2 supported
```

**Note:** This support model is **not yet publicly documented** by MongoDB (as of Nov 2025).

### Test Matrix

Traffic-Replay will be tested against:

#### Target MongoDB Versions (Replay Destination)
- **MongoDB 7.0** - Previous major release (latest patch)
- **MongoDB 8.0** - Current major release (latest patch)
- **MongoDB 8.2** - Latest minor release
- **Transition periods** - Both supported minor releases during 60-day overlap

#### Source Recording Versions
Recordings may come from:
- MongoDB 7.0+ (officially supported)
- MongoDB 6.0 (recently EOL, may still exist in field)
- MongoDB 5.x and earlier (legacy, best-effort only)

#### Test Scenarios

**Phase 1 (MVP):**
- [ ] MongoDB 7.0 → MongoDB 7.0 (same version)
- [ ] MongoDB 8.0 → MongoDB 8.0 (same version)
- [ ] MongoDB 8.2 → MongoDB 8.2 (same version)
- [ ] MongoDB 7.0 recording → MongoDB 8.0 replay (forward compatibility)
- [ ] MongoDB 7.0 recording → MongoDB 8.2 replay (forward compatibility)
- [ ] Basic OP_MSG operations only

**Phase 2 (Production-Ready):**
- [ ] All Phase 1 scenarios with time-scaled mode (various multipliers: 0.5x, 1.0x, 2.0x, 10.0x)
- [ ] MongoDB 8.0 → MongoDB 7.0 (backward compatibility)
- [ ] MongoDB 8.2 → MongoDB 8.0 (backward compatibility)
- [ ] OP_COMPRESSED message handling
- [ ] Multiple concurrent sessions
- [ ] Large recordings (100K+ operations)

**Phase 3 (Full-Featured):**
- [ ] Legacy opcode detection (MongoDB 6.0 and earlier recordings)
- [ ] Response validation
- [ ] All authentication mechanisms (SCRAM, x.509, LDAP)
- [ ] TLS/SSL connections
- [ ] Replica sets and sharded clusters

**Phase 4 (Production-Grade):**
- [ ] Stress testing with very large recordings (1M+ ops)
- [ ] Long-running replays (hours)
- [ ] High connection count (1000+ sessions)
- [ ] All compression algorithms (Snappy, Zlib, Zstd)

### OpCode Compatibility

MongoDB 7.0+ uses **only modern opcodes**:
- ✅ **OP_MSG** (2013) - Primary operation message
- ✅ **OP_COMPRESSED** (2012) - Compression wrapper

**Legacy opcodes removed in MongoDB 5.1:**
- ❌ OP_QUERY (2004)
- ❌ OP_INSERT (2002)
- ❌ OP_UPDATE (2001)
- ❌ OP_DELETE (2006)
- ❌ OP_GET_MORE (2005)
- ❌ OP_KILL_CURSORS (2007)

**Implication:** Since we target MongoDB 7.0+, we only need to handle OP_MSG and OP_COMPRESSED. Legacy opcodes will be detected and skipped with clear error messages.

### Version Detection Strategy

Traffic-Replay will:

1. **Detect opcodes in recording** during initial scan/validation
2. **Warn if legacy opcodes found:**
   ```
   Warning: Recording contains legacy opcode OP_QUERY (removed in MongoDB 5.1)
   This recording appears to be from MongoDB ≤5.0
   Target server MongoDB 7.0+ cannot replay these operations
   Use --skip-unsupported to skip legacy operations
   ```
3. **Support --skip-unsupported flag** to filter out incompatible operations
4. **Future:** Optional opcode translation (Phase 3+)

### Testing Infrastructure

**Required Test Environments:**
- MongoDB 7.0 (latest patch)
- MongoDB 8.0 (latest patch)
- MongoDB 8.2 (current minor release)
- During transition: both supported 8.x minor releases

**Test Data:**
- Sample recordings from MongoDB 7.0
- Sample recordings from MongoDB 8.0
- Sample recordings from MongoDB 8.2
- Legacy recordings (6.0, 5.0) for compatibility testing
- Synthetic recordings for edge cases

**CI/CD Strategy:**
- Automated tests against MongoDB 7.0, 8.0, and 8.2
- Update test matrix when new minor releases are published
- 60-day grace period for updating tests during version transitions

---

