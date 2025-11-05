# Documentation

This directory contains all documentation for the Traffic-Replay project.

## Directory Structure

```
docs/
├── README.md                              # This file
├── design.md                              # Design decisions and architecture
├── RESEARCH-SUMMARY.md                    # Research phase summary and key findings
├── FILTERING.md                           # Guide to filtering recordings (99%+ reduction)
├── COMMAND-AMBIGUITIES.md                 # MongoDB command interpretation for consulting engineers
└── reference/
    ├── mongodb-traffic-recording.md       # MongoDB traffic recording reference
    ├── deprecated-opcodes.md              # Wire protocol OpCode compatibility
    ├── go-driver-wire-protocol.md         # Go driver wire protocol API research
    ├── mongoreplay-analysis.md            # Original mongoreplay implementation analysis
    └── mongoreplay-cli.md                 # Mongoreplay CLI reference
```

## Documentation Files

### design.md
**Main design document** - The central planning and decision-making document containing:
- Project background and context (Mongoreplay history, traffic recording feature)
- Requirements (functional and non-functional)
- Architecture and component design
- Design decisions with rationale
- Binary file format specification
- CLI interface design
- Implementation phases and roadmap
- MongoDB version compatibility matrix
- Decision log with dates

This is a **living document** that evolves as we make decisions and progress. **Start here** to understand the project.

### RESEARCH-SUMMARY.md
High-level summary of the research phase completed in October-November 2025:
- Overview of key findings
- Analysis of Mongoreplay (original tool)
- Wire protocol and OpCode decisions
- Go driver capabilities
- Final design decisions summary

Quick reference for understanding what research was done and why we made certain choices.

### FILTERING.md
**Comprehensive filtering guide** for MongoDB traffic recordings:
- The problem: 99% of traffic is cluster chatter
- How command name extraction works (OP_MSG BSON parsing)
- Simple vs context-aware classification
- Filter tool usage and examples
- Real-world results (2.2 MiB → 15 KiB reduction)
- Performance implications for replay
- When to use each filter mode

**Essential reading** for understanding why recordings grow large and how to prepare them for efficient replay.

### COMMAND-AMBIGUITIES.md
**MongoDB operations interpretation guide** for consulting engineers:
- Command-by-command analysis of ambiguous operations
- getMore: User cursors vs oplog tailing (90% is replication!)
- hello: Connection health checks (explains high op counts)
- Ops Manager / Atlas metrics interpretation
- Customer conversation templates
- "Why 10,000 ops/sec when my app does 500?" explained
- Replica set overhead calculations
- Best practices for workload analysis

**Critical resource** for MongoDB consultants explaining monitoring statistics to customers. Also valuable for understanding what traffic recordings capture.

## Reference Documentation

### reference/mongodb-traffic-recording.md
**Comprehensive reference guide** for MongoDB's traffic recording feature (server-side traffic capture):
- Complete architecture documentation
- Binary file format specification (packet structure, event types)
- Command reference (startRecordingTraffic, stopRecordingTraffic)
- Implementation internals from MongoDB Server source code
- Performance characteristics and memory management
- Security considerations

This was created by analyzing the MongoDB Server source code and serves as our definitive reference for understanding the recording format we'll be parsing.

### reference/deprecated-opcodes.md
**Wire protocol OpCode compatibility guide**:
- Evolution of MongoDB wire protocol OpCodes
- Legacy vs. current OpCodes (OP_QUERY, OP_INSERT vs. OP_MSG)
- MongoDB version timeline (when opcodes were removed)
- **The chicken-and-egg problem** - why legacy recordings are unlikely
- Target version support (MongoDB 7.0+ only)
- OpCode detection strategy for defensive programming
- Testing considerations

Critical for understanding why we only support OP_MSG and OP_COMPRESSED.

### reference/go-driver-wire-protocol.md
**Go driver wire protocol API research**:
- Analysis of `go.mongodb.org/mongo-driver/v2/x/mongo/driver` packages
- Available experimental APIs for raw wire message access
- `wiremessage` package for OpCode constants and parsing
- `mnet.Connection` for raw Read/Write operations
- Decision to use experimental packages with version pinning
- Risks and mitigation strategies

Documents the **key technical decision** on how we'll send raw wire messages while leveraging the driver's connection management.

### reference/mongoreplay-analysis.md
**Deep analysis of the original Mongoreplay implementation**:
- Source code structure and architecture
- How Mongoreplay handled timing control (proven algorithm we'll adopt)
- Connection strategy (one goroutine per session)
- Wire protocol handling with MGo driver
- OpCode support and handling
- Lessons learned and what we'll do differently

Essential reading for understanding the proven approaches we're building upon.

### reference/mongoreplay-cli.md
**Mongoreplay CLI reference**:
- Command structure (play, record, monitor, filter)
- Flag documentation
- Usage examples
- Filter subcommand capabilities (splitting, time ranges)

Reference for CLI design inspiration and understanding Mongoreplay's user interface.

## Document Reading Order

**For developers implementing traffic-replay:**
1. Start with `design.md` - Understand the architecture
2. Read `RESEARCH-SUMMARY.md` - Context on decisions made
3. Review `reference/mongodb-traffic-recording.md` - Binary format details
4. Check `reference/go-driver-wire-protocol.md` - How we'll send messages
5. Reference `FILTERING.md` - How to process recordings

**For MongoDB consulting engineers:**
1. Read `COMMAND-AMBIGUITIES.md` - Interpret customer metrics
2. Review `FILTERING.md` - Understand recording overhead
3. Reference `reference/mongodb-traffic-recording.md` - How traffic recording works

**For understanding the original tool:**
1. `reference/mongoreplay-analysis.md` - Original implementation
2. `reference/mongoreplay-cli.md` - CLI reference
3. `reference/deprecated-opcodes.md` - OpCode evolution

## Future Documentation

As the project develops, we'll add:
- `user-guide.md` - How to use traffic-replay
- `api.md` - API documentation for packages
- `examples/` - Usage examples and tutorials
- `troubleshooting.md` - Common issues and solutions
- `contributing.md` - Guidelines for contributors
