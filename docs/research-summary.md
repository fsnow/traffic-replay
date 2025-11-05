# Traffic-Replay Research & Design Phase - Complete

**Date Completed:** 2025-11-04
**Status:** ✅ ALL RESEARCH COMPLETE - READY FOR IMPLEMENTATION

---

## Overview

This document summarizes the complete research and design phase for Traffic-Replay, a tool to replay MongoDB traffic recordings.

---

## What We Built

### Documentation Created

1. **`docs/design.md`** (945 lines)
   - Complete architecture and design decisions
   - Implementation roadmap with 4 phases
   - All major decisions finalized
   - Ready-to-follow plan

2. **`docs/reference/mongodb-traffic-recording.md`** (1,502 lines)
   - Complete traffic recording feature reference
   - Binary file format specification
   - Command reference
   - Performance characteristics
   - Created from MongoDB source code analysis

3. **`docs/reference/mongoreplay-analysis.md`** (450+ lines)
   - Comprehensive Mongoreplay code analysis
   - Wire protocol handling approach
   - Connection strategy patterns
   - Lessons learned and recommendations

4. **`docs/reference/deprecated-opcodes.md`**
   - OpCode evolution timeline
   - What works in MongoDB 5.1+
   - Detection and handling strategies
   - Translation approach for legacy opcodes

5. **`docs/reference/mongoreplay-cli.md`**
   - Complete CLI reference
   - File splitting strategies
   - Filter command analysis
   - Parallel replay patterns

6. **`docs/reference/go-driver-wire-protocol.md`**
   - Go driver capabilities assessment
   - Wire protocol API documentation
   - Three implementation approaches
   - Recommendation with rationale

---

## Key Research Findings

### 1. Mongoreplay Analysis

**Repository:** https://github.com/mongodb-labs/mongoreplay

**Key Discoveries:**
- Uses MGo driver's socket API (`mgo.MongoSocket`)
- One goroutine per connection pattern
- Excellent timing algorithm we can adopt
- Channel-based architecture (10k buffer per session)
- Complex cursor mapping (skip for MVP)
- Has useful `filter` command for parallel replay

**Critical Lesson:**
- Fatal flaw was reliance on unmaintained MGo driver
- Network packet capture doesn't work with TLS
- Traffic recording solves both problems

### 2. OpCode Compatibility

**Major Issue Discovered:**
Many opcodes Mongoreplay supports are **removed in MongoDB 5.1+**:

| Removed | Still Supported |
|---------|-----------------|
| OP_QUERY | OP_MSG ✅ |
| OP_INSERT | OP_COMPRESSED ✅ |
| OP_UPDATE | |
| OP_DELETE | |
| OP_GET_MORE | |
| OP_KILL_CURSORS | |

**Implication:**
- recordings from old MongoDB versions may contain legacy opcodes
- Modern servers will reject them
- Must detect and handle appropriately

**Solution:**
- Phase 1: Detect and warn/skip
- Phase 2: Optional translation to OP_MSG

### 3. Go Driver Wire Protocol

**CRITICAL DISCOVERY:** The Go driver DOES have wire protocol APIs!

**Available Packages:**
- `x/mongo/driver/wiremessage` - OpCode parsing, header handling
- `x/mongo/driver/mnet.Connection` - Read/Write raw bytes
- `x/mongo/driver/topology` - Internal connection impl

**Status:** Experimental/Internal
- Marked as "may change without notice"
- But Apache 2.0 licensed (can fork if needed)
- Used in production by driver itself

**Decision:** Use experimental packages with version pinning

**Advantages:**
- ✅ Handles auth (SCRAM, x.509, LDAP, etc.)
- ✅ Handles TLS
- ✅ Handles compression (Snappy, Zlib, Zstd)
- ✅ Production-tested
- ✅ Actively maintained

**Risks:**
- ⚠️ API may change
- **Mitigation:** Pin version, vendor dependencies, abstraction layer

---

## Final Design Decisions

All critical decisions have been made and documented:

| Decision | Choice | Status |
|----------|--------|--------|
| **Language** | Go | ✅ Final |
| **Wire Protocol** | Use Go driver experimental packages | ✅ Final |
| **Connection Strategy** | One goroutine per session | ✅ Final (Phase 1) |
| **Timing** | Mongoreplay's algorithm | ✅ Final |
| **OpCode Support** | OP_MSG + OP_COMPRESSED only | ✅ Final (Phase 1) |
| **Response Handling** | Fire-and-forget | ✅ Final (Phase 1) |
| **Authentication** | Driver's built-in | ✅ Final |
| **CLI Structure** | Subcommands (play/analyze/filter/validate) | ✅ Final |
| **File Format** | recording binary (.bin) | ✅ Final |

