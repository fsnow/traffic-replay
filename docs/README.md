# Documentation

This directory contains all documentation for the Traffic-Replay project.

## Directory Structure

```
docs/
├── README.md                              # This file
├── design.md                              # Design decisions and architecture
├── RESEARCH-SUMMARY.md                    # Research phase summary and key findings
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

## Future Documentation

As the project develops, we'll add:
- `user-guide.md` - How to use traffic-replay
- `api.md` - API documentation for packages
- `examples/` - Usage examples and tutorials
- `troubleshooting.md` - Common issues and solutions
- `contributing.md` - Guidelines for contributors
