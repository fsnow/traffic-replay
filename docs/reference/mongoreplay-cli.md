# Mongoreplay CLI Reference

**Purpose:** Document Mongoreplay's command structure for reference when designing Traffic-Replay

---

## Command Structure

Mongoreplay uses a subcommand-based CLI with 4 main commands:

```bash
mongoreplay <command> [options]
```

### Commands

1. **play** - Replay captured traffic against MongoDB
2. **record** - Convert network traffic (pcap) into playback file
3. **monitor** - Inspect and analyze traffic (live or recorded)
4. **filter** - Filter and split playback files

---

## `play` Command

Replay recorded traffic against a MongoDB instance.

### Options

```bash
mongoreplay play [options]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-p, --playback-file` | string | **required** | Path to playback file |
| `-h, --host` | string | `mongodb://localhost:27017` | Target MongoDB URI |
| `--speed` | float | `1.0` | Playback speed multiplier |
| `--fullSpeed` | bool | `false` | Replay as fast as possible |
| `--repeat` | int | `1` | Number of times to repeat playback |
| `--queueTime` | int | `15` | Max seconds to buffer ahead |
| `--no-preprocess` | bool | `false` | Skip cursor ID preprocessing |
| `--gzip` | bool | `false` | Decompress gzipped input |
| `--collect` | string | `none` | Stats collection format (json/format/none) |

### SSL Options (if built with SSL)

| Flag | Description |
|------|-------------|
| `--ssl` | Use SSL |
| `--sslCAFile` | CA certificate file |
| `--sslPEMKeyFile` | PEM key file |
| And other SSL-related flags |

### Examples

```bash
# Basic replay at original speed
mongoreplay play -p recording.playback -h mongodb://localhost:27017

# Replay at 2x speed
mongoreplay play -p recording.playback --speed 2.0

# Replay as fast as possible
mongoreplay play -p recording.playback --fullSpeed

# Replay 3 times with stats
mongoreplay play -p recording.playback --repeat 3 --collect json

# Replay gzipped recording
mongoreplay play -p recording.playback.gz --gzip
```

### Key Behaviors

**Speed Control:**
- `--speed 1.0` = Real-time (maintain original timing)
- `--speed 2.0` = 2x faster
- `--speed 0.5` = Half speed (2x slower)
- `--fullSpeed` = Send operations as fast as possible (ignore timing)

**Queue Management:**
- `--queueTime` prevents buffering too far ahead
- Default 15 seconds prevents memory issues with long recordings
- Operations queued up to N seconds ahead, then playback pauses

**Preprocessing:**
- By default, makes two passes through the file
- First pass: Build cursor ID mapping
- Second pass: Actual replay with cursor rewriting
- `--no-preprocess` skips this (may cause getMore failures)

**Repeat:**
- Replays the recording N times in sequence
- Useful for sustained load testing

---

## `record` Command

Convert network packet capture to playback file.

### Options

```bash
mongoreplay record [options]
```

| Flag | Type | Description |
|------|------|-------------|
| `-i, --interface` | string | Network interface to capture from |
| `-f, --pcap` | string | Path to pcap file to read |
| `-p, --playback-file` | string | Output playback file path |
| `--expr` | string | BPF filter expression |
| `--gzip` | bool | Compress output with gzip |

### Examples

```bash
# Capture live traffic
mongoreplay record -i eth0 -p output.playback

# Convert existing pcap file
mongoreplay record -f capture.pcap -p output.playback

# Capture with BPF filter
mongoreplay record -i eth0 --expr "port 27017" -p output.playback

# Capture and compress
mongoreplay record -i eth0 -p output.playback --gzip
```

---

## `monitor` Command

Inspect and analyze traffic in real-time or from recordings.

### Options

```bash
mongoreplay monitor [options]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-i, --interface` | string | | Network interface to monitor |
| `-f, --pcap` | string | | Path to pcap file |
| `-p, --playback-file` | string | | Path to playback file |
| `--paired` | bool | `false` | Output request/reply pairs on single line |
| `--collect` | string | `format` | Stats format (json/format/none) |
| `--gzip` | bool | `false` | Decompress gzipped input |