---

## Architecture

### High-Level Flow

```
Recording Files (*.bin)
            ↓
    RecordingSetIterator
    (Parse binary packets)
            ↓
      OpCode Detection
    (Filter unsupported)
            ↓
    Timing Controller
   (Real-time/scaled/fast)
            ↓
     Session Manager
   (Goroutine per session)
            ↓
   Wire Protocol Sender
  (Go driver experimental)
            ↓
   Target MongoDB Server
```

### Components

```
traffic-replay/
├── cmd/
│   └── traffic-replay/
│       └── main.go              # CLI entry point
├── pkg/
│   ├── reader/
│   │   ├── packet.go            # Recording packet parsing
│   │   ├── recording.go         # Single file reader
│   │   └── iterator.go          # Multi-file iterator
│   ├── session/
│   │   ├── manager.go           # Session lifecycle
│   │   └── mapper.go            # Session ID mapping
│   ├── replay/
│   │   ├── replayer.go          # Main orchestration
│   │   ├── scheduler.go         # Timing control
│   │   └── stats.go             # Statistics
│   ├── sender/
│   │   ├── connection.go        # MongoDB connections
│   │   └── sender.go            # Wire message sending
│   └── config/
│       └── config.go            # Configuration
├── go.mod
├── docs/                        # ← We are here!
└── README.md
```

---

## Implementation Roadmap

### Phase 1: MVP (Weeks 1-2)
**Goal:** Working basic replayer

**Tasks:**
- [ ] Go module setup
- [ ] recording binary packet parser
- [ ] Recording file iterator
- [ ] Wire message sender (using Go driver)
- [ ] Basic replayer (fast-forward only)
- [ ] CLI with `play` command
- [ ] Unit tests

**Deliverable:** Replay OP_MSG traffic at maximum speed

### Phase 2: Production-Ready (Weeks 3-4)
**Goal:** Real-world usable

**Tasks:**
- [ ] Timing control (real-time, scaled)
- [ ] Session management (goroutines)
- [ ] Connection lifecycle
- [ ] Statistics and progress
- [ ] `validate` subcommand
- [ ] Error handling
- [ ] Documentation

**Deliverable:** Production-ready replayer

### Phase 3: Advanced Features (Weeks 5-6)
**Goal:** Power user features

**Tasks:**
- [ ] `analyze` subcommand
- [ ] `filter` subcommand (splitting)
- [ ] Response validation
- [ ] OpCode translation (optional)
- [ ] Advanced connection management

**Deliverable:** Full-featured tool

### Phase 4: Polish (Week 7+)
**Goal:** Battle-tested

**Tasks:**
- [ ] Performance optimization
- [ ] Comprehensive testing
- [ ] Metrics export
- [ ] Production polish
- [ ] Documentation complete

**Deliverable:** Production-grade replayer

---

## What Makes This Different from Mongoreplay

| Aspect | Mongoreplay | Traffic-Replay |
|--------|-------------|---------------|
| **Capture Method** | Network packets (pcap) | Server-side (transport layer) |
| **TLS Support** | ❌ Cannot see through TLS | ✅ Captures after TLS decryption |
| **Driver** | Unmaintained MGo | Modern Go driver (experimental) |
| **Maintenance** | ❌ Deprecated | ✅ Active development |
| **Wire Protocol** | Custom implementation | Leverage driver code |
| **OpCodes** | Legacy support | Modern only (Phase 1) |
| **Auth** | Limited | Full driver support |

---

## Key Lessons from Mongoreplay

### What To Adopt ✅

1. **Timing Algorithm**
   - Baseline from first operation
   - Scale delta by speed multiplier
   - Simple and effective

2. **Connection Architecture**
   - One goroutine per session
   - Channel-based (buffered 10k)
   - Maintains session affinity

3. **CLI Structure**
   - Subcommand-based
   - Clean separation of concerns
   - Extensible

4. **File Splitting**
   - Connection-based (modulo distribution)
   - Enables parallel replay
   - Simple algorithm

### What To Avoid ❌

1. **Driver Coupling**
   - Don't tie to specific driver
   - Use abstraction layer
   - Plan for API changes

2. **Cursor Mapping Complexity**
   - Too complex for MVP
   - Make it optional
   - Document limitations

3. **No Connection Limits**
   - Can exhaust resources
   - Add configurable limits
   - Warn users

---

## Technical Advantages

### Why Traffic-Replay Will Be Better

