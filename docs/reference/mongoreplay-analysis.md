# Mongoreplay Code Analysis

**Date:** 2025-10-31
**Repository:** https://github.com/mongodb-labs/mongoreplay
**Purpose:** Understand design decisions and implementation approaches for building Traffic-Replay

---

## Executive Summary

Mongoreplay is a sophisticated tool that captures and replays MongoDB traffic. It was built using the legacy MGo driver and uses its own BSON-based recording format. The tool provides valuable insights for our Traffic-Replay project, particularly around:
- Wire protocol parsing and execution
- Timing control mechanisms
- Connection management strategies
- Session/cursor ID mapping

**Key Takeaway:** Mongoreplay uses direct socket access from the MGo driver (`mgo.MongoSocket`) to send raw wire protocol operations, which is critical for exact replay.

---

## Architecture Overview

### Core Components

```
PlaybackFileReader  →  Play() orchestrator  →  ExecutionContext
                                             ↓
                                    Connection goroutines
                                             ↓
                                    Socket.Execute(op)
                                             ↓
                                    MongoDB Server
```

### File Structure

```
mongoreplay/
├── play.go                  # Main replay orchestration
├── execute.go               # Execution context and operation execution
├── opcode.go                # Wire protocol opcodes
├── recorded_op.go           # RecordedOp structure
├── playbackfile.go          # File reading (BSON-based format)
├── message.go               # Base message handling
├── query_op.go              # OP_QUERY implementation
├── msg_op.go                # OP_MSG implementation
├── insert_op.go             # OP_INSERT implementation
├── update_op.go             # OP_UPDATE implementation
├── delete_op.go             # OP_DELETE implementation
├── getmore_op.go            # OP_GET_MORE implementation
├── killcursors_op.go        # OP_KILL_CURSORS implementation
├── command_op.go            # OP_COMMAND implementation
└── reply_op.go              # OP_REPLY handling
```

---

## Key Design Decisions

### 1. Recording Format

**Mongoreplay's Format:**
- BSON-based serialization
- Each operation stored as BSON document
- Metadata includes:
  - `Seen` timestamp (when operation was captured)
  - `SrcEndpoint` / `DstEndpoint` (connection endpoints)
  - `SeenConnectionNum` (original connection ID)
  - `Order` (sequence number)
  - `Generation` (replay iteration)
  - Raw wire protocol message embedded

**Structure (from recorded_op.go:9-22):**
```go
type RecordedOp struct {
    RawOp
    Seen                *PreciseTime
    PlayAt              *PreciseTime  // Calculated during replay
    EOF                 bool
    SrcEndpoint         string
    DstEndpoint         string
    SeenConnectionNum   int64
    PlayedConnectionNum int64
    PlayedAt            *PreciseTime
    Generation          int
    Order               int64
}
```

**vs. Traffic Recording Format:**
- Binary packet format (not BSON-wrapped)
- Fixed header structure
- Session events (start/end)
- Microsecond offsets from recording start
- Session metadata as BSON string

**Implication for Traffic-Replay:** We need to build a different parser, but the concept of tracking session IDs and timing is similar.

### 2. Timing Control

**Implementation (play.go:144-200):**

```go
func Play(context *ExecutionContext, opChan <-chan *RecordedOp,
          speed float64, repeat int, queueTime int) error {

    var playbackStartTime, recordingStartTime time.Time

    for op := range opChan {
        if recordingStartTime.IsZero() {
            recordingStartTime = op.Seen.Time
            playbackStartTime = time.Now()
        }

        // Calculate time delta from recording start
        opDelta := op.Seen.Sub(recordingStartTime)

        // Scale the delta by speed factor
        scaledDelta := float64(opDelta) / speed

        // Calculate absolute playback time
        op.PlayAt = &PreciseTime{
            playbackStartTime.Add(time.Duration(int64(scaledDelta)))
        }

        // Queue management to prevent buffering too far ahead
        if !context.fullSpeed {
            if opCounter % queueGranularity == 0 {
                time.Sleep(op.PlayAt.Add(
                    time.Duration(-queueTime) * time.Second
                ).Sub(time.Now()))
            }
        }

        // Send to connection channel...
    }
}
```

