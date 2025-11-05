# MongoDB Go Driver Wire Protocol Capabilities

**Date:** 2025-11-04
**Go Driver Repository:** https://github.com/mongodb/mongo-go-driver
**Purpose:** Determine if official Go driver can be used for sending raw wire protocol messages

---

## Executive Summary

**ANSWER: YES** - The MongoDB Go driver DOES provide wire protocol APIs, but they are in **internal/experimental packages** marked as unstable.

### Key Findings

1. ✅ **Wire protocol parsing/construction available** (`x/mongo/driver/wiremessage`)
2. ✅ **Raw byte read/write available** (`x/mongo/driver/mnet.Connection`)
3. ⚠️ **Marked as experimental** - No backward compatibility guarantee
4. ⚠️ **Internal packages** - Not intended for external use
5. ✅ **Apache 2.0 License** - Can legally use/copy code if needed

---

## Available Wire Protocol APIs

### 1. Wire Message Package

**Location:** `x/mongo/driver/wiremessage/wiremessage.go`

**Package Documentation:**
```go
// Package wiremessage is intended for internal use only. It is made available
// to facilitate use cases that require access to internal MongoDB driver
// functionality and state. The API of this package is not stable and there is
// no backward compatibility guarantee.
//
// WARNING: THIS PACKAGE IS EXPERIMENTAL AND MAY BE MODIFIED OR REMOVED WITHOUT
// NOTICE! USE WITH EXTREME CAUTION!
```

#### OpCode Constants

```go
type OpCode int32

const (
    OpReply        OpCode = 1
    OpUpdate       OpCode = 2001
    OpInsert       OpCode = 2002
    OpQuery        OpCode = 2004  // Deprecated: Use OpMsg instead
    OpGetMore      OpCode = 2005
    OpDelete       OpCode = 2006
    OpKillCursors  OpCode = 2007
    OpCommand      OpCode = 2010
    OpCommandReply OpCode = 2011
    OpCompressed   OpCode = 2012
    OpMsg          OpCode = 2013
)

func (oc OpCode) String() string // Human-readable names
```

#### Header Functions

**Read wire message header:**
```go
func ReadHeader(src []byte) (
    length int32,
    requestID int32,
    responseTo int32,
    opcode OpCode,
    rem []byte,
    ok bool,
)
```

**Construct wire message header:**
```go
func AppendHeaderStart(dst []byte, reqid, respto int32, opcode OpCode) (
    index int32,
    b []byte,
)

func AppendHeader(dst []byte, length, reqid, respto int32, opcode OpCode) []byte
```

**Request ID generation:**
```go
func NextRequestID() int32  // Thread-safe global counter
```

#### OP_MSG Functions

```go
// Flags
type MsgFlag uint32
const (
    ChecksumPresent MsgFlag = 1 << iota
    MoreToCome
    ExhaustAllowed MsgFlag = 1 << 16
)

// Read/Write flags
func ReadMsgFlags(src []byte) (flags MsgFlag, rem []byte, ok bool)
func AppendMsgFlags(dst []byte, flags MsgFlag) []byte

// Section handling
type SectionType uint8
const (
    SingleDocument SectionType = iota
    DocumentSequence
)

func ReadMsgSectionType(src []byte) (stype SectionType, rem []byte, ok bool)
func ReadMsgSectionSingleDocument(src []byte) (doc bsoncore.Document, rem []byte, ok bool)
func ReadMsgSectionDocumentSequence(src []byte) (identifier string, docs []bsoncore.Document, rem []byte, ok bool)
func ReadMsgChecksum(src []byte) (checksum uint32, rem []byte, ok bool)
```

#### Compression Support

```go
type CompressorID uint8
const (
    CompressorNoOp CompressorID = iota
    CompressorSnappy
    CompressorZLib
    CompressorZstd
)

func ReadCompressedOriginalOpCode(src []byte) (opcode OpCode, rem []byte, ok bool)
```

### 2. Connection Package

**Location:** `x/mongo/driver/mnet/connection.go`

#### Public Connection Interface

