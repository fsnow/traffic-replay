# MongoDB Server-Side Traffic Capture: Complete Guide

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Quick Start](#quick-start)
4. [Commands](#commands)
5. [File Format](#file-format)
6. [Internals](#internals)
7. [Reading and Replay](#reading-and-replay)
8. [Performance](#performance)
9. [Security](#security)
10. [Testing](#testing)

---

## Overview

MongoDB's server-side traffic capture (recording) feature allows you to record all incoming and outgoing wire protocol messages for diagnostic, testing, and replay purposes. It captures raw network traffic at the transport layer level and stores it in binary files with metadata about sessions and timing.

### Key Features

- **Wire protocol capture**: Records all MongoDB wire protocol messages
- **Session tracking**: Captures session lifecycle events (start/end)
- **Memory-bounded**: Configurable memory limits prevent unbounded growth
- **Asynchronous I/O**: Background thread handles disk writes without blocking server
- **Scheduled recordings**: Start/stop at specific future times
- **Integrity verification**: CRC32C checksums for all recorded data
- **File rotation**: Automatic file rolling based on size limits

### Use Cases

- Debugging production issues
- Performance analysis and replay
- Testing and validation
- Traffic pattern analysis
- Workload capture for benchmarking

---

## Architecture

### Core Components

```
┌─────────────────────────────────────────────────────────────┐
│                    Transport Layer                           │
│  (src/mongo/transport/session_workflow.cpp)                 │
│                                                              │
│  • Session Start/End                                        │
│  • Incoming Requests                                        │
│  • Outgoing Responses                                       │
└────────────────┬────────────────────────────────────────────┘
                 │
                 │ observe() / sessionStarted() / sessionEnded()
                 ▼
┌─────────────────────────────────────────────────────────────┐
│              TrafficRecorder (Singleton)                     │
│      (src/mongo/db/traffic_recorder.{h,cpp})                │
│                                                              │
│  • Service Context Global                                   │
│  • Manages Recording Lifecycle                              │
│  • Scheduled Task Management                                │
└────────────────┬────────────────────────────────────────────┘
                 │
                 │ pushRecord()
                 ▼
┌─────────────────────────────────────────────────────────────┐
│                Recording Instance                            │
│                                                              │
│  ┌─────────────────────────────────────────────────────┐   │
│  │   Producer-Consumer Queue                            │   │
│  │   (Memory-bounded, Multi-producer, Single-consumer)  │   │
│  └─────────────────┬───────────────────────────────────┘   │
│                    │                                         │
│                    ▼                                         │
│  ┌─────────────────────────────────────────────────────┐   │
│  │   Background Writer Thread                           │   │
│  │   • Serializes packets                               │   │
│  │   • Writes to .bin files                            │   │
│  │   • Computes CRC32C checksums                       │   │
│  │   • Handles file rotation                           │   │
│  └─────────────────┬───────────────────────────────────┘   │
└────────────────────┼───────────────────────────────────────┘
                     │
                     ▼
           ┌──────────────────┐
           │  Recording Files │
           │  • *.bin         │
           │  • checksum.txt  │
           └──────────────────┘
```

### File Locations

| Component | Path |
|-----------|------|
| **Core Implementation** | `src/mongo/db/traffic_recorder.{h,cpp}` |
| **Configuration** | `src/mongo/db/traffic_recorder.idl` |
| **Commands** | `src/mongo/db/commands/traffic_recording_cmds.cpp` |
| **Traffic Reader** | `src/mongo/db/traffic_reader.{h,cpp}` |
| **Replay Iterator** | `src/mongo/replay/traffic_recording_iterator.h` |
| **Transport Integration** | `src/mongo/transport/session_workflow.cpp:414,768,830,914` |
| **Unit Tests** | `src/mongo/db/traffic_recorder_test.cpp` |
| **Integration Tests** | `jstests/noPassthrough/traffic_recording/` |

---

## Quick Start

### 1. Configure Recording Directory

Start MongoDB with the recording directory parameter:

```bash
mongod --trafficRecordingDirectory /data/recordings
```

Or set via server parameter:

```javascript
db.adminCommand({
    setParameter: 1,
    trafficRecordingDirectory: "/data/recordings"
});
```

### 2. Start Recording

```javascript
db.adminCommand({
    startTrafficRecording: 1,
    destination: "my-recording",        // Subdirectory under trafficRecordingDirectory
    maxFileSize: NumberLong(1073741824), // 1 GB per file
    maxMemUsage: NumberLong(134217728)   // 128 MB buffer
});
```

### 3. Monitor Status

**Via serverStatus:**

```javascript
db.runCommand({serverStatus: 1}).trafficRecording;
```

Output:
```javascript
{
    "running": true,
    "bufferSize": 134217728,
    "bufferedBytes": 1048576,
    "recordingDir": "/data/recordings/my-recording",
    "maxFileSize": 1073741824,
    "currentFileSize": 52428800
}
```

**Via getTrafficRecordingStatus:**

```javascript
db.adminCommand({getTrafficRecordingStatus: 1});
```

Output:
```javascript
{
    "ok": 1,
    "status": "running",
    "recordingID": "my-recording-001"
}
```

### 4. Stop Recording

```javascript
db.adminCommand({stopTrafficRecording: 1});
```

---

## Commands

### startTrafficRecording

Starts recording network traffic to disk.

**Syntax:**

```javascript
db.adminCommand({
    startTrafficRecording: 1,
    destination: <string>,           // Required: output directory name
    maxFileSize: <bytes>,            // Optional: max per file (default: 1GB)
    maxMemUsage: <bytes>,            // Optional: max buffer memory (default: 128MB)
    startTime: <Date>,               // Optional: scheduled start time
    endTime: <Date>,                 // Optional: scheduled end time
    recordingID: <string>            // Optional: unique identifier
});
```

**Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `destination` | string | Yes | - | Subdirectory path under `trafficRecordingDirectory` |
| `maxFileSize` | long | No | 1073741824 | Maximum size per recording file (1 GB) |
| `maxMemUsage` | long | No | 134217728 | Maximum buffer memory (128 MB) |
| `startTime` | Date | No | - | Future time to start (must be within 1 day) |
| `endTime` | Date | No | - | Future time to stop (must be within 10 days) |
| `recordingID` | string | No | auto-generated | Unique identifier for idempotency |

**Returns:**

```javascript
{
    "ok": 1,
    "recordingID": "abc123",
    "created": true,
    "status": "running"  // or "scheduled"
}
```

**Status values:**
- `none` - No recording active
- `scheduled` - Recording scheduled for future time
- `running` - Recording currently active
- `failed` - Recording failed (e.g., queue overflow)
- `finished` - Recording completed

**Authorization:**
- Requires `trafficRecord` privilege on cluster resource
- Allowed on secondary replicas

**Validation:**
- `trafficRecordingDirectory` must be set
- `destination` must be a valid directory path
- If `startTime`/`endTime` provided, both must be specified
- `startTime` must be in future and within 1 day
- `endTime` must be after `startTime` and within 10 days
- Cannot start new recording while one is active (unless same `recordingID`)

**Examples:**

**Immediate recording:**
```javascript
db.adminCommand({
    startTrafficRecording: 1,
    destination: "production-capture-001",
    maxFileSize: NumberLong(2147483648),  // 2 GB
    maxMemUsage: NumberLong(268435456)    // 256 MB
});
```

**Scheduled recording:**
```javascript
let startTime = new Date(Date.now() + 3600000);  // 1 hour from now
let endTime = new Date(Date.now() + 7200000);    // 2 hours from now

db.adminCommand({
    startTrafficRecording: 1,
    destination: "scheduled-recording",
    startTime: startTime,
    endTime: endTime,
    recordingID: "maintenance-window-001"
});
```

**Idempotent recording (safe to retry):**
```javascript
// Will not create duplicate recording if called multiple times
db.adminCommand({
    startTrafficRecording: 1,
    destination: "my-recording",
    recordingID: "unique-id-12345"
});
```

---

### stopTrafficRecording

Stops the currently active recording and flushes all buffered data.

**Syntax:**

```javascript
db.adminCommand({stopTrafficRecording: 1});
```

**Returns:**

```javascript
{"ok": 1}
```

**Behavior:**
- Stops active recording immediately
- Cancels any scheduled recording tasks
- Flushes all buffered packets to disk
- Waits for background thread to finish writing
- Records session-end events for all open sessions

**Authorization:**
- Requires `trafficRecord` privilege on cluster resource

**Example:**

```javascript
// Start recording
db.adminCommand({
    startTrafficRecording: 1,
    destination: "test-recording"
});

// ... perform operations ...

// Stop recording
db.adminCommand({stopTrafficRecording: 1});
```

---

### getTrafficRecordingStatus

Returns the current status of traffic recording.

**Syntax:**

```javascript
db.adminCommand({getTrafficRecordingStatus: 1});
```

**Returns:**

```javascript
{
    "ok": 1,
    "status": "running",      // none|scheduled|running|failed|finished
    "recordingID": "abc123"   // Only present if recording active/scheduled
}
```

**Authorization:**
- Requires `trafficRecord` privilege on cluster resource

---

## File Format

### Directory Structure

```
/data/recordings/my-recording/
├── 1698765432000.bin       # Recording file (timestamp in ms)
├── 1698765433000.bin       # Next recording file after rollover
├── 1698765434000.bin
└── checksum.txt            # CRC32C checksums for each file
```

### Binary File Format

Each `.bin` file contains a sequence of packets with the following format:

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

**Header Fields:**

| Field | Type | Size | Description |
|-------|------|------|-------------|
| `size` | uint32_t | 4 bytes | Total packet size including header |
| `eventType` | enum | 1 byte | 0=regular, 1=session start, 2=session end |
| `id` | uint64_t | 8 bytes | Session/connection identifier |
| `session` | string | variable | Null-terminated BSON string with session metadata |
| `offset` | uint64_t | 8 bytes | Microseconds elapsed since recording started |
| `order` | uint64_t | 8 bytes | Sequence number for ordering packets |
| `message` | bytes | variable | Wire protocol message (may be empty for session events) |

**Event Types:**

```cpp
enum class EventType : uint8_t {
    kRegular = 0,       // A regular message event
    kSessionStart = 1,  // Session started (no message data)
    kSessionEnd = 2,    // Session ended (no message data)
};
```

### Checksum File

`checksum.txt` contains CRC32C checksums for integrity verification:

```
1698765432000.bin:a1b2c3d4
1698765433000.bin:e5f6a7b8
1698765434000.bin:c9d0e1f2
```

Format: `filename:hex_crc32c`

---

## Internals

### TrafficRecorder Class

**Location:** `src/mongo/db/traffic_recorder.h:85-230`

The main singleton class managing recording lifecycle.

**Key Methods:**

```cpp
class TrafficRecorder {
public:
    // Singleton access
    static TrafficRecorder& get(ServiceContext* svc);

    // Recording control
    StatusWith<StartRecordingResult> start(StartRecordingOptions options,
                                           ServiceContext* svcCtx);
    void stop(ServiceContext* svcCtx);
    RecordingStatus status() const;

    // Observation methods (called by transport layer)
    void observe(const transport::Session& session,
                 const Message& message,
                 EventType eventType = EventType::kRegular);
    void sessionStarted(const transport::Session& session);
    void sessionEnded(const transport::Session& session);

private:
    AtomicWord<bool> _shouldRecord;              // Fast-path flag
    Synchronized<Recording::Handle> _recording;   // Active recording
    std::unique_ptr<TaskScheduler> _worker;      // Scheduled tasks
};
```

**Member Variables:**

- `_shouldRecord`: Atomic boolean checked on every `observe()` call for fast-path when not recording
- `_recording`: Thread-safe wrapper around active Recording instance
- `_worker`: Task scheduler for delayed start/stop

### Recording Class

**Location:** `src/mongo/db/traffic_recorder.h:121-194`

Manages a single recording session.

**Key Responsibilities:**
- Background thread for disk I/O
- Producer-consumer queue for thread-safe packet passing
- File rotation based on size limits
- CRC32C checksum computation
- Statistics tracking

**Key Methods:**

```cpp
class Recording {
public:
    // Lifecycle
    void start();
    void shutdown();

    // Recording
    bool pushRecord(TrafficRecordingPacket&& packet);

    // Status
    BSONObj getStats() const;
    RecordingStatus getStatus() const;

private:
    void _writerThread();  // Background write loop

    MultiProducerSingleConsumerQueue<TrafficRecordingPacket>::Pipe _pcqPipe;
    stdx::thread _writer;
    std::string _recordingId;
    boost::filesystem::path _recordingDir;
    uint64_t _maxFileSize;
    // ... statistics and state
};
```

### Producer-Consumer Queue

**Type:** `MultiProducerSingleConsumerQueue<TrafficRecordingPacket, CostFunction>`

**Design:**
- Multiple producer threads (transport layer threads)
- Single consumer thread (background writer)
- Cost function: `sizeof(TrafficRecordingPacket) + packet.message.size()`
- Bounded by `maxMemUsage` parameter
- Blocks producers when full (triggers recording failure)

**Memory Safety:**
```cpp
struct CostFunction {
    size_t operator()(const TrafficRecordingPacket& packet) {
        return sizeof(TrafficRecordingPacket) + packet.message.size();
    }
};
```

### Transport Layer Integration

**Location:** `src/mongo/transport/session_workflow.cpp`

Four capture points in the session workflow:

**1. Session Start (Line 414):**
```cpp
void SessionWorkflow::_sourceMessage() {
    // ...
    TrafficRecorder::get(_serviceContext).sessionStarted(*session());
    // ...
}
```

**2. Incoming Message (Line 768):**
```cpp
void SessionWorkflow::_dispatchWork() {
    // ...
    TrafficRecorder::get(_serviceContext).observe(*session(), _work->in());
    // ...
}
```

**3. Outgoing Response (Line 830):**
```cpp
void SessionWorkflow::_acceptResponse() {
    // ...
    TrafficRecorder::get(_serviceContext).observe(*session(), toSink);
    // ...
}
```

**4. Session End (Line 914):**
```cpp
SessionWorkflow::~SessionWorkflow() {
    // ...
    TrafficRecorder::get(_serviceContext).sessionEnded(*session());
}
```

### Background Writer Thread

**Location:** `src/mongo/db/traffic_recorder.cpp:147-249`

The writer thread executes this loop:

1. **Dequeue packet** from producer-consumer queue
2. **Check file size** - rotate if exceeds `maxFileSize`
3. **Serialize packet** using `appendPacketHeader()`
4. **Update CRC32C** checksum
5. **Write to file**
6. **Update statistics**
7. **Handle errors** - mark recording as failed on I/O error

**File Rotation:**
```cpp
if (_currentFileSize + packetSize > _maxFileSize) {
    // Close current file
    _recordingFile.close();
    _checksumFile << _recordingFile.name() << ":" << _crc << std::endl;

    // Open new file with timestamp
    auto timestamp = duration_cast<milliseconds>(now.toDurationSinceEpoch());
    _recordingFile.open(_recordingDir / (to_string(timestamp.count()) + ".bin"));

    _currentFileSize = 0;
    _crc.reset();
}
```

### Packet Serialization

**Location:** `src/mongo/db/traffic_recorder.cpp:91-105`

```cpp
void appendPacketHeader(DataBuilder& dataBuilder, const TrafficRecordingPacket& packet) {
    dataBuilder.writeAndAdvance<LittleEndian<uint32_t>>(totalSize);
    dataBuilder.writeAndAdvance(packet.eventType);
    dataBuilder.writeAndAdvance<LittleEndian<uint64_t>>(packet.id);
    dataBuilder.writeAndAdvance<Terminated<'\0', StringData>>(packet.session);
    dataBuilder.writeAndAdvance<LittleEndian<uint64_t>>(packet.offset.count());
    dataBuilder.writeAndAdvance<LittleEndian<uint64_t>>(packet.order);

    // Append message data
    if (!packet.message.empty()) {
        dataBuilder.writeAndAdvance(packet.message.view());
    }
}
```

### Scheduled Recordings

**Location:** `src/mongo/db/traffic_recorder/utils/task_scheduler.{h,cpp}`

The `TaskScheduler` manages delayed tasks:

**Features:**
- Dedicated background thread
- Priority queue ordered by execution time
- Cancellable tasks
- Thread-safe task submission

**Usage:**
```cpp
_worker->runAt(startTime, [this, svcCtx]() {
    // Start recording at scheduled time
    _start(std::move(rec), svcCtx);
});

_worker->runAt(endTime, [this, svcCtx]() {
    // Stop recording at scheduled time
    _stop(std::move(rec), svcCtx);
});
```

### Session Metadata

**Location:** `src/mongo/db/traffic_recorder_session_utils.cpp:42-56`

Session information captured in BSON format:

```cpp
std::vector<std::pair<transport::SessionId, std::string>>
getActiveSessions(ServiceContext* svcCtx) {
    std::vector<std::pair<transport::SessionId, std::string>> sessions;

    auto tlm = svcCtx->getTransportLayerManager();
    for (auto tl : tlm->getAllTransportLayers()) {
        auto sm = tl->getSessionManager();
        sm->forEach([&](Session& session) {
            sessions.emplace_back(session.id(), session.toBSON().toString());
        });
    }

    return sessions;
}
```

**Session BSON includes:**
- Client IP address and port
- Server IP address and port
- SSL/TLS information
- Connection ID
- Authentication information

---

## Reading and Replay

### TrafficReader

**Location:** `src/mongo/db/traffic_reader.{h,cpp}`

Low-level API for reading recording files.

**Key Types:**

```cpp
struct TrafficReaderPacket {
    EventType eventType;
    uint64_t id;
    StringData session;
    Microseconds offset;
    uint64_t order;
    MsgData::ConstView message;
};
```

**Usage:**

```cpp
#include "mongo/db/traffic_reader.h"

// Read single packet
auto swPacket = TrafficReader::readPacket(fileData);
if (swPacket.isOK()) {
    auto packet = swPacket.getValue();
    // Process packet
}
```

### RecordingIterator

**Location:** `src/mongo/replay/traffic_recording_iterator.h:51-76`

Memory-mapped iterator for efficient reading.

**Usage:**

```cpp
#include "mongo/replay/traffic_recording_iterator.h"

// Iterate through single file
RecordingIterator it("/data/recordings/my-recording/1698765432000.bin");
RecordingIterator end;

for (; it != end; ++it) {
    const TrafficReaderPacket& packet = *it;

    switch (packet.eventType) {
        case EventType::kSessionStart:
            // Handle session start
            break;
        case EventType::kRegular:
            // Handle message
            break;
        case EventType::kSessionEnd:
            // Handle session end
            break;
    }
}
```

**Features:**
- Memory-mapped files for performance
- Standard input iterator interface
- Range-based for loop support

### RecordingSetIterator

**Location:** `src/mongo/replay/traffic_recording_iterator.h:125-152`

Iterates across multiple recording files in a directory.

**Usage:**

```cpp
// Iterate through all files in recording directory
RecordingSetIterator it("/data/recordings/my-recording");
RecordingSetIterator end;

for (; it != end; ++it) {
    const TrafficReaderPacket& packet = *it;
    // Process packets in order across all files
}
```

**Features:**
- Automatically discovers `.bin` files in directory
- Memory maps files on demand
- Maintains temporal ordering across files
- Handles file transitions transparently

### Replay Example

```cpp
void replayTraffic(const std::string& recordingDir) {
    RecordingSetIterator it(recordingDir);
    RecordingSetIterator end;

    std::map<uint64_t, SessionPtr> sessions;

    for (; it != end; ++it) {
        const auto& packet = *it;

        switch (packet.eventType) {
            case EventType::kSessionStart: {
                // Create new session
                auto session = createSession(packet.id, packet.session);
                sessions[packet.id] = session;
                break;
            }

            case EventType::kRegular: {
                // Send message to server
                auto session = sessions[packet.id];
                auto response = session->sendMessage(packet.message);

                // Optionally verify response matches recording
                break;
            }

            case EventType::kSessionEnd: {
                // Close session
                sessions[packet.id]->close();
                sessions.erase(packet.id);
                break;
            }
        }
    }
}
```

---

## Performance

### Design Principles

1. **Zero-Copy**: Message objects passed by reference, no deep copying
2. **Asynchronous I/O**: Background thread prevents blocking transport layer
3. **Memory Bounded**: Queue depth limited to prevent memory exhaustion
4. **Fast Path**: Atomic flag check for minimal overhead when not recording

### Performance Characteristics

**Fast Path (Recording Disabled):**
```cpp
void TrafficRecorder::observe(const Session& session, const Message& message) {
    if (!_shouldRecord.load()) {
        return;  // Single atomic load, no contention
    }
    // ... recording logic
}
```

- **Cost:** Single atomic load (~1-2 ns)
- **Impact:** Negligible when recording disabled

**Recording Path:**
- **Producer overhead:** Queue insertion + packet construction (~100-500 ns)
- **Consumer overhead:** Async, does not block producers
- **Memory:** Bounded by `maxMemUsage` parameter

### Backpressure Handling

When the queue fills up:

1. `pushRecord()` returns `false`
2. Producer side of queue closes
3. Recording marked as `failed`
4. `_shouldRecord` set to `false`
5. Background thread finishes writing buffered data
6. Future `observe()` calls short-circuit

**Code (traffic_recorder.cpp:251-275):**
```cpp
bool Recording::pushRecord(TrafficRecordingPacket&& packet) {
    if (!_pcqPipe.producer.push(std::move(packet))) {
        // Queue full - close producer and mark failed
        _pcqPipe.producer.close();
        _status.store(RecordingStatus::kFailed);
        return false;
    }
    return true;
}
```

### Memory Management

**Queue Memory:**
```
Total Memory = Sum of (sizeof(TrafficRecordingPacket) + message.size())
```

**Per-Packet Overhead:**
- `TrafficRecordingPacket` struct: ~64 bytes
- Session metadata string: ~200-500 bytes
- Message data: Variable (typical: 100 bytes - 16 MB)

**Tuning Recommendations:**

| Workload | maxMemUsage | maxFileSize | Notes |
|----------|-------------|-------------|-------|
| Low traffic | 64 MB | 512 MB | Minimal memory, smaller files |
| Medium traffic | 128 MB | 1 GB | Default settings |
| High traffic | 256-512 MB | 2-4 GB | Reduce I/O overhead |
| Large messages | 512 MB - 1 GB | 4-8 GB | GridFS, backups, etc. |

---

## Security

### Data Sensitivity

**WARNING:** Recording files contain **unencrypted user traffic** including:
- Query content and predicates
- Document data
- Index keys
- User credentials (during authentication)
- Application data

### Security Recommendations

1. **Access Control:**
   - Store recordings on encrypted filesystem
   - Restrict file permissions (chmod 600)
   - Limit access to authorized personnel only

2. **Data Retention:**
   - Delete recordings after analysis
   - Implement automatic cleanup policies
   - Avoid long-term storage

3. **Network Security:**
   - Do not transfer recordings over unencrypted channels
   - Use secure methods (SCP, SFTP) for file transfer
   - Consider encrypting recording directory

4. **Authorization:**
   - Require `trafficRecord` privilege for recording commands
   - Audit access to recording commands
   - Limit privilege to DBA/ops personnel

5. **Compliance:**
   - Review GDPR/privacy implications
   - Document recording policies
   - Obtain necessary approvals

### Warning Message

On `startTrafficRecording`, MongoDB logs:

```
"Recording network traffic to {destination}. "
"Recorded traffic contains user data and must be handled securely. "
"It is recommended to limit retention time and store on an encrypted filesystem."
```

---

## Testing

### Unit Tests

**Location:** `src/mongo/db/traffic_recorder_test.cpp`

**Test Utilities:**

```cpp
class MockSessionWithBSON : public MockSession {
    // Mock session with custom BSON representation
};

class TrafficRecorderTestUtil {
    // Helper functions for test setup
};

class TrafficRecorderForTest : public TrafficRecorder {
    // Exposes internal queue for testing
    auto& getRecordingPipe() { return _recording->_pcqPipe; }
};
```

**Test Coverage:**
- Offset calculation with mock time
- Session BSON serialization
- Error handling
- Queue overflow behavior

### Integration Tests

**Location:** `jstests/noPassthrough/traffic_recording/`

**Key Test Files:**
- `traffic_recording.js` - Basic functionality
- `traffic_reading.js` - Reading and verification

**Test Scenarios:**

1. **Basic Recording:**
```javascript
// Start recording
let res = db.adminCommand({
    startTrafficRecording: 1,
    destination: "test"
});
assert.eq(res.ok, 1);

// Run queries
db.test.insertOne({x: 1});
db.test.find({x: 1}).toArray();

// Stop recording
db.adminCommand({stopTrafficRecording: 1});

// Verify files exist
assert(fileExists("/recordings/test/*.bin"));
```

2. **Directory Validation:**
```javascript
// Invalid directory
assert.commandFailedWithCode(
    db.adminCommand({
        startTrafficRecording: 1,
        destination: "/nonexistent"
    }),
    ErrorCodes.FileNotOpen
);
```

3. **Concurrent Start:**
```javascript
// Start recording
db.adminCommand({startTrafficRecording: 1, destination: "test1"});

// Attempt second start (should fail)
assert.commandFailed(
    db.adminCommand({startTrafficRecording: 1, destination: "test2"})
);
```

4. **RecordingID Idempotency:**
```javascript
// First start
let res1 = db.adminCommand({
    startTrafficRecording: 1,
    destination: "test",
    recordingID: "abc123"
});
assert.eq(res1.created, true);

// Second start with same ID (should succeed but not create new recording)
let res2 = db.adminCommand({
    startTrafficRecording: 1,
    destination: "test",
    recordingID: "abc123"
});
assert.eq(res2.created, false);
```

5. **File Size Limits:**
```javascript
// Small file size to trigger rollover
db.adminCommand({
    startTrafficRecording: 1,
    destination: "test",
    maxFileSize: NumberLong(1048576)  // 1 MB
});

// Insert large documents to trigger rollover
for (let i = 0; i < 1000; i++) {
    db.test.insertOne({data: "x".repeat(10000)});
}

// Verify multiple files created
let files = listFiles("/recordings/test/*.bin");
assert.gt(files.length, 1);
```

6. **Scheduled Recording:**
```javascript
let startTime = new Date(Date.now() + 5000);   // 5 seconds from now
let endTime = new Date(Date.now() + 10000);    // 10 seconds from now

db.adminCommand({
    startTrafficRecording: 1,
    destination: "scheduled",
    startTime: startTime,
    endTime: endTime
});

// Check status - should be "scheduled"
let status = db.adminCommand({getTrafficRecordingStatus: 1});
assert.eq(status.status, "scheduled");

// Wait for recording to start
sleep(6000);
status = db.adminCommand({getTrafficRecordingStatus: 1});
assert.eq(status.status, "running");

// Wait for recording to stop
sleep(5000);
status = db.adminCommand({getTrafficRecordingStatus: 1});
assert.eq(status.status, "none");
```

### Manual Testing

**Test Traffic Capture:**

```bash
# 1. Start MongoDB with recording directory
mongod --trafficRecordingDirectory /tmp/recordings --port 27017

# 2. In mongo shell, start recording
mongo --eval 'db.adminCommand({startTrafficRecording: 1, destination: "manual-test"})'

# 3. Generate traffic
mongo --eval 'for (let i = 0; i < 100; i++) { db.test.insertOne({x: i}) }'

# 4. Stop recording
mongo --eval 'db.adminCommand({stopTrafficRecording: 1})'

# 5. Verify files
ls -lh /tmp/recordings/manual-test/
cat /tmp/recordings/manual-test/checksum.txt
```

**Test Traffic Reading:**

```cpp
// Example C++ program to read recording
#include "mongo/db/traffic_reader.h"
#include "mongo/replay/traffic_recording_iterator.h"

int main() {
    RecordingSetIterator it("/tmp/recordings/manual-test");
    RecordingSetIterator end;

    int sessionStarts = 0;
    int messages = 0;
    int sessionEnds = 0;

    for (; it != end; ++it) {
        switch (it->eventType) {
            case EventType::kSessionStart: sessionStarts++; break;
            case EventType::kRegular: messages++; break;
            case EventType::kSessionEnd: sessionEnds++; break;
        }
    }

    std::cout << "Session Starts: " << sessionStarts << "\n";
    std::cout << "Messages: " << messages << "\n";
    std::cout << "Session Ends: " << sessionEnds << "\n";

    return 0;
}
```

---

## Troubleshooting

### Common Issues

**1. Recording not starting**

**Symptoms:** `startTrafficRecording` returns error

**Possible causes:**
- `trafficRecordingDirectory` not set
- Destination directory doesn't exist
- Insufficient permissions
- Another recording already active

**Solution:**
```javascript
// Check server parameter
db.adminCommand({getParameter: 1, trafficRecordingDirectory: 1});

// Set if not configured
db.adminCommand({setParameter: 1, trafficRecordingDirectory: "/data/recordings"});

// Ensure directory exists
mkdir -p /data/recordings

// Check for active recording
db.adminCommand({getTrafficRecordingStatus: 1});

// Stop active recording if needed
db.adminCommand({stopTrafficRecording: 1});
```

**2. Recording status shows "failed"**

**Symptoms:** Recording stops unexpectedly with "failed" status

**Possible causes:**
- Queue overflow (buffer too small for traffic volume)
- Disk full
- I/O errors
- Permissions issues

**Solution:**
```javascript
// Check server logs for specific error
// Increase buffer size
db.adminCommand({
    startTrafficRecording: 1,
    destination: "test",
    maxMemUsage: NumberLong(268435456)  // Increase to 256 MB
});

// Check disk space
df -h /data/recordings

// Check file permissions
ls -la /data/recordings
```

**3. Large memory usage**

**Symptoms:** High memory consumption during recording

**Possible causes:**
- `maxMemUsage` set too high
- Slow disk causing queue buildup
- Very large messages

**Solution:**
```javascript
// Reduce buffer size
db.adminCommand({
    startTrafficRecording: 1,
    destination: "test",
    maxMemUsage: NumberLong(67108864)  // Reduce to 64 MB
});

// Use faster storage for recording directory
// Consider SSD/NVMe for recordings

// Check for slow queries generating large responses
db.currentOp({"secs_running": {$gte: 5}});
```

**4. Files not being created**

**Symptoms:** Recording running but no `.bin` files

**Possible causes:**
- No traffic to record
- Background thread blocked
- Path misconfiguration

**Solution:**
```javascript
// Generate test traffic
for (let i = 0; i < 100; i++) {
    db.test.insertOne({x: i});
}

// Check serverStatus
db.runCommand({serverStatus: 1}).trafficRecording;

// Verify recording directory
db.adminCommand({getParameter: 1, trafficRecordingDirectory: 1});
```

**5. Cannot read recording files**

**Symptoms:** RecordingIterator fails or returns corrupted data

**Possible causes:**
- Recording not stopped gracefully
- File corruption
- CRC mismatch

**Solution:**
```bash
# Verify checksums
cd /data/recordings/my-recording
for file in *.bin; do
    actual=$(crc32 "$file")
    expected=$(grep "$file" checksum.txt | cut -d: -f2)
    if [ "$actual" != "$expected" ]; then
        echo "Checksum mismatch: $file"
    fi
done

# Always stop recording gracefully
mongo --eval 'db.adminCommand({stopTrafficRecording: 1})'
```

### Debugging Tips

**Enable verbose logging:**
```javascript
db.adminCommand({setParameter: 1, logLevel: 2});
```

**Monitor recording status:**
```javascript
// Periodic status check
while (true) {
    let status = db.runCommand({serverStatus: 1}).trafficRecording;
    print(JSON.stringify(status));
    sleep(1000);
}
```

**Check server logs:**
```bash
# Look for traffic recording messages
grep -i "traffic" /var/log/mongodb/mongod.log

# Look for errors
grep -i "error.*traffic" /var/log/mongodb/mongod.log
```

---

## Advanced Topics

### Custom Replay Logic

Build custom replay tools using the iterator API:

```cpp
class TrafficReplayer {
public:
    void replay(const std::string& recordingDir) {
        RecordingSetIterator it(recordingDir);
        RecordingSetIterator end;

        Microseconds startTime;
        bool first = true;

        for (; it != end; ++it) {
            if (first) {
                startTime = it->offset;
                first = false;
            }

            // Wait until appropriate time
            Microseconds elapsed = it->offset - startTime;
            waitUntil(elapsed);

            // Process packet
            handlePacket(*it);
        }
    }

private:
    void waitUntil(Microseconds target);
    void handlePacket(const TrafficReaderPacket& packet);
};
```

### Traffic Analysis

Analyze recorded traffic without replay:

```cpp
struct TrafficStats {
    size_t totalPackets = 0;
    size_t totalBytes = 0;
    std::map<int, size_t> opCounts;  // OpCode -> count
    Microseconds duration;
};

TrafficStats analyzeTraffic(const std::string& recordingDir) {
    TrafficStats stats;
    RecordingSetIterator it(recordingDir);
    RecordingSetIterator end;

    Microseconds firstOffset, lastOffset;
    bool first = true;

    for (; it != end; ++it) {
        if (it->eventType == EventType::kRegular) {
            stats.totalPackets++;
            stats.totalBytes += it->message.dataLen();

            // Extract opcode from message
            int opCode = it->message.getOperation();
            stats.opCounts[opCode]++;

            if (first) {
                firstOffset = it->offset;
                first = false;
            }
            lastOffset = it->offset;
        }
    }

    stats.duration = lastOffset - firstOffset;
    return stats;
}
```

### Filtering Recordings

Filter recordings by session or time:

```cpp
void filterRecording(const std::string& inputDir,
                     const std::string& outputDir,
                     uint64_t targetSessionId) {
    RecordingSetIterator it(inputDir);
    RecordingSetIterator end;

    std::ofstream out(outputDir + "/filtered.bin", std::ios::binary);

    for (; it != end; ++it) {
        if (it->id == targetSessionId) {
            // Write packet to filtered output
            writePacket(out, *it);
        }
    }
}
```

### Performance Profiling

Measure recording overhead:

```cpp
// In transport layer
auto startTime = std::chrono::high_resolution_clock::now();
TrafficRecorder::get(_serviceContext).observe(*session(), message);
auto endTime = std::chrono::high_resolution_clock::now();
auto duration = std::chrono::duration_cast<std::chrono::nanoseconds>(endTime - startTime);

// Log if overhead is high
if (duration.count() > 1000) {  // > 1 microsecond
    LOGV2_WARNING(12345, "High traffic recording overhead", "duration"_attr = duration);
}
```

---

## FAQ

**Q: Can I record on a replica set primary?**
A: Yes, recording works on primaries, secondaries, and standalones.

**Q: Does recording affect replication?**
A: No, recording is a local operation and does not affect replication.

**Q: Can I record on multiple nodes simultaneously?**
A: Yes, each node has independent recording. You can record on all replica set members.

**Q: What is the performance impact?**
A: Minimal when not recording (<1ns). During recording, ~100-500ns per message. Background I/O is async and doesn't block.

**Q: How much disk space is needed?**
A: Depends on traffic volume. Estimate: 1-2x the size of data transferred. Monitor `currentFileSize` in serverStatus.

**Q: Can I record specific databases or collections?**
A: No, recording captures all traffic. You can filter during replay/analysis.

**Q: Are recordings portable across MongoDB versions?**
A: The binary format is stable, but wire protocol changes between versions may affect replay. Test carefully.

**Q: Can I replay on a different topology?**
A: Yes, but session IDs won't match. Build custom replay logic to handle this.

**Q: What happens if disk fills up during recording?**
A: Recording will fail with I/O error and status becomes "failed". Buffered data is lost.

**Q: Can I pause and resume recording?**
A: No, you must stop and start a new recording.

**Q: Are compressed messages recorded compressed or decompressed?**
A: Messages are recorded as they appear on the wire (compressed if compression is enabled).

**Q: Does recording capture internal operations?**
A: No, only client-visible traffic through the transport layer.

---

## Changelog

### MongoDB Server Versions

The traffic recording feature has evolved across versions. Key milestones:

- **Initial implementation:** Core recording functionality
- **SERVER-106769:** Future enhancement to TickSource usage (pending)
- **SERVER-111903:** Fix for session start double-reporting (pending)

Check MongoDB release notes for version-specific changes.

---

## Additional Resources

### Source Code

- Core implementation: `src/mongo/db/traffic_recorder.{h,cpp}`
- Command handlers: `src/mongo/db/commands/traffic_recording_cmds.cpp`
- Reader API: `src/mongo/db/traffic_reader.{h,cpp}`
- Iterators: `src/mongo/replay/traffic_recording_iterator.h`
- Tests: `src/mongo/db/traffic_recorder_test.cpp`
- Integration tests: `jstests/noPassthrough/traffic_recording/`

### Related Features

- **mongobridge:** Network simulation tool for testing
- **resmoke.py:** Test framework with traffic recording integration
- **Wire Protocol Documentation:** Understanding message format

### MongoDB Tools

- **mongodump/mongorestore:** Different approach to capturing data
- **profiler:** Captures query patterns without network data
- **auditLog:** Captures operations for compliance

---

## License

This document describes MongoDB Server features. MongoDB Server is available under the Server Side Public License (SSPL).

---

**Document Version:** 1.0
**Last Updated:** 2025-10-31
**MongoDB Source:** mongodb/mongo master branch