1. **TLS Compatible**
   - Traffic recording captures after decryption
   - Works with modern MongoDB deployments
   - No packet capture limitations

2. **Modern Codebase**
   - Go modules
   - Modern Go driver
   - Active maintenance possible

3. **Simpler Format**
   - Binary format (not pcap)
   - Clear packet structure
   - Easy to parse

4. **Better OpCode Handling**
   - Detect legacy opcodes
   - Clear error messages
   - Optional translation

5. **Leverage Driver Features**
   - Full auth support
   - Compression handling
   - Connection management
   - TLS configuration

---

## Risk Mitigation

### Risk: Experimental API Changes

**Mitigation:**
1. Pin exact Go driver version in go.mod
2. Create abstraction layer around driver APIs
3. Vendor dependencies
4. Monitor driver releases
5. Apache 2.0 allows forking if needed

### Risk: Legacy OpCodes in Recordings

**Mitigation:**
1. Clear error messages
2. Option to skip unsupported ops
3. Document MongoDB version requirements
4. Phase 2: Implement translation

### Risk: High Session Count

**Mitigation:**
1. Configurable connection limits
2. Session multiplexing option
3. Warning for high session counts
4. Documentation on resource requirements

### Risk: Large Recordings

**Mitigation:**
1. Streaming file reading
2. Memory-mapped files (optional)
3. File splitting support
4. Progress reporting

---

## Success Criteria

### Phase 1 (MVP)
- [ ] Successfully parses recording .bin files
- [ ] Replays OP_MSG traffic to MongoDB 5.1+
- [ ] Handles auth, TLS automatically
- [ ] Reports basic statistics
- [ ] Clear error messages
- [ ] Passes integration tests

### Phase 2 (Production-Ready)
- [ ] Supports real-time and scaled replay
- [ ] Handles multiple sessions concurrently
- [ ] Provides detailed statistics
- [ ] Graceful error handling
- [ ] Production-quality documentation
- [ ] Passes stress tests

### Phase 3 (Full-Featured)
- [ ] All subcommands working
- [ ] File splitting and filtering
- [ ] Response validation
- [ ] Advanced connection management
- [ ] Community-ready documentation

### Phase 4 (Production-Grade)
- [ ] Performance optimized
- [ ] Battle-tested on large recordings
- [ ] Production monitoring support
- [ ] Complete documentation
- [ ] Ready for MongoDB Labs release

---

## Next Immediate Steps

1. **Create Go module**
   ```bash
   cd /Users/frank.snow/Projects/traffic-replay
   go mod init github.com/fsnow/traffic-replay
   ```

2. **Set up directory structure**
   - Create pkg/ and cmd/ directories
   - Set up basic package structure

3. **Start with packet parser**
   - Implement recording binary format reading
   - Parse packet headers
   - Extract wire protocol messages

4. **Build simple test**
   - Create test .bin file or use existing
   - Parse and display contents
   - Verify OpCode detection

5. **Implement basic sender**
   - Connect to MongoDB using Go driver
   - Send single wire message
   - Verify it works

---

## Resources

### Our Documentation
- `docs/design.md` - Complete design
- `docs/reference/mongodb-traffic-recording.md` - Traffic recording reference
- `docs/reference/mongoreplay-analysis.md` - Mongoreplay learnings
- `docs/reference/deprecated-opcodes.md` - OpCode handling
- `docs/reference/mongoreplay-cli.md` - CLI patterns
- `docs/reference/go-driver-wire-protocol.md` - Driver APIs

### External References
- **Traffic Recording Source:** MongoDB Server `src/mongo/db/traffic_recorder.*`
- **Mongoreplay:** https://github.com/mongodb-labs/mongoreplay
- **Go Driver:** https://github.com/mongodb/mongo-go-driver
- **Wire Protocol Spec:** https://www.mongodb.com/docs/manual/reference/mongodb-wire-protocol/

---

## Conclusion

**We are 100% ready to begin implementation.**

All research has been completed, all major decisions have been made, and all documentation is in place. We have:

✅ Thoroughly analyzed the problem space
✅ Studied the original Mongoreplay implementation
✅ Researched Go driver capabilities
✅ Made all critical design decisions
✅ Created comprehensive documentation
✅ Planned a realistic implementation roadmap

The design is solid, the approach is proven, and the path forward is clear.

**Time to build!**

---

**Research Phase Duration:** Oct 31 - Nov 4, 2025
**Total Documentation:** ~3,500 lines across 6 documents
**Status:** ✅ COMPLETE - READY FOR PHASE 1 IMPLEMENTATION