```go
// ReadWriteCloser represents a Connection where server operations
// can read from, written to, and closed.
type ReadWriteCloser interface {
    Read(ctx context.Context) ([]byte, error)
    Write(ctx context.Context, wm []byte) error
    io.Closer
}

// Connection represents a connection to a MongoDB server
type Connection struct {
    ReadWriteCloser
    Describer
    Streamer
    Compressor
    Pinner
}
```

**Key Methods:**
- `Read(ctx context.Context) ([]byte, error)` - Read raw wire message
- `Write(ctx context.Context, wm []byte) error` - Write raw wire message
- `Close() error` - Close connection

#### Describer Interface

```go
type Describer interface {
    Description() description.Server
    ID() string
    ServerConnectionID() *int64
    DriverConnectionID() int64
    Address() address.Address
    Stale() bool
    OIDCTokenGenID() uint64
    SetOIDCTokenGenID(uint64)
}
```

### 3. Internal Connection Implementation

**Location:** `x/mongo/driver/topology/connection.go`

The internal implementation shows how raw wire messages are handled:

```go
// Write raw wire message bytes
func (c *connection) writeWireMessage(ctx context.Context, wm []byte) error {
    // Sets deadline, handles cancellation
    // Writes to underlying net.Conn
    _, err = c.nc.Write(wm)
    return err
}

// Read raw wire message bytes
func (c *connection) readWireMessage(ctx context.Context) ([]byte, error) {
    // Reads 4-byte length prefix
    // Validates message size
    // Reads remaining message bytes
    // Returns complete wire message
}
```

**Key Details:**
- Uses `net.Conn` internally (accessible as `c.nc`)
- Handles deadlines and context cancellation
- Validates message sizes
- Manages connection state

---

## Three Approaches for Traffic-Replay

### Approach 1: Use Experimental Packages (RECOMMENDED)

**Use the internal packages directly:**

```go
import (
    "go.mongodb.org/mongo-driver/v2/x/mongo/driver/wiremessage"
    "go.mongodb.org/mongo-driver/v2/x/mongo/driver/mnet"
    "go.mongodb.org/mongo-driver/v2/x/mongo/driver/topology"
)

// Parse recording packet
func parsePacket(recordingPacket []byte) (*Packet, error) {
    // Use wiremessage.ReadHeader to parse
    length, reqID, respTo, opcode, rem, ok := wiremessage.ReadHeader(recordingPacket)
    if !ok {
        return nil, fmt.Errorf("invalid wire message header")
    }

    // Detect legacy opcodes
    if isLegacyOpCode(opcode) {
        return nil, fmt.Errorf("unsupported opcode: %s", opcode)
    }

    return &Packet{
        OpCode:    opcode,
        RequestID: reqID,
        Data:      recordingPacket,
    }, nil
}

// Send via connection
func sendWireMessage(conn mnet.ReadWriteCloser, message []byte) error {
    return conn.Write(ctx, message)
}
```

**Pros:**
- ✅ Leverage tested, production code
- ✅ Handles compression, checksums automatically
- ✅ Connection management (auth, TLS, etc.)
- ✅ Actively maintained
- ✅ Less code to write

**Cons:**
- ⚠️ No backward compatibility guarantee
- ⚠️ API may change in future releases
- ⚠️ Not intended for external use

**Mitigation:**
- Vendor dependencies (use Go modules)
- Pin to specific driver version
- Document which version we depend on
- Test before upgrading driver

### Approach 2: Copy/Adapt Wire Protocol Code

**Extract and adapt code into our codebase:**

```go
// In traffic-replay/pkg/wiremessage/

// Copy OpCode constants and functions from driver
// Adapt as needed for our use case
// Maintain our own version
```

**Pros:**
- ✅ Full control over code
- ✅ No external dependency concerns
- ✅ Can simplify to only what we need
- ✅ Apache 2.0 license allows this

**Cons:**
- ❌ Must maintain wire protocol code ourselves
- ❌ No bug fixes/updates from MongoDB
- ❌ More initial work
- ❌ Risk of introducing bugs

### Approach 3: Pure Raw Socket Implementation

**Implement everything from scratch using `net.Conn`:**

