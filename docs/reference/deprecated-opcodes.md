# MongoDB Wire Protocol OpCodes: Deprecated vs. Current

**Date:** 2025-10-31
**Updated:** 2025-11-04

## Overview

MongoDB's wire protocol has evolved significantly over time. Many opcodes that Mongoreplay supported are now deprecated or removed from modern MongoDB versions.

**Traffic-Replay targets MongoDB 7.0+ only**, which exclusively uses OP_MSG and OP_COMPRESSED opcodes. Legacy opcodes (OP_QUERY, OP_INSERT, etc.) were removed in MongoDB 5.1 and are not supported.

## OpCode Evolution

### Legacy OpCodes (Mostly Deprecated)

| OpCode | Value | Name | Status | Deprecated In | Notes |
|--------|-------|------|--------|---------------|-------|
| OP_REPLY | 1 | Reply | **Deprecated** | 3.6+ | Replaced by OP_MSG replies |
| OP_UPDATE | 2001 | Update | **Removed** | 5.1+ | Use OP_MSG with `update` command |
| OP_INSERT | 2002 | Insert | **Removed** | 5.1+ | Use OP_MSG with `insert` command |
| OP_RESERVED | 2003 | Reserved | **Removed** | Always | Was OP_GET_BY_OID, never widely used |
| OP_QUERY | 2004 | Query | **Removed** | 5.1+ | Use OP_MSG with `find` command |
| OP_GET_MORE | 2005 | GetMore | **Removed** | 5.1+ | Use OP_MSG with `getMore` command |
| OP_DELETE | 2006 | Delete | **Removed** | 5.1+ | Use OP_MSG with `delete` command |
| OP_KILL_CURSORS | 2007 | KillCursors | **Removed** | 5.1+ | Use OP_MSG with `killCursors` command |
| OP_COMMAND | 2010 | Command | **Deprecated** | 3.6+ | Superseded by OP_MSG |
| OP_COMMANDREPLY | 2011 | CommandReply | **Deprecated** | 3.6+ | Superseded by OP_MSG |

### Current OpCodes

| OpCode | Value | Name | Status | Introduced | Notes |
|--------|-------|------|--------|------------|-------|
| OP_COMPRESSED | 2012 | Compressed | **Active** | 3.4+ | Wraps other opcodes with compression |
| OP_MSG | 2013 | Message | **Active** | 3.6+ | **Primary opcode for all operations** |

## MongoDB Version Timeline

### MongoDB 3.6 (Nov 2017)
- Introduced **OP_MSG** as the new unified wire protocol message
- OP_QUERY, OP_INSERT, OP_UPDATE, OP_DELETE still supported but deprecated
- OP_MSG supports both commands and replies in a single opcode

### MongoDB 4.0 (Jun 2018)
- Legacy opcodes still supported for backward compatibility
- Official drivers begin migrating to OP_MSG

### MongoDB 5.0 (Jul 2021)
- Legacy opcodes deprecated but still functional
- Documentation recommends OP_MSG for all new development

### MongoDB 5.1 (Oct 2021)
- **Legacy opcodes removed** from server
- Only OP_MSG and OP_COMPRESSED supported
- Attempts to use legacy opcodes result in error

### MongoDB 6.0 (Jul 2022)
- Continues OP_MSG and OP_COMPRESSED only
- **EOL: July 31, 2025** - Not supported by Traffic-Replay

### MongoDB 7.0 (Aug 2023) ✅ **Minimum Supported Version**
- OP_MSG and OP_COMPRESSED only
- Traffic-Replay targets this version as minimum
- **Support until August 31, 2027**

### MongoDB 8.0+ (Current)
- OP_MSG and OP_COMPRESSED only
- Introduces new minor release support model (8.1, 8.2, 8.3, etc.)
- **Support until October 31, 2029** (8.x series)

## OP_MSG Structure

OP_MSG is a flexible, extensible message format that handles:
- Commands (requests)
- Replies (responses)
- Document sequences (batch operations)
- Metadata flags