**Key Insights:**
1. **Baseline Approach:** Track first operation as time=0, calculate all others relative to it
2. **Speed Scaling:** Divide time delta by speed multiplier (2x speed = delta/2)
3. **Queue Management:** Prevent buffering too many operations ahead (default: 15 seconds)
4. **FullSpeed Mode:** Skip all sleeping for maximum throughput

**For Traffic-Replay:** This timing approach is excellent and we should adopt it. Traffic recordings already provide `offset` in microseconds, making it even easier.

### 3. Connection Strategy

**Mongoreplay's Approach:** One goroutine per recorded connection

**Implementation (execute.go:172-226):**

```go
func (context *ExecutionContext) newExecutionConnection(
    start time.Time, connectionNum int64) chan<- *RecordedOp {

    ch := make(chan *RecordedOp, 10000)
    context.ConnectionChansWaitGroup.Add(1)

    go func() {
        // Sleep until 5 seconds before start time
        time.Sleep(start.Add(-5 * time.Second).Sub(time.Now()))

        // Acquire a socket from the MGo session
        socket, err := context.session.AcquireSocketDirect()
        if err == nil {
            connected = true
            defer socket.Close()
        }

        // Process operations from channel
        for recordedOp := range ch {
            // Calculate sleep time if not fullSpeed
            if !context.fullSpeed && recordedOp.RawOp.Header.OpCode != OpCodeReply {
                if t.Before(recordedOp.PlayAt.Time) {
                    time.Sleep(recordedOp.PlayAt.Sub(t))
                }
            }

            // Execute the operation
            parsedOp, reply, err = context.Execute(recordedOp, socket)

            // Collect stats...
        }
    }()
    return ch
}
```

**Connection Mapping (play.go:189-199):**

```go
connectionChans := make(map[int64]chan<- *RecordedOp)

for op := range opChan {
    connectionChan, ok := connectionChans[op.SeenConnectionNum]
    if !ok {
        connectionID++
        connectionChan = context.newExecutionConnection(op.PlayAt.Time, connectionID)
        connectionChans[op.SeenConnectionNum] = connectionChan
    }

    if op.EOF {
        close(connectionChan)
        delete(connectionChans, op.SeenConnectionNum)
    } else {
        connectionChan <- op
    }
}
```

**Key Insights:**
1. **One goroutine per recorded connection** - maintains session affinity
2. **Channel-based communication** - main thread distributes ops to connection goroutines
3. **Buffered channels** (10,000 ops) - allows queueing ahead
4. **Per-connection timing** - each goroutine sleeps independently before sending
5. **Session pooling** - uses `session.AcquireSocketDirect()` from MGo

**For Traffic-Replay:** This is a solid approach. We should consider:
- Similar goroutine-per-session model
- But add configurable limits (max concurrent connections)
- Use connection pooling for when session count is very high

### 4. Wire Protocol Handling

**MGo Driver Socket API:**

Mongoreplay uses a low-level socket API from the MGo driver:

```go
// From query_op.go:162-188
func (op *QueryOp) Execute(socket *mgo.MongoSocket) (Replyable, error) {
    before := time.Now()

    // ExecOpWithReply sends the raw operation and waits for reply
    _, _, replyData, resultReply, err := mgo.ExecOpWithReply(socket, &op.QueryOp)

    after := time.Now()
    if err != nil {
        return nil, err
    }

    // Process reply...
    reply := &ReplyOp{ReplyOp: *mgoReply}
    reply.Latency = after.Sub(before)
    return reply, nil
}
```

**Key Functions:**
- `mgo.ExecOpWithReply(socket, op)` - Sends wire protocol op and receives reply
- Socket is obtained via `session.AcquireSocketDirect()`
- Operations implement an interface with `Execute(socket)` method

**Wire Protocol Opcodes Supported (opcode.go:47-60):**

```go
const (
    OpCodeReply        = OpCode(1)     // OP_REPLY
    OpCodeUpdate       = OpCode(2001)  // OP_UPDATE
    OpCodeInsert       = OpCode(2002)  // OP_INSERT
    OpCodeQuery        = OpCode(2004)  // OP_QUERY
    OpCodeGetMore      = OpCode(2005)  // OP_GET_MORE
    OpCodeDelete       = OpCode(2006)  // OP_DELETE
    OpCodeKillCursors  = OpCode(2007)  // OP_KILL_CURSORS
    OpCodeCommand      = OpCode(2010)  // OP_COMMAND
    OpCodeCommandReply = OpCode(2011)  // OP_COMMANDREPLY
    OpCodeCompressed   = OpCode(2012)  // OP_COMPRESSED
    OpCodeMessage      = OpCode(2013)  // OP_MSG (modern)
)
```

