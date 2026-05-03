# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
go build -o gopm.exe ./cmd/gopm/
go run ./cmd/gopm/ -mode server -port 9000 -verbose
go run ./cmd/gopm/ -mode client -server 127.0.0.1:9000 -local 8080 -map 8080 -verbose
```

No external dependencies — pure Go standard library only. No Makefile, no CI yet.

## Architecture

**gopm** is a reverse TCP tunnel tool (intranet penetration). One binary acts as either server or client via `-mode` flag. The PRD.md is the authoritative protocol and design spec.

### Two Connection Model (critical to understand)

All client→server connections go to a **single control port** (e.g. `:9000`). The first JSON message determines the connection's role:

- **Control connection**: First message is `register`. Stays open for the client's lifetime. Carries `ping/pong` heartbeats and `new_conn` notifications.
- **Data connection**: First message is `join`. Short-lived. After `join_ok`, the TCP stream becomes raw bidirectional traffic (no more JSON).

### Traffic Flow

```
Visitor → server:map_port → server generates conn_id → notifies client via control conn
→ client opens new data conn to server:9000 → sends join(conn_id)
→ server bridges visitor_conn ↔ data_conn
→ client bridges data_conn ↔ local_service_conn
```

### Package Layout

- `cmd/gopm/main.go` — CLI flag parsing, signal handling (SIGINT/SIGTERM), optional `-timeout` for auto shutdown, dispatches to server or client
- `internal/protocol/` — Message structs (`RegisterReq`, `NewConnMsg`, `JoinReq`, etc.) and JSON-line codec (`ReadMessage`/`WriteMessage`/`DecodeMessage`). Messages are limited to 4 KiB (`MaxMessageSize = 4096`) to prevent memory exhaustion attacks.
- `internal/server/` — Server core: `mappings` map (map_port→Mapping), `pending` map (conn_id→PendingConn). Each mapping starts its own listener on `:map_port`. Pending connections have a 10-second timeout. Writer concurrency is protected by `Mapping.writeMu`. A `healthCheck` goroutine per mapping enforces the 45-second heartbeat timeout. Supports graceful shutdown via `Shutdown()` method.
- `internal/client/` — Client core: connect→register→read loop + heartbeat goroutine. `handleNewConn` opens a data tunnel per visitor. Retry with backoff (1s→2s→5s→10s) when `-retry` is set. `Stop()` is protected by `sync.Once` to prevent double-close panic.
- `internal/common/` — `PipeConns`/`PipeRW` (bidirectional `io.Copy` bridge with error logging), `NormalizeLocalAddr`, `GenerateConnID` (with `crypto/rand` fallback), thread-safe logging utilities (`atomic.Bool` for verbose flag)

### Key Timeouts

| Parameter | Value |
|---|---|
| TCP dial timeout | 5s |
| Pending join wait | 10s |
| Ping interval | 15s |
| Heartbeat timeout | 45s (server removes mapping if no message received) |
| Message size limit | 4096 bytes |

### Concurrency Safety

- **`Mapping.writeMu`** — A `sync.Mutex` on each `Mapping` protects `Writer` from concurrent writes. All `WriteMessage` calls on the control connection writer (pong, new_conn, register_ok) are serialized through this mutex.
- **`Server.mu`** — A `sync.RWMutex` protecting the `mappings` and `pending` maps. `removeMapping` delegates to `removeMappingLocked` (caller must hold lock) to support batch cleanup during shutdown.
- **`Client.stopOnce`** — A `sync.Once` protecting `Stop()` from double-close panic on `done` channel.
- **`common.verbose`** — An `atomic.Bool` for thread-safe read/write of the verbose logging flag.

### Graceful Shutdown

Both server and client listen for `SIGINT`/`SIGTERM` via `os/signal`:

- **Server**: `Shutdown()` closes the listener (unblocking `Accept`), then iterates all mappings calling `removeMappingLocked` to clean up listeners, control connections, and pending connections.
- **Client**: `Stop()` closes the `done` channel (unblocking retry waits and heartbeat) and closes the control connection (unblocking the read loop).

### Protocol

JSON line protocol (`\n`-delimited). All messages have a `"type"` field. After a successful `join`/`join_ok` exchange, the data connection switches to raw TCP passthrough — no more JSON framing. Messages exceeding 4096 bytes are rejected with an error.

### Error Responses

Error messages use the proper protocol structs (`RegisterError`, `JoinError`) instead of generic maps, ensuring consistent serialization and correct field presence (e.g., `JoinError.ConnID`).