### Basic Format

```
struct OP_MSG {
    MsgHeader header;           // Standard message header
    uint32 flagBits;           // Message flags
    Sections sections[];        // 1 or more sections
    optional<uint32> checksum; // Optional CRC-32C checksum
}
```

### Sections

**Section 0 (Body):**
- BSON document with command and arguments
- Required, always present

**Section 1 (Document Sequence):**
- Sequence of BSON documents
- Used for batch operations (inserts, updates, etc.)
- Optional, more efficient than embedding in Section 0

### Flag Bits

```
#define checksumPresent     (1 << 0)
#define moreToCome          (1 << 1)  // Don't wait for reply
#define exhaustAllowed      (1 << 16) // Server may send multiple replies
```

## Implications for Traffic-Replay

### Target MongoDB Version: 7.0+ Only

**Traffic-Replay exclusively targets MongoDB 7.0 and later**, which are the only officially supported MongoDB versions as of November 2025:
- MongoDB 7.0 (support until August 31, 2027)
- MongoDB 8.x (support until October 31, 2029)

Both versions **only support OP_MSG and OP_COMPRESSED opcodes**.

### Important Context: The Chicken-and-Egg Problem

**A key insight:** MongoDB's traffic recording feature has been largely undocumented and unused because **there was no replayer tool** until Traffic-Replay. This creates a chicken-and-egg situation:

- 🐔 Traffic recording exists in MongoDB Server
- 🥚 But it's undocumented because there's no replayer
- 🐣 **We're building the replayer** (solving the egg problem)
- 🐔 But there are few existing recordings to test with (the chicken problem)

**Implications for Traffic-Replay:**

- ✅ **Very few legacy recordings exist in the wild**
- ✅ **Most recordings will be created AFTER Traffic-Replay is available**
- ✅ **New recordings will come from currently supported MongoDB versions (7.0+)**
- ✅ **Modern MongoDB (3.6+) already uses OP_MSG exclusively**
- ⚠️ **Limited scope for real-world testing** - we must create our own test recordings
- ⚠️ **Cannot anticipate all edge cases** - real-world usage may reveal unexpected scenarios

Therefore, **legacy opcode support is a low priority**. The vast majority of recordings will naturally contain only OP_MSG and OP_COMPRESSED.

**Testing Strategy:**
- Create synthetic recordings from MongoDB 7.0+ for testing
- Focus on modern opcode support (OP_MSG, OP_COMPRESSED)
- Add defensive checks for legacy opcodes (unlikely but possible)
- Iterate based on real-world usage after launch

### Edge Case: Legacy Recordings

**In rare cases, recordings may contain legacy opcodes:**
- ❌ Legacy opcodes (OP_QUERY, OP_INSERT, etc.) - only if recorded from old/unsupported MongoDB versions
- ❌ Likely only from internal/experimental use before Traffic-Replay existed

**Target MongoDB servers (7.0+):**
- Only accept OP_MSG and OP_COMPRESSED
- Will **reject legacy opcodes with error**

### Approach: Detect and Skip (Defensive Programming)

Since Traffic-Replay targets **only MongoDB 7.0+** and legacy recordings are not expected to be common, legacy opcode handling is **defensive programming** rather than a core requirement.

**Phase 1 (MVP):**
1. ✅ **Support OP_MSG and OP_COMPRESSED only** (99+% of expected use cases)
2. ✅ **Detect legacy opcodes during parsing** (defensive check)
3. ✅ **Provide clear error message:**
   ```
   Error: Recording contains unsupported opcode OP_QUERY (2004).

   This opcode was removed in MongoDB 5.1.
   Traffic-Replay targets MongoDB 7.0+ only, which does not support legacy opcodes.

   This is unexpected - most recordings should contain only OP_MSG.
   This recording may be from MongoDB ≤5.0 or experimental internal use.

   Options:
   - Re-record from a currently supported MongoDB version (7.0+)
   - Use --skip-unsupported to skip legacy operations (may affect replay accuracy)
   ```