```go
import "net"

func connect(address string) (net.Conn, error) {
    return net.Dial("tcp", address)
}

func writeMessage(conn net.Conn, message []byte) error {
    _, err := conn.Write(message)
    return err
}

func readMessage(conn net.Conn) ([]byte, error) {
    // Read 4-byte length prefix
    lenBuf := make([]byte, 4)
    if _, err := io.ReadFull(conn, lenBuf); err != nil {
        return nil, err
    }

    length := int32(binary.LittleEndian.Uint32(lenBuf))

    // Read remaining bytes
    message := make([]byte, length)
    copy(message[:4], lenBuf)
    if _, err := io.ReadFull(conn, message[4:]); err != nil {
        return nil, err
    }

    return message, nil
}
```

**Pros:**
- ✅ Zero external dependencies
- ✅ Complete control
- ✅ Simple, transparent implementation

**Cons:**
- ❌ Must handle auth manually
- ❌ Must handle TLS manually
- ❌ Must handle compression manually
- ❌ Must handle all error cases
- ❌ More code to write and maintain

---

## Recommendation

### **Use Approach 1: Experimental Packages**

**Rationale:**

1. **Wire protocol code is complex**
   - Compression handling (Snappy, Zlib, Zstd)
   - Checksum validation
   - OP_MSG sections (body + document sequences)
   - Error handling edge cases

2. **Authentication is non-trivial**
   - SCRAM-SHA-1, SCRAM-SHA-256
   - x.509 certificates
   - LDAP, Kerberos, OIDC
   - MongoDB-CR (legacy)

3. **Driver code is production-tested**
   - Used by thousands of applications
   - Well-tested edge cases
   - Active bug fixes

4. **Experimental warning is acceptable**
   - We can pin to specific version
   - Wire protocol is stable (unlikely to change)
   - Package has existed for years
   - Risk is manageable

5. **Apache 2.0 License**
   - If API changes, we can fork/copy
   - Legally clear to use

### Implementation Strategy

**Phase 1: MVP with Experimental Packages**

```go
// go.mod
require (
    go.mongodb.org/mongo-driver/v2 v2.0.0  // Pin specific version
)

// pkg/sender/connection.go
import (
    "go.mongodb.org/mongo-driver/v2/mongo"
    "go.mongodb.org/mongo-driver/v2/x/mongo/driver/wiremessage"
    "go.mongodb.org/mongo-driver/v2/x/mongo/driver/topology"
)

type Sender struct {
    client *mongo.Client
    // Use driver's connection management
}

func (s *Sender) SendWireMessage(ctx context.Context, message []byte) error {
    // 1. Validate message using wiremessage.ReadHeader
    // 2. Get connection from pool
    // 3. Write raw bytes using connection.Write()
    // 4. Optionally read response
}
```

**Phase 2: Isolation Layer**

```go
// Abstract wire protocol dependencies
type WireMessageParser interface {
    ParseHeader([]byte) (*Header, error)
    DetectOpCode([]byte) (OpCode, error)
}

// Use driver implementation
type DriverWireMessageParser struct{}

// If driver API changes, swap implementation
type CustomWireMessageParser struct{}
```

**Phase 3: Fallback Plan**

- Monitor driver releases for breaking changes
- If experimental package removed:
  - Copy code from last working version (Apache 2.0)
  - Maintain our own fork
  - Or switch to Approach 3 (raw sockets)

---

## Code Examples

### Example 1: Parse Recording Wire Message

```go
import "go.mongodb.org/mongo-driver/v2/x/mongo/driver/wiremessage"

func parseWireMessage(recordingMessage []byte) error {
    // Parse header
    length, reqID, respTo, opcode, body, ok := wiremessage.ReadHeader(recordingMessage)
    if !ok {
        return fmt.Errorf("invalid wire message header")
    }

    log.Printf("OpCode: %s, RequestID: %d, Length: %d bytes",
        opcode.String(), reqID, length)

    // Check for legacy opcodes
    switch opcode {
    case wiremessage.OpQuery,
         wiremessage.OpInsert,
         wiremessage.OpUpdate,
         wiremessage.OpDelete,
         wiremessage.OpGetMore,
         wiremessage.OpKillCursors:
        return fmt.Errorf("unsupported legacy opcode: %s", opcode)
    }

    // Handle supported opcodes
    switch opcode {
    case wiremessage.OpMsg:
        return handleOpMsg(body)
    case wiremessage.OpCompressed:
        return handleOpCompressed(body)
    default:
        return fmt.Errorf("unknown opcode: %d", opcode)
    }
}
```