**For Traffic-Replay:**
- The official MongoDB Go driver does NOT expose raw socket/wire protocol access
- We have two options:
  1. **Extract and use MGo's internal wire protocol code** (if license compatible)
  2. **Implement raw wire protocol sender ourselves** using net.Conn
  3. **Find if the official Go driver has any low-level APIs** we can use

This is a **critical decision point** we need to research further.

### 5. Cursor ID Mapping

**Problem:** Cursor IDs from recording won't match cursor IDs from replay server

**Solution:** Mongoreplay maintains a mapping between recorded and live cursor IDs

**Implementation:**
- Extract cursor ID from replies
- Store mapping: `recordedCursorID -> liveCursorID`
- Rewrite cursor IDs in subsequent getMore/killCursors operations

**Code (execute.go:127-147):**

```go
func (context *ExecutionContext) rewriteCursors(
    rewriteable cursorsRewriteable, connectionNum int64) (bool, error) {

    cursorIDs, err := rewriteable.getCursorIDs()

    index := 0
    for _, cursorID := range cursorIDs {
        liveCursorID, ok := context.CursorIDMap.GetCursor(cursorID, connectionNum)
        if ok {
            cursorIDs[index] = liveCursorID
            index++
        }
    }

    newCursors := cursorIDs[0:index]
    err = rewriteable.setCursorIDs(newCursors)
    return len(newCursors) != 0, nil
}
```

**For Traffic-Replay:** We'll need similar cursor mapping logic. This is important for correct replay of multi-batch queries.

### 6. Response Handling

**Mongoreplay's Approach:**
- Always reads replies from server
- Compares recorded reply vs. live reply
- Tracks cursor IDs from both
- Collects statistics on latency, errors, etc.

**Reply Verification (execute.go:19-125):**

```go
type ReplyPair struct {
    ops [2]Replyable  // [0] = from wire, [1] = from file
}

// Waits for both recorded and live replies to complete
// Then compares them and extracts cursor mappings
```