4. ✅ **Optional flag:** `--skip-unsupported` to skip legacy operations with warning

**Phase 2+ (Not Planned):**
- Opcode translation (OP_QUERY → OP_MSG, etc.) is **not planned**
- Rationale:
  - **Very few recordings are expected to have legacy opcodes**
  - Adds significant complexity for minimal real-world benefit
  - MongoDB 3.6+ (Nov 2017) already uses OP_MSG exclusively
  - Users can simply re-record from a modern MongoDB instance

## OpCode Detection

### In Traffic Recording Binary Format

The opcode is in the MongoDB wire protocol message header:

```
struct MsgHeader {
    int32  messageLength;  // Total message size
    int32  requestID;      // Identifier for this message
    int32  responseTo;     // RequestID from original request
    int32  opCode;         // ← OpCode here (4 bytes, little-endian)
};
```

In recording packets, the `message` field contains the full wire protocol bytes, including this header.

### Detection Logic

```go
func detectOpCode(messageBytes []byte) (OpCode, error) {
    if len(messageBytes) < 16 {
        return 0, fmt.Errorf("message too short for header")
    }

    // OpCode is at offset 12 (after messageLength, requestID, responseTo)
    opCode := binary.LittleEndian.Uint32(messageBytes[12:16])
    return OpCode(opCode), nil
}

func isLegacyOpCode(opCode OpCode) bool {
    legacyCodes := map[OpCode]bool{
        2001: true, // OP_UPDATE
        2002: true, // OP_INSERT
        2004: true, // OP_QUERY
        2005: true, // OP_GET_MORE
        2006: true, // OP_DELETE
        2007: true, // OP_KILL_CURSORS
        2010: true, // OP_COMMAND
        2011: true, // OP_COMMANDREPLY
    }
    return legacyCodes[opCode]
}
```

## Testing Considerations

### Test Cases Needed

1. **Modern recording (OP_MSG only)**
   - Should replay successfully to MongoDB 7.0+
   - Should replay successfully to MongoDB 8.x

2. **Legacy recording (with deprecated opcodes)**
   - Should detect legacy opcodes (OP_QUERY, OP_INSERT, etc.)
   - Should provide clear error message explaining incompatibility
   - With `--skip-unsupported`: should skip legacy ops and continue with OP_MSG messages
   - Should track and report skipped operation count

3. **Compressed messages (OP_COMPRESSED)**
   - Should handle OP_COMPRESSED wrapper correctly
   - Should replay compressed messages as-is (no decompression needed)
   - Server handles decompression automatically

4. **Unknown opcodes**
   - Should handle gracefully without crashing
   - Should provide clear error message with opcode value
   - Should suggest checking recording validity

## References

- **MongoDB Wire Protocol Spec:** https://www.mongodb.com/docs/manual/reference/mongodb-wire-protocol/
- **OP_MSG Spec:** https://github.com/mongodb/specifications/blob/master/source/message/OP_MSG.rst
- **MongoDB 5.1 Release Notes:** https://www.mongodb.com/docs/v5.1/release-notes/5.1/#removed-commands
- **Mongoreplay OpCodes:** See `mongoreplay/opcode.go` for reference implementation

---

## Key Takeaways

1. **Target Version:** Traffic-Replay targets **MongoDB 7.0+ exclusively**, supporting only OP_MSG and OP_COMPRESSED opcodes.

2. **The Chicken-and-Egg Problem:** MongoDB's traffic recording feature has been undocumented because there was no replayer. We're building the replayer, but this means:
   - Very few existing recordings to test with
   - Limited real-world data for validation
   - Must create synthetic test recordings
   - Real edge cases will emerge after launch

3. **Legacy Opcode Support:** Legacy opcodes (OP_QUERY, OP_INSERT, etc.) are **not expected** in real-world recordings and will be handled defensively with detection and skip capabilities. Opcode translation is not planned.

4. **Forward Focus:** The design prioritizes modern MongoDB deployments (7.0+) and expects recordings to be created going forward, naturally containing only modern opcodes.
