# TCPchat

A terminal-based chat application written in Go. This project was built as a learning exercise to explore the Go programming language, raw TCP networking, and JSON-based persistence.

## What I Learned

### Go
This was my introduction to Go as a language. The project covers a lot of ground:

- **Goroutines and channels** — the server spawns a goroutine per client for both sending and receiving, with a shared global channel used to fan messages out to all connected users.
- **Structs and interfaces** — modeling users, messages, and payloads as typed structs.
- **Maps** — using `map[uint32]ClientInfo` to track connected clients by ID on the server, and `map[uint32]Client` on each client to track known users locally.
- **Standard library** — `net`, `os`, `encoding/json`, `encoding/gob`, `encoding/binary`, `sync`, `flag`, and more.
- **The `flag` package** — the same binary runs as either a server or client depending on a `--mode` flag.

### TCP
The core of the app is a raw TCP server built with Go's `net` package:

- The server listens on port 1000 with `net.Listen("tcp", ":1000")`.
- Each new connection is handled in its own goroutine via `go handleConn(conn)`.
- Messages are serialized over the wire using Go's `gob` encoder/decoder, which wraps the TCP stream.
- A custom opcode system (`DATA_MESSAGE`, `NEW_USERNAME`, `NEW_USER_ONLINE`) is embedded in every message so the receiver knows what kind of payload it's dealing with — a simple, but easily expandable protocol built on top of TCP.
- TCP's guaranteed ordering is relied on so that the first message a client receives is always the current user list.

### JSON Files
Both the server and client persist state to disk using JSON:

- On first launch, a `data.json` file is created to store the user's ID and their local friends list (a map of user IDs to names).
- On subsequent launches, the file is read back in with `json.Unmarshal` so the user doesn't need to re-enter their name and already knows who other users are.
- The server maintains a per-user `friendsList<id>.json` to track which users each client has seen before, enabling the server to send a `NEW_USERNAME` opcode only when a client encounters someone they haven't seen yet.

## How to Run

**Start the server:**
```
go run . --mode=server
```

**Start a client (in a separate terminal):**
```
go run . --mode=client
```

The client uses a terminal UI ([tview](https://github.com/rivo/tview)) with a scrollable message pane and an input field at the bottom.

## Architecture

```
main.go       — entry point, routes to serverMain() or clientMain() via --mode flag
server.go     — TCP listener, per-client goroutines, message fanout, JSON persistence
client.go     — TCP connection, message send/receive goroutines, terminal UI (tview)
utils.go      — shared types (Message, UserData, Client) and opcode constants
panic_hook.go — panic handling utilities
```