### Examples

```bash
# Monitor live traffic
mongoreplay monitor -i eth0

# Analyze recording
mongoreplay monitor -p recording.playback

# Paired mode (match requests with replies)
mongoreplay monitor -p recording.playback --paired

# JSON output
mongoreplay monitor -p recording.playback --collect json
```

### Output

Monitor provides detailed analysis:
- Operation types and counts
- Latencies
- Errors
- Throughput
- Connection information

---

## `filter` Command

Filter and split playback files.

### Options

```bash
mongoreplay filter [options]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-p, --playback-file` | string | **required** | Input playback file |
| `-o, --outputFile` | string | | Output file (single file mode) |
| `--outfilePrefix` | string | | Output file prefix (split mode) |
| `--split` | int | `1` | Number of files to split into |
| `--startAt` | string | | ISO 8601 timestamp to start from |
| `--duration` | string | | Duration to capture after start |
| `--removeDriverOps` | bool | `false` | Remove driver-issued operations |
| `--gzip` | bool | `false` | Decompress gzipped input |

### Splitting Strategy

**Connection-based splitting:**
- Operations distributed by connection number
- Uses modulo: `fileNum = connectionID % splitCount`
- Maintains connection affinity within each output file
- Formula (filter.go:124): `fileNum := op.SeenConnectionNum % int64(len(outfiles))`

**Empty file handling:**
- If a split file receives no operations, it's automatically removed
- Prevents creating empty output files

### Examples

```bash
# Filter by time range
mongoreplay filter -p input.playback -o output.playback \
  --startAt 2023-10-31T10:00:00Z \
  --duration 5m

# Split into 4 files by connection
mongoreplay filter -p input.playback \
  --outfilePrefix split_ \
  --split 4

# Result: split_00.playback, split_01.playback, split_02.playback, split_03.playback

# Remove driver operations and filter
mongoreplay filter -p input.playback -o output.playback \
  --removeDriverOps \
  --startAt 2023-10-31T10:00:00Z
```

### Use Cases

1. **Time-based filtering:**
   - Extract specific time window from long recording
   - Test specific events or incidents

2. **Parallel replay:**
   - Split large recording into N files
   - Replay each file from different process/machine
   - Achieve higher throughput

3. **Connection isolation:**
   - Extract traffic from specific connections
   - Debug connection-specific issues

4. **Cleanup:**
   - Remove driver overhead operations (isMaster, etc.)
   - Focus on application traffic

---

## Implications for Traffic-Replay

### Command Structure

**Recommended structure:**

```bash
traffic-replay <command> [options]
```

**Commands:**

1. **play** - Replay traffic recordings *(primary command)*
2. **analyze** - Inspect recording contents *(like mongoreplay monitor)*
3. **filter** - Filter and split recordings *(like mongoreplay filter)*
4. **validate** - Validate recording format and checksums

### Key Features to Adopt

#### From `play` command:
- ✅ Speed control (--speed, --full-speed)
- ✅ Target URI (--target)
- ✅ Stats collection
- ⚠️ Cursor preprocessing (maybe later, not MVP)
- ❌ Repeat (nice-to-have, not critical)

#### From `filter` command:
- ✅ Connection-based splitting (very useful for parallelization)
- ✅ Time-based filtering (--start-time, --duration)
- ⚠️ OpCode filtering (remove unsupported opcodes)
- ⚠️ Compression support

#### From `monitor` command:
- ✅ Analysis/inspection capability
- ✅ Statistics output
- ✅ Connection information

### Traffic-Replay CLI Design

```bash
# Primary use case: replay
traffic-replay play \
  --recording-dir /data/recordings/prod-capture \
  --target mongodb://localhost:27017 \
  --timing scaled \
  --time-scale 2.0

# Analyze recording
traffic-replay analyze \
  --recording-dir /data/recordings/prod-capture \
  --output json

# Split for parallel replay
traffic-replay filter \
  --recording-dir /data/recordings/prod-capture \
  --output-dir /data/split \
  --split 4 \
  --by connections

# Validate recording
traffic-replay validate \
  --recording-dir /data/recordings/prod-capture \
  --verify-checksums
```