### Example 2: Send Raw Wire Message

```go
import (
    "go.mongodb.org/mongo-driver/v2/mongo"
    "go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Replayer struct {
    client *mongo.Client
}

func NewReplayer(uri string) (*Replayer, error) {
    client, err := mongo.Connect(context.Background(),
        options.Client().ApplyURI(uri))
    if err != nil {
        return nil, err
    }

    return &Replayer{client: client}, nil
}

func (r *Replayer) SendRawMessage(ctx context.Context, wireMessage []byte) error {
    // Get a connection from the pool
    // This part requires accessing internal connection APIs
    // which we'll need to explore further

    // Option: Use driver's operation execution with custom operation
    // that sends raw bytes

    // This is the area where we'll need to either:
    // 1. Use internal APIs (topology.Connection)
    // 2. Or implement custom connection handling
}
```

### Example 3: Connection Management

```go
// If we need direct access to connections, we may need to use
// internal topology APIs or implement our own connection pool

import (
    "go.mongodb.org/mongo-driver/v2/x/mongo/driver/topology"
)

// This would require using internal packages more deeply
// We'd need to explore topology.Server and topology.Connection APIs
```

---

## Decision Matrix

| Criterion | Approach 1 (Experimental) | Approach 2 (Copy) | Approach 3 (Raw) |
|-----------|---------------------------|-------------------|------------------|
| **Development Speed** | ⭐⭐⭐ Fast | ⭐⭐ Moderate | ⭐ Slow |
| **Code Complexity** | ⭐⭐⭐ Low | ⭐⭐ Moderate | ⭐ High |
| **Maintenance Burden** | ⭐⭐ Low-Moderate | ⭐⭐ Moderate | ⭐ High |
| **Stability Risk** | ⭐⭐ Moderate | ⭐⭐⭐ Low | ⭐⭐⭐ Low |
| **Feature Completeness** | ⭐⭐⭐ High | ⭐⭐ Moderate | ⭐ Low |
| **Auth Support** | ⭐⭐⭐ Full | ⭐ Partial | ❌ None |
| **Compression Support** | ⭐⭐⭐ Full | ⭐⭐ Partial | ❌ None |
| **TLS Support** | ⭐⭐⭐ Full | ⭐⭐ Partial | ⭐ Manual |

**Recommended:** Approach 1 (Use Experimental Packages)

---

## Next Steps

1. **Prototype with experimental packages**
   - Test parsing recording wire messages
   - Test sending to MongoDB
   - Verify OpCode detection works

2. **Create abstraction layer**
   - Interface for wire protocol parsing
   - Interface for connection management
   - Makes switching approaches easier

3. **Document dependencies**
   - Pin exact driver version
   - Document which APIs we use
   - Create migration plan if APIs change

4. **Build MVP**
   - Focus on OP_MSG only
   - Fire-and-forget replay
   - Basic error handling

5. **Monitor driver releases**
   - Watch for changes to experimental packages
   - Be prepared to adapt if needed

---

## Conclusion

**The MongoDB Go driver DOES provide wire protocol capabilities**, though they're in experimental packages. This is actually **good news** for Traffic-Replay:

✅ **We can use the driver** - Saves significant development time
✅ **Production-tested code** - Handles edge cases we might miss
✅ **Full feature support** - Auth, TLS, compression all work
⚠️ **Some risk** - API may change, but risk is manageable

**Recommendation:** Proceed with Approach 1, using the experimental packages with proper version pinning and an abstraction layer for future-proofing.

---

## References

- **Go Driver Repository:** https://github.com/mongodb/mongo-go-driver
- **Wire Message Package:** `x/mongo/driver/wiremessage/wiremessage.go`
- **Connection Package:** `x/mongo/driver/mnet/connection.go`
- **Topology Package:** `x/mongo/driver/topology/connection.go`
- **Wire Protocol Spec:** https://www.mongodb.com/docs/manual/reference/mongodb-wire-protocol/

---

**Research Date:** 2025-11-04
**Conclusion:** Use experimental packages with version pinning
