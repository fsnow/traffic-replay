# Traffic Recording Filtering

## The Problem: 99% Cluster Chatter

When you record MongoDB traffic, you capture **everything**:
- User operations (insert, find, update, delete)
- **Internal replication traffic** (secondaries tailing oplog)
- **Health checks** (driver connection pings)
- **Monitoring** (cluster state queries)
- **Responses** (often larger than requests)

**Real-world example** (recording1.txt):
```
Total: 5041 packets (2.2 MiB)
├─ User operations:    37 packets (0.7%) - 15 KiB
└─ Cluster chatter: 5004 packets (99.3%) - 2.2 MiB
```

**For replay, you typically only need the user operations!**

---

## How We Differentiate Operations

### Method 1: Command Name Extraction

For **OP_MSG** messages (opcode 2013), the command name is the **first field** in the BSON document:

```go
// OP_MSG structure:
Wire Header (16 bytes)
Flags (4 bytes)
Section kind (1 byte) = 0
BSON document:
  ├─ size (4 bytes)
  ├─ element type (1 byte)
  ├─ element name (null-terminated) ← THIS IS THE COMMAND NAME
  └─ element value ...
```

Example from Packet #1173 (insert):
```
Hex: 02 69 6e 73 65 72 74 00 06 00 00 00 75 73 65 72 73 00
     └─ type=2 (string)
        └─ "insert\0"
                      └─ length=6
                         └─ "users\0"
```

### Method 2: Simple Classification

Commands are categorized by name:

**Likely User Operations (but context-dependent):**
- CRUD: `insert`, `update`, `delete`, `find`, `findAndModify`, `aggregate`, `count`, `distinct`
  - **Note:** Even writes can be internal (e.g., `insert` on `system.sessions`)
- DDL: `create`, `drop`, `createIndexes`, `dropIndexes`, `listIndexes`, `collMod`

**Definitely Internal Operations:**
- Health: `hello`, `isMaster`, `ping`, `buildInfo`
- Replication: `replSetHeartbeat`, `replSetUpdatePosition`

**Highly Ambiguous:**
- `getMore` - User cursor continuation OR oplog tailing (90%+ is replication)
- `find` - User query OR driver/monitoring discovery
- `aggregate` - User pipeline OR Atlas metrics collection

**The Problem: Most commands need context to classify correctly!**

### Method 3: Context-Aware Classification (Smart)

Some commands need **context** to classify correctly:

#### Example: `getMore`

`getMore` can be either:

1. **Internal** (replication):
   ```
   Database: local
   Collection: oplog.rs
   → This is a secondary tailing the oplog → INTERNAL
   ```

2. **User** (cursor continuation):
   ```
   Database: traffictest
   Collection: users
   → This is continuing a user's find() query → USER
   ```

Our smart filter checks:
```go
func (p *Packet) IsLikelyUserOperation() bool {
    cmd := p.ExtractCommandName()

    if cmd == "getMore" {
        db := p.ExtractDatabase()
        coll := p.ExtractCollection()

        // Oplog tailing = internal
        if db == "local" && coll == "oplog.rs" {
            return false
        }

        // User database = user operation
        if !IsInternalDatabase(db) {
            return true
        }
    }

    // ... other logic
}
```

---

## Filter Tool Usage

### Basic Filters

```bash
# Remove responses (61% reduction)
filter -input recording.bin -output filtered.bin -requests-only

# Remove cluster chatter (99% reduction!)
filter -input recording.bin -output filtered.bin -user-ops-only

# Combined: user requests only (maximum reduction)
filter -input recording.bin -output filtered.bin -requests-only -user-ops-only
```

### Smart Context-Aware Filter

```bash
# Uses database/collection context for ambiguous commands
filter -input recording.bin -output filtered.bin -user-ops-smart -requests-only
```

### Command-Specific Filters

```bash
# Only insert operations
filter -input recording.bin -output filtered.bin -include-commands insert

# Only inserts and updates
filter -input recording.bin -output filtered.bin -include-commands insert,update

# Everything except health checks
filter -input recording.bin -output filtered.bin -exclude-commands hello,ping
```