---

## File Splitting Design for Traffic-Replay

### Splitting Strategies

#### 1. By Connection (Like Mongoreplay)
```bash
traffic-replay filter \
  --recording-dir input/ \
  --output-dir output/ \
  --split 4 \
  --by connections
```

**Algorithm:**
```go
fileNum := sessionID % splitCount
```

**Pros:**
- Maintains session affinity
- Simple distribution
- Works well for uniform connection distribution

**Cons:**
- Uneven split if few connections
- One hot connection could create unbalanced files

#### 2. By Operation Type
```bash
traffic-replay filter \
  --recording-dir input/ \
  --output-dir output/ \
  --split-by optype
```

**Categories:**
- CRUD operations (insert, update, delete, find)
- Aggregation pipelines
- DDL operations (createIndex, etc.)
- Administrative commands

**Use Case:** Test different workload types separately

#### 3. By Time Window
```bash
traffic-replay filter \
  --recording-dir input/ \
  --output-dir output/ \
  --split-by time \
  --window 5m
```

**Creates one file per time window (e.g., 5 minutes)**

**Use Case:** Analyze traffic patterns over time

#### 4. By Database/Collection
```bash
traffic-replay filter \
  --recording-dir input/ \
  --output-dir output/ \
  --split-by database
```

**Use Case:** Isolate specific database traffic

### Is Splitting Needed?

**YES, for:**

1. **Parallelization:**
   - Run multiple replayer processes
   - Each process handles subset of traffic
   - Achieve higher aggregate throughput
   - Scale beyond single process limits

2. **Selective Replay:**
   - Test specific connection patterns
   - Isolate problematic traffic
   - Focus on specific databases/operations

3. **Large Recordings:**
   - Multi-GB recordings may be unwieldy
   - Easier to work with smaller chunks
   - Can fit in memory better

4. **Distributed Replay:**
   - Run replayers on multiple machines
   - Each machine gets portion of traffic
   - Simulate large-scale load

**Recommended Priority:** Medium (not MVP, but valuable for Phase 2)

### Implementation Approach

**Phase 1 (MVP):**
- Single replayer, no splitting required
- User can manually split with external tools if needed

**Phase 2:**
- Implement `filter` subcommand
- Support connection-based splitting (simplest, most useful)
- Support time-based filtering

**Phase 3:**
- Add operation-type splitting
- Add database/collection splitting
- Add more sophisticated filtering options

---

## Statistics and Monitoring

### Mongoreplay's Stats

**Collects:**
- Operation counts by type
- Latencies (min/max/avg)
- Errors
- Throughput (ops/sec)
- Connection information

**Output Formats:**
- JSON (machine-readable)
- Formatted (human-readable)
- None (silent)

### For Traffic-Replay

**Recommended stats:**

```
Traffic-Replay Statistics
========================
Duration:        5m 32s
Operations:      125,430
  - Sent:        125,430
  - Successful:  125,128
  - Failed:      302
Throughput:      377.2 ops/sec
Connections:     42 active, 127 total
Packets:
  - Regular:     125,430
  - SessionStart: 127
  - SessionEnd:   85
Timing:          Real-time (1.0x)
Errors:          302 (0.24%)
```

**Output formats:**
- Console (real-time updates)
- JSON (--output json)
- Metrics export (Prometheus format, future)

---

## References

- **Mongoreplay Source:** https://github.com/mongodb-labs/mongoreplay
- **Commands:** `main/mongoreplay.go:53-82`
- **Filter Implementation:** `mongoreplay/filter.go`
- **Play Implementation:** `mongoreplay/play.go`
- **Monitor Implementation:** `mongoreplay/monitor.go`

---

**Key Takeaways:**
1. Subcommand structure is clean and extensible
2. Connection-based splitting is simple and effective
3. Time-based filtering is very useful
4. Statistics collection is important for users
5. Start simple (play command), add features incrementally