**For Traffic-Replay:**
- Start with fire-and-forget (don't read replies)
- Add response validation/verification as optional feature later
- Traffic recordings may not include server responses (need to verify)

### 7. Preprocessing

**Mongoreplay preprocesses the recording file before replay:**

**Purpose:**
- First pass: build complete cursor ID map
- Prevents getMore operations from failing due to missing cursor mappings

**Implementation (play.go:109-128):**

```go
if !play.NoPreprocess {
    // First pass through file
    opChan, errChan = playbackFileReader.OpChan(1)
    preprocessMap, err := newPreprocessCursorManager(opChan)

    // Rewind file
    playbackFileReader.Seek(0, 0)
    context.CursorIDMap = preprocessMap
}

// Second pass: actual replay
opChan, errChan = playbackFileReader.OpChan(play.Repeat)
Play(context, opChan, play.Speed, play.Repeat, play.QueueTime)
```

**For Traffic-Replay:**
- Consider if we need preprocessing
- Might be needed if we implement cursor mapping
- Or for validation/analysis of recording before replay

---

## Code Patterns Worth Adopting

### 1. CLI Structure

**Uses jessevdk/go-flags for clean CLI:**

```go
type PlayCommand struct {
    GlobalOpts   *Options `no-flag:"true"`
    PlaybackFile string   `description:"..." short:"p" long:"playback-file" required:"yes"`
    Speed        float64  `description:"..." long:"speed" default:"1.0"`
    URL          string   `short:"h" long:"host" default:"mongodb://localhost:27017"`
    FullSpeed    bool     `long:"fullSpeed" description:"..."`
}
```

**Recommendation:** Use similar pattern (or cobra/viper) for Traffic-Replay

### 2. Operation Interface

**Clean abstraction for different operation types:**

```go
type Op interface {
    OpCode() OpCode
    String() string
    Abbreviated(chars int) string
}

type Executable interface {
    Op
    Execute(socket *mgo.MongoSocket) (Replyable, error)
}
```

**Each opcode implements its own Execute method**

**Recommendation:** We may need similar abstraction if we handle opcodes differently

### 3. Statistics Collection

**Collects detailed stats during replay:**

```go
type StatCollector struct {
    // Tracks:
    // - Operation counts by type
    // - Latencies
    // - Errors
    // - Throughput
}
```

**Recommendation:** Implement similar stats tracking for Traffic-Replay

### 4. Parallel File Reading

**Uses worker pool for reading large files:**

```go
type parallelFileReadManager struct {
    numWorkers int
    workers    []*parallelFileReadWorker
}

func (p *parallelFileReadManager) begin(numWorkers int, rs io.ReadSeeker) {
    // Creates worker pool
    // Each worker reads chunks from file
    // Main thread merges results in order
}
```

**Recommendation:** Consider for large recording files, but may not be necessary initially

---

## Challenges and Limitations

### 1. MGo Driver Dependency

**Problem:** MGo is unmaintained
- Mongoreplay is tightly coupled to MGo's socket API
- Uses `internal/llmgo` (forked MGo)
- This is why it's deprecated

**For Traffic-Replay:**
- Cannot use official Go driver the same way
- Need to research wire protocol options

### 2. No TLS Support

**Problem:** Network packet capture can't see through TLS
- This is the fatal flaw that led to traffic recording being created

**For Traffic-Replay:** Not an issue - Traffic recording captures after TLS decryption

### 3. Complex Cursor Mapping

**Challenge:** Maintaining cursor state across replay
- Requires two-pass preprocessing
- Needs careful synchronization
- Can fail if cursor lifetimes don't match

**For Traffic-Replay:**
- Start without cursor mapping
- Add as advanced feature if needed
- May not be critical for many use cases

---

## Recommendations for Traffic-Replay

### Design Decisions

#### 1. **Wire Protocol Approach: RAW SOCKET IMPLEMENTATION**

**Recommendation:** Implement raw wire protocol sender using `net.Conn`

**Rationale:**
- Official Go driver doesn't expose wire protocol APIs
- MGo's approach requires maintaining forked driver
- Traffic recordings give us raw wire protocol bytes - we should send them as-is
- Exact replay is the goal

**Implementation Plan:**
1. Parse wire protocol message headers from recording packets
2. Open TCP connection to MongoDB
3. Write raw bytes to socket
4. Optionally read reply (for validation mode)

**Wire Protocol Reference:**
- https://www.mongodb.com/docs/manual/reference/mongodb-wire-protocol/
- Message format is well-documented
- Relatively straightforward to implement

#### 2. **Connection Strategy: HYBRID APPROACH**

**Recommendation:** Start with one goroutine per session, add pooling later

**Phase 1:**
- One goroutine per recorded session ID (like Mongoreplay)
- Simple and maintains session semantics
- Works well for recordings with reasonable connection counts

**Phase 2 (if needed):**
- Add `--max-connections` flag
- When limit reached, multiplex sessions over available connections
- Warn user that session affinity may be lost

#### 3. **Timing Control: ADOPT MONGOREPLAY'S APPROACH**

**Recommendation:** Use Mongoreplay's timing algorithm

**Implementation:**
```go
type TimingMode int

const (
    TimingRealTime TimingMode = iota
    TimingScaled
    TimingFastForward
)

// Calculate playback time based on offset
playbackTime := playbackStart.Add(
    time.Duration(float64(packet.Offset) / timeScale)
)

// Sleep until playback time
if mode != TimingFastForward {
    time.Sleep(playbackTime.Sub(time.Now()))
}
```

**Advantages:**
- Recording offset field is perfect for this (microseconds from start)
- Simpler than Mongoreplay (no need to calculate deltas)
- Proven approach

#### 4. **Response Handling: START FIRE-AND-FORGET**

**Recommendation:** Phase implementation

**Phase 1:**
- Fire-and-forget (send operations, ignore replies)
- Simplest implementation
- Sufficient for load testing use cases

**Phase 2:**
- Read replies for validation (check for errors)
- Don't compare to recorded replies

**Phase 3:**
- Full verification (if traffic recordings include responses)
- Compare responses to recorded data

#### 5. **Cursor Mapping: SKIP INITIALLY**

**Recommendation:** Don't implement cursor mapping in v1

**Rationale:**
- Adds significant complexity
- Requires preprocessing pass
- May not be needed for many use cases
- Can be added later if required

**Alternative:**
- Document that multi-batch queries may fail on replay
- Suggest filtering them out or accepting failures

#### 6. **Authentication: SKIP AUTH PACKETS**

**Recommendation:** Skip recorded auth, use provided credentials

**Implementation:**
- Detect auth-related opcodes/commands
- Skip during replay
- Establish connection with user-provided credentials
- Document which operations are skipped

**Future:**
- Add option to replay auth for testing auth performance

---

## Critical Research Questions

### 1. Wire Protocol Implementation

**PRIORITY: HIGH**

**Questions:**
- Does official MongoDB Go driver expose any low-level wire protocol APIs?
- Can we fork/extract MGo's wire protocol code (license check)?
- How complex is implementing raw wire protocol from scratch?

**Next Steps:**
- Research official Go driver source code
- Check MGo license compatibility
- Review wire protocol specification
- Build PoC wire protocol sender

### 2. Traffic Recording Response Inclusion

**PRIORITY: MEDIUM**

**Questions:**
- Do traffic recordings include server responses?
- Or only client requests?
- Check mongodb-traffic-capture-guide.md

**Impact:**
- Determines if response verification is possible
- Affects which response modes we can support

**Next Steps:**
- Review traffic recording documentation carefully
- Test traffic recording capture to see what's included

### 3. Authentication Packet Detection

**PRIORITY: MEDIUM**

**Questions:**
- How to detect auth-related operations in wire protocol?
- What opcodes/commands indicate authentication?
- SCRAM, x.509, LDAP, etc.

**Next Steps:**
- Research MongoDB authentication mechanisms
- Identify wire protocol patterns for auth
- Build detection logic

---

## Lessons Learned

### What Mongoreplay Did Well

1. **Clean timing implementation** - scalable, simple, effective
2. **Goroutine per connection** - good session affinity
3. **Channel-based architecture** - clean separation of concerns
4. **Statistics collection** - valuable for users
5. **Operation abstraction** - extensible for different opcodes

### What to Avoid

1. **Tight coupling to specific driver** - led to abandonment
2. **Complex preprocessing** - adds overhead and complexity
3. **No connection limits** - could exhaust resources
4. **All-or-nothing cursor mapping** - simpler to make optional

### Key Takeaways

1. **Wire protocol access is critical** - can't rely on high-level drivers
2. **Timing is straightforward** - don't overcomplicate
3. **Start simple** - fire-and-forget, basic replay first
4. **Iterate** - add features incrementally based on needs

---

## Code Reusability Assessment

### Can We Reuse Mongoreplay Code?

**License:** Apache 2.0 (same as our project) ✅

**Potentially Reusable:**
- ❌ File reading (different format)
- ❌ Wire protocol ops (too coupled to MGo)
- ✅ Timing algorithm (adapt logic, not code directly)
- ✅ OpCode constants/definitions
- ✅ CLI structure patterns
- ✅ Statistics collection patterns
- ❌ Cursor mapping (too complex for v1)

**Recommendation:**
- Use Mongoreplay as reference, not as dependency
- Learn from patterns, reimplement for traffic recording format
- Focus on clean, maintainable code for our specific use case

---

## Next Steps

1. **Research MongoDB Go driver wire protocol capabilities**
   - Check if low-level APIs exist
   - Review source code
   - Test PoC

2. **Design wire protocol sender**
   - Decide on raw socket vs. driver-based approach
   - Document wire protocol message format
   - Plan implementation

3. **Update design.md with decisions**
   - Document chosen approaches
   - Update architecture based on findings
   - Finalize component design

4. **Begin implementation**
   - Start with packet parser (recording binary format)
   - Build file iterator
   - Implement basic replay loop
   - Add wire protocol sender

---

## References

- **Mongoreplay Repository:** https://github.com/mongodb-labs/mongoreplay
- **MongoDB Wire Protocol Spec:** https://www.mongodb.com/docs/manual/reference/mongodb-wire-protocol/
- **MGo Driver:** https://github.com/go-mgo/mgo (archived)
- **Official Go Driver:** https://github.com/mongodb/mongo-go-driver

---

**Analysis completed: 2025-10-31**