### Time-Based Filters

```bash
# First 100ms of recording
filter -input recording.bin -output filtered.bin -max-offset 100000

# Between 100ms and 200ms
filter -input recording.bin -output filtered.bin -min-offset 100000 -max-offset 200000
```

---

## Internal Databases & Collections

### Internal Databases
- `local` - Replication, oplog, startup logs
- `admin` - Administrative commands (mixed - some user ops go here too)
- `config` - Sharding metadata

### Internal Collections
- `oplog.rs` - Replication oplog (in `local` database)
- `system.*` - System collections (indexes, users, etc.)
- `replset.*` - Replica set internal state
- `startup_log` - Server startup history

---

## Filter Results Example

```bash
$ filter -input recording1.txt -output filtered.bin -user-ops-only -requests-only

================================================================================
FILTER RESULTS
================================================================================

Input:
  Packets: 5041
  Size:    2.2 MiB

Output:
  Packets: 37
  Size:    15.0 KiB

Reduction:
  Packets dropped: 5004 (99.3%)
  Bytes dropped:   2.2 MiB (99.3%)

Dropped by reason:
  Responses:           3104
  Internal operations: 1900
```

**What's in the filtered file:**
```
12 aggregate operations
10 find operations
5  insert operations
4  update operations
3  delete operations
1  createIndexes
1  distinct
1  listIndexes
```

---

## When to Use Each Filter Mode

| Scenario | Recommended Filter |
|----------|-------------------|
| **Load testing** | `--requests-only --user-ops-only` |
| **Functional testing** | `--user-ops-only` (keep responses for validation) |
| **Debugging specific operation** | `--include-commands <cmd>` |
| **Large-scale replay** | `--user-ops-smart --requests-only` |
| **Analyzing recording** | No filter (use analyze tool instead) |

---

## Command Categories Reference

### Likely User Operations (Context-Dependent)
**CRUD Operations:**
- `insert`, `update`, `delete` - **BUT** can be internal on system collections (e.g., `system.sessions`)
- `find`, `findAndModify` - **BUT** can be internal on admin/config databases
- `aggregate`, `count`, `distinct` - **BUT** can be internal for metrics collection

**DDL Operations:**
- `create`, `drop`
- `createIndexes`, `dropIndexes`
- `collMod`, `renameCollection`

**Smart filter checks:** Database must be user database AND collection must not be a system collection

### Definitely Internal Operations
**Health Checks:**
- `hello`, `isMaster`, `ping`
- `buildInfo`, `serverStatus`

**Replication:**
- `replSetHeartbeat`
- `replSetUpdatePosition`
- `replSetGetStatus`, `replSetGetConfig`

### Highly Ambiguous (Always Check Context)
- `getMore` - User cursor continuation (user DB) OR oplog tailing (`local.oplog.rs`)
- `find` - User query (user DB) OR driver discovery (admin/config)
- `aggregate` - User pipeline (user DB) OR metrics collection (admin/atlascli)
- `listIndexes` - User query OR driver discovery
- `listCollections` - User query OR tool refresh
- `listDatabases` - User query OR monitoring

---

## Performance Impact

Filtering **before** replay is critical for performance:

### Without Filtering
```
Recording: 2.2 MiB, 5041 packets
Replay time: ~400ms (replaying all cluster chatter)
I/O overhead: Reading and parsing 99% unnecessary data
Memory pressure: Buffering thousands of internal operations
```

### With Filtering
```
Filtered: 15 KiB, 37 packets (99.3% reduction)
Replay time: ~5ms (only user operations)
I/O overhead: Minimal
Memory pressure: Minimal
```

**Result:** 80x smaller files, 80x faster replay, cleaner testing

---

## See Also

- `cmd/analyze/` - Analyze recording contents without filtering
- `cmd/packets/` - Inspect individual packets in detail
- `cmd/filter/` - Filter recording files
- `pkg/reader/commands.go` - Command extraction implementation
- `pkg/reader/context.go` - Context-aware classification
