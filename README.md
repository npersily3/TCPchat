# TCPchat

A TCP chat application written in Go. The core networking, protocol design, and persistence were built and designed by hand as a learning exercise. The profiling harness, integration test suite, and project reorganization were implemented with AI assistance after the core was working.

---

## What Was Human-Built

The entire functional application — the thing that actually sends and receives messages — was designed and written from scratch:

- **The wire protocol** — a custom opcode system (`DATA_MESSAGE`, `NEW_USERNAME`, `NEW_USER_ONLINE`, `EXISTING_USER`, `NEW_USER_WHO_WAS_ONLINE`) carried inside gob-encoded `Message` structs over a raw TCP stream.
- **The server** (`internal/server/server.go`) — TCP listener, per-client goroutine pairs, a single global broadcast channel, and JSON persistence for the user/friend-list state.
- **The client** (`internal/client/client.go`) — TCP connection, goroutine-per-direction message pipeline, and local JSON state.

---

## Architecture

```
cmd/
  tcpchat/
    main.go         — entry point; parses flags, wires Config, calls server.Main or client.Main
    panic_hook.go   — debug panic handler (stack trace + INT3 breakpoint)

internal/
  shared/
    types.go        — Message, UserData, Client structs; opcode constants
    config.go       — Config struct and global Cfg var
  server/
    server.go       — TCP server, broadcast loop, JSON persistence
  client/
    client.go       — TCP client, browser UI (HTTP + SSE), JSON persistence

profiling/          — AI-implemented load harness (see below)
  main.go
  client.go

tests/              — AI-implemented integration test suite (see below)
  integration_test.go
  helpers_test.go
```

### Server (`internal/server/server.go`)

The server has one goroutine architecture per connected client plus a single shared broadcast goroutine:

```
net.Listen(:1000)
    └── handleConn(conn)          — one goroutine per accepted connection
            ├── perClientReceiver — reads gob messages from the TCP stream → globalChannel
            ├── perClientSender   — reads from a per-client buffered channel → encodes to TCP
            └── writeToPerClientJson — flushes the friend-list to disk every 4 s if dirty

main loop: for { msg := <-globalChannel; sendMessageToEveryOne(msg) }
```

On connection, `handleConn` performs the handshake (the first message is the username), loads or creates the user's `friendsList<id>.json`, sends the current online user list to the new client, then registers them and fires a `NEW_USER_ONLINE` broadcast before starting the goroutine pair.

`sendMessageToEveryOne` iterates every registered client and pushes the message onto their per-client channel. `perClientSender` then decides what opcode to actually deliver based on whether the receiver already knows the sender (`NEW_USERNAME` for first contact, `NEW_USER_ONLINE` if known but was offline, or the raw `DATA_MESSAGE` if fully initialized).

### Client (`internal/client/client.go`)

The client has three goroutines plus an HTTP server for the browser UI:

```
initClient()
    ├── receiveMessage()          — gob-decodes from TCP → receiverChannel
    ├── receivedMessageManager()  — processes receiverChannel; updates local state; pushes HTML snippets to broadcastToGUI
    ├── sendMessage()             — gob-encodes from senderChannel → TCP
    └── writeToClientSideJson()   — flushes data.json every 4 s if dirty

HTTP server (localhost:8080 by default)
    GET  /         — serves the full chat HTML/CSS/JS page
    GET  /events   — SSE stream; replays history to new tabs, then pushes live messages
    POST /send     — accepts a form field "m", pushes to senderChannel, echoes locally
```

The browser UI is a single self-contained HTML page embedded as a Go string constant. It connects to `/events` with `EventSource`, appends incoming HTML snippets to the message div, and posts to `/send` on Enter or button click. All message content is HTML-escaped before being injected into the DOM.

### Protocol opcodes

| Opcode | Value | Meaning |
|--------|-------|---------|
| `DATA_MESSAGE` | 0 | A normal chat message; `Contents` is the message text |
| `NEW_USERNAME` | 1 | Sender is a user this receiver has never seen; `Contents` is their name |
| `NEW_USER_ONLINE` | 2 | A known user came online; `Contents` is nil |
| `EXISTING_USER` | 3 | Sent on login: a user the server knows this client has seen before |
| `NEW_USER_WHO_WAS_ONLINE` | 4 | Sent on login: a user who was online before, but unknown to this client |

---

## AI-Implemented Systems

### Profiling harness (`profiling/`)

After the core app was working, the profiling package was written with AI assistance. It:

- Starts the server in-process (so all goroutines are captured in the same profile)
- Connects 30 simulated users with staggered join timing
- Runs a warm-up drain phase, then starts `pprof` CPU and `runtime/trace` recording
- Fires a hot-load phase (30 users × 20 messages at 5 ms intervals)
- Holds for 10 seconds so GoLand's sampler has time to collect enough data
- Stops profiling and writes `profiles/cpu.prof`, `profiles/mem.prof`, `profiles/trace.out`
- Runs a probe client to verify the server is still healthy after load

### Browser GUI (`internal/client/client.go` — `initGUI` and `chatHTML`)

The browser-based chat UI was added with AI assistance. It replaces an earlier terminal UI. The GUI is a self-contained HTTP server embedded in the client:

- Serves a single HTML page with a dark monospace style, scrollable message list, and text input.
- Pushes incoming messages to all open browser tabs via a Server-Sent Events stream (`/events`), with history replay for tabs opened mid-session.
- Accepts outgoing messages via a `POST /send` endpoint and echoes them locally so the sender sees their own messages without a round-trip through the server.
- All message content is HTML-escaped before being injected into the DOM.

### Entry point and flags (`cmd/tcpchat/main.go`)

The entry point and the full flag interface (`--mode`, `--addr`, `--data-dir`, `--pprof`, `--gui-port`, `--name`, `--headless`) were added with AI assistance to turn the single-purpose prototype into a configurable binary.

### Panic hook (`cmd/tcpchat/panic_hook.go`)

Shadows the built-in `panic` to print a full stack trace and fire an INT3 breakpoint so GoLand catches panics while debugging. Added with AI assistance.

### Integration tests (`tests/`)

The integration test suite was written with AI assistance to catch regressions found during debugging:

| Test | What it checks |
|------|---------------|
| `TestFirstConnect` | First user receives no unsolicited messages |
| `TestNoDoublePrint` | A joining user triggers exactly one notification per observer |
| `TestNamePropagation` | `NEW_USERNAME` and `NEW_USER_WHO_WAS_ONLINE` carry the correct name |
| `TestJSONPersistence` | Friend-lists survive a server restart; `EXISTING_USER` vs `NEW_USER_WHO_WAS_ONLINE` behavior |
| `TestMessageOrdering` | Messages from a single sender arrive in sent order |
| `TestMediumScale` | 15 concurrent users survive load; server accepts a probe connection afterward |
| `TestServerWithPprof` | The `-pprof` flag starts an HTTP listener |

---

## How to Run

### Start the server

```
go run ./cmd/tcpchat --mode=server
```

### Start a client

```
go run ./cmd/tcpchat --mode=client
```

Open `http://localhost:8080` in a browser. Type in the input box and press Enter or click Send.

On first launch the client will prompt for a username in the terminal. Subsequent runs load the saved `data.json`.

### Run multiple clients on the same machine

Each client needs its own data directory and GUI port:

```
go run ./cmd/tcpchat --mode=client --data-dir=./alice --gui-port=8081
go run ./cmd/tcpchat --mode=client --data-dir=./bob   --gui-port=8082
```

---

## Command-Line Flags

| Flag | Default | Applies to | Description |
|------|---------|------------|-------------|
| `--mode` | `client` | both | Run as `server` or `client` |
| `--addr` | `:1000` | both | TCP address to listen on (server) or connect to (client) |
| `--data-dir` | `.` | both | Directory for JSON persistence files |
| `--pprof` | _(off)_ | both | Start a `net/http/pprof` HTTP server on this address, e.g. `:6060` |
| `--gui-port` | `8080` | client | Port for the browser chat UI |
| `--name` | _(prompt)_ | client | Username; skips the interactive prompt when set |
| `--headless` | `false` | client | Disable the browser UI; write received messages to stdout (for scripting/testing) |

### Live pprof while the server is running

```
go run ./cmd/tcpchat --mode=server --pprof=:6060
go tool pprof -http=:8080 http://localhost:6060/debug/pprof/profile?seconds=10
```

### Run the integration tests

```
go test -tags integration -v -count=1 -timeout 120s TCPchat/tests
```

### Run the profiling harness (writes to `profiles/`)

```
go run ./profiling/
```

Then inspect the output:

```
go tool pprof -http=:8080 profiles/cpu.prof
go tool pprof -http=:8080 profiles/mem.prof
go tool trace profiles/trace.out
```

---

## What I Learned

### Go

This was my introduction to Go as a language. The project covers a lot of ground:

- **Goroutines and channels** — the server spawns a goroutine per client for both sending and receiving, with a shared global channel used to fan messages out to all connected users.
- **Structs and interfaces** — modeling users, messages, and payloads as typed structs.
- **Maps** — using `map[uint32]ClientInfo` to track connected clients by ID on the server, and `map[uint32]Client` on each client to track known users locally.
- **Standard library** — `net`, `os`, `encoding/json`, `encoding/gob`, `sync`, `sync/atomic`, and more.

### TCP

The core of the app is a raw TCP server built with Go's `net` package:

- The server listens on port 1000 with `net.Listen("tcp", ":1000")`.
- Each new connection is handled in its own goroutine via `go handleConn(conn)`.
- Messages are serialized over the wire using Go's `gob` encoder/decoder, which wraps the TCP stream.
- A custom opcode system is embedded in every message so the receiver knows what kind of payload it's dealing with — a simple, but easily expandable protocol built on top of TCP.
- TCP's guaranteed ordering is relied on so that the first message a client receives is always the current user list.

### JSON Files

Both the server and client persist state to disk using JSON:

- On first launch, a `data.json` file is created to store the user's ID and their local friends list (a map of user IDs to names).
- On subsequent launches, the file is read back in with `json.Unmarshal` so the user doesn't need to re-enter their name and already knows who other users are.
- The server maintains a per-user `friendsList<id>.json` to track which users each client has seen before, enabling the server to send a `NEW_USERNAME` opcode only when a client encounters someone they haven't seen yet.
