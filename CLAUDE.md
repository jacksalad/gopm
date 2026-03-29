# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
go build -o gopm.exe ./cmd/gopm/
go run ./cmd/gopm/ -mode server -port 9000 -verbose
go run ./cmd/gopm/ -mode client -server 127.0.0.1:9000 -local 8080 -map 8080 -verbose
```

No external dependencies — pure Go standard library only. No Makefile, no tests, no CI yet.

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

- `cmd/gopm/main.go` — CLI flag parsing, dispatches to server or client
- `internal/protocol/` — Message structs (`RegisterReq`, `NewConnMsg`, `JoinReq`, etc.) and JSON-line codec (`ReadMessage`/`WriteMessage`)
- `internal/server/` — Server core: `mappings` map (map_port→Mapping), `pending` map (conn_id→PendingConn). Each mapping starts its own listener on `:map_port`. Pending connections have a 10-second timeout.
- `internal/client/` — Client core: connect→register→read loop + heartbeat goroutine. `handleNewConn` opens a data tunnel per visitor. Retry with backoff (1s→2s→5s→10s) when `-retry` is set.
- `internal/common/` — `PipeConns` (bidirectional `io.Copy` bridge), `NormalizeLocalAddr`, `GenerateConnID`, logging utilities

### Key Timeouts

| Parameter | Value |
|---|---|
| TCP dial timeout | 5s |
| Pending join wait | 10s |
| Ping interval | 15s |
| Control connection timeout | 45s (no pong) |

### Protocol

JSON line protocol (`\n`-delimited). All messages have a `"type"` field. After a successful `join`/`join_ok` exchange, the data connection switches to raw TCP passthrough — no more JSON framing.
