# Sender Package

The `sender` package provides functionality to send MongoDB commands extracted from traffic recordings to a live MongoDB instance. It uses the official MongoDB Go driver to execute commands via `Database.RunCommand()`.

## Overview

The sender package enables automated replay of recorded MongoDB traffic by:
1. Extracting BSON commands from recorded wire protocol packets
2. Cleaning internal driver/server fields (e.g., `$clusterTime`, `lsid`, `txnNumber`)
3. Sending commands to a target MongoDB instance
4. Capturing results, timing, and errors

## Architecture

### Core Types

- **`Sender`** - Manages MongoDB connections and sends commands
- **`Command`** - Represents an extracted MongoDB command with database and BSON document
- **`Result`** - Contains the outcome of sending a command (success/failure, response, duration)

### Key Features

- **Connection Management**: Automatic connection pooling via MongoDB Go driver
- **Internal Field Cleaning**: Removes driver/server metadata while preserving MongoDB operators
- **Error Handling**: Captures and reports errors with timing information
- **Timeout Support**: Optional command timeouts
- **Result Validation**: Checks both error status and MongoDB `ok` field

## Usage

### Basic Example

```go
package main

import (
    "context"
    "fmt"
    "github.com/fsnow/traffic-replay/pkg/sender"
    "github.com/fsnow/traffic-replay/pkg/reader"
    "io"
)

func main() {
    ctx := context.Background()

    // Connect to MongoDB
    snd, err := sender.New(ctx, "mongodb://localhost:27017")
    if err != nil {
        panic(err)
    }
    defer snd.Close()

    // Open recording
    rec, err := reader.NewRecordingReader("recording.bin")
    if err != nil {
        panic(err)
    }
    defer rec.Close()

    // Replay packets
    for {
        packet, err := rec.Next()
        if err == io.EOF {
            break
        }
        if err != nil {
            panic(err)
        }

        // Skip responses
        if !packet.IsRequest() {
            continue
        }

        // Extract command
        cmd, err := sender.ExtractCommand(packet)
        if err != nil {
            continue // Skip unparseable packets
        }

        // Send command
        result, err := snd.SendCommand(cmd.Database, cmd.Document)
        if err != nil {
            fmt.Printf("Error: %v\n", err)
            continue
        }

        fmt.Printf("✓ %s.%s (took %v)\n", cmd.Database, cmd.Name, result.Duration)
    }
}
```

### Command Extraction

The `ExtractCommand` function parses wire protocol packets and extracts:
- Command name (e.g., "insert", "find", "update")
- Database name
- BSON command document (cleaned of internal fields)

```go
cmd, err := sender.ExtractCommand(packet)
if err != nil {
    // Handle error
}

fmt.Printf("Command: %s on %s\n", cmd.Name, cmd.Database)
```

### Internal Field Cleaning

The following fields are automatically removed from commands:
- `$clusterTime` - Cluster time for causal consistency
- `$db` - Database name (redundant, already specified)
- `$readPreference` - Read preference from driver
- `lsid` - Logical session ID
- `txnNumber` - Transaction number
- `autocommit` - Transaction autocommit flag
- `startTransaction` - Transaction start flag
- `readConcern` - Read concern (driver-managed)
- `writeConcern` - Write concern (driver-managed)

MongoDB operators (e.g., `$set`, `$push`, `$match`) are preserved.

### Result Handling

The `Result` type provides detailed information about command execution:

```go
result, err := snd.SendCommand(db, command)
if err != nil {
    // Network error, connection error, etc.
    fmt.Printf("Error: %v\n", err)
}

if !result.IsOK() {
    // MongoDB returned ok: 0
    fmt.Printf("Command failed: %v\n", result.Response)
}

fmt.Printf("Success! Took %v\n", result.Duration)
```

### Timeouts

Use `SendCommandWithTimeout` for commands with specific time limits:

```go
result, err := snd.SendCommandWithTimeout(db, command, 5*time.Second)
if err != nil {
    // May be a timeout error
}
```

## Testing

### Unit Tests

```bash
go test ./pkg/sender -run TestCleanInternalFields
```

### Integration Tests

Integration tests require a running MongoDB instance:

```bash
MONGODB_URI=mongodb://localhost:27017 go test -v ./pkg/sender
```

The integration tests will:
- Create a test database (`traffic_replay_test`)
- Run insert, find, update, aggregate operations
- Clean up the test database

## Limitations

- **OP_MSG Only**: Only supports OP_MSG (opcode 2013) packets. Legacy opcodes are not supported.
- **No Transaction State**: Does not maintain transaction state across commands
- **No Cursor Management**: getMore commands will fail (cursors are session-specific)
- **No Response Validation**: Does not compare responses with recorded responses

## Future Enhancements

- Session management for multi-command transactions
- Cursor handling for getMore operations
- Response validation mode
- Parallel replay with goroutines
- Rate limiting and throttling
- Statistics and metrics collection

## See Also

- [`cmd/replay/`](../../cmd/replay/) - Automated replay tool using this package
- [`pkg/reader/`](../reader/) - Packet parsing and reading
- [MongoDB Wire Protocol](https://www.mongodb.com/docs/manual/reference/mongodb-wire-protocol/)
