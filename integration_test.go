//go:build integration

// This code is written by ai to test my program
package main

// Run with: go test -tags integration -v -count=1
// (count=1 prevents caching; tests share port :1000 so they must run sequentially)

import (
	"encoding/gob"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"
)

// testBin holds the path to the binary built in TestMain.
var testBin string

func TestMain(m *testing.M) {
	bin, err := os.CreateTemp("", "tcpchat-test-*.exe")
	if err != nil {
		panic("create temp bin: " + err.Error())
	}
	bin.Close()
	testBin = bin.Name()
	defer os.Remove(testBin)

	out, buildErr := exec.Command("go", "build", "-o", testBin, ".").CombinedOutput()
	if buildErr != nil {
		panic("build failed:\n" + string(out) + "\n" + buildErr.Error())
	}

	os.Exit(m.Run())
}

// spawnServer starts the server binary in a fresh temp directory so every test
// gets its own data.json and friends-list files. Returns a cleanup func.
func spawnServer(t *testing.T, extraArgs ...string) func() {
	t.Helper()
	dir := t.TempDir()

	args := append([]string{"-mode=server"}, extraArgs...)
	cmd := exec.Command(testBin, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("spawnServer: %v", err)
	}

	// Poll until the port is accepting connections (up to 3 s).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", "127.0.0.1"+port, 50*time.Millisecond)
		if err == nil {
			c.Close()
			return func() { cmd.Process.Kill() }
		}
		time.Sleep(20 * time.Millisecond)
	}

	cmd.Process.Kill()
	t.Fatal("spawnServer: did not become ready within 3 s")
	return nil
}

// rawClient wraps a TCP connection to the server with a gob encoder/decoder.
type rawClient struct {
	id   uint32
	name string
	conn net.Conn
	enc  *gob.Encoder
	dec  *gob.Decoder
}

// connect dials the server and sends the initial handshake message.
func connect(t *testing.T, name string) *rawClient {
	t.Helper()
	conn, err := net.Dial("tcp", "127.0.0.1"+port)
	if err != nil {
		t.Fatalf("connect %q: %v", name, err)
	}
	c := &rawClient{
		id:   rand.Uint32(),
		name: name,
		conn: conn,
		enc:  gob.NewEncoder(conn),
		dec:  gob.NewDecoder(conn),
	}
	if err := c.enc.Encode(Message{
		SenderId: c.id,
		Payload:  InternalMessageData{OPcode: DATA_MESSAGE, Contents: []byte(name)},
	}); err != nil {
		conn.Close()
		t.Fatalf("connect %q: handshake encode: %v", name, err)
	}
	return c
}

// drain reads up to n messages, stopping when the deadline expires or a decode
// error occurs. Safe to call multiple times.
func (c *rawClient) drain(n int, timeout time.Duration) []Message {
	deadline := time.Now().Add(timeout)
	var msgs []Message
	for len(msgs) < n {
		c.conn.SetReadDeadline(deadline)
		var m Message
		if err := c.dec.Decode(&m); err != nil {
			break
		}
		msgs = append(msgs, m)
	}
	c.conn.SetReadDeadline(time.Time{})
	return msgs
}

// send encodes a DATA_MESSAGE to the server.
func (c *rawClient) send(t *testing.T, text string) {
	t.Helper()
	if err := c.enc.Encode(Message{
		SenderId: c.id,
		Payload:  InternalMessageData{OPcode: DATA_MESSAGE, Contents: []byte(text)},
	}); err != nil {
		t.Fatalf("%s send: %v", c.name, err)
	}
}

func hasOpcode(msgs []Message, op int) bool {
	for _, m := range msgs {
		if m.Payload.OPcode == op {
			return true
		}
	}
	return false
}

// TestHandshake: a single client connects and receives the expected initial opcodes.
func TestHandshake(t *testing.T) {
	cleanup := spawnServer(t)
	defer cleanup()

	alice := connect(t, "alice")
	defer alice.conn.Close()

	msgs := alice.drain(5, 2*time.Second)

	if !hasOpcode(msgs, NEW_USERNAME) {
		t.Errorf("expected NEW_USERNAME in initial messages; got %+v", msgs)
	}
	if !hasOpcode(msgs, NEW_USER_ONLINE) {
		t.Errorf("expected NEW_USER_ONLINE in initial messages; got %+v", msgs)
	}
}

// TestNewUserNotification: when bob connects, alice should receive NEW_USER_ONLINE with bob's ID.
func TestNewUserNotification(t *testing.T) {
	cleanup := spawnServer(t)
	defer cleanup()

	alice := connect(t, "alice")
	defer alice.conn.Close()
	alice.drain(5, time.Second) // discard alice's own join messages

	bob := connect(t, "bob")
	defer bob.conn.Close()

	msgs := alice.drain(5, 2*time.Second)
	var gotBobOnline bool
	for _, m := range msgs {
		if m.Payload.OPcode == NEW_USER_ONLINE && m.SenderId == bob.id {
			gotBobOnline = true
		}
	}
	if !gotBobOnline {
		t.Errorf("alice did not receive NEW_USER_ONLINE for bob (id=%d); got: %+v", bob.id, msgs)
	}
}

// TestMessageBroadcast: alice sends a message; bob receives a DATA_MESSAGE with the exact text.
func TestMessageBroadcast(t *testing.T) {
	cleanup := spawnServer(t)
	defer cleanup()

	alice := connect(t, "alice")
	defer alice.conn.Close()
	alice.drain(5, time.Second)

	bob := connect(t, "bob")
	defer bob.conn.Close()
	bob.drain(5, time.Second)

	// Let bob's join propagate and drain alice's notification about it.
	time.Sleep(100 * time.Millisecond)
	alice.drain(5, 200*time.Millisecond)

	const payload = "hello from alice"
	alice.send(t, payload)

	msgs := bob.drain(5, 3*time.Second)
	var found bool
	for _, m := range msgs {
		if m.Payload.OPcode == DATA_MESSAGE && string(m.Payload.Contents) == payload {
			found = true
		}
	}
	if !found {
		t.Errorf("bob did not receive %q; got: %+v", payload, msgs)
	}
}

// TestServerWithPprof: server starts with -pprof and the endpoint becomes reachable.
func TestServerWithPprof(t *testing.T) {
	cleanup := spawnServer(t, "-pprof=:16060")
	defer cleanup()

	deadline := time.Now().Add(3 * time.Second)
	var up bool
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", "127.0.0.1:16060", 50*time.Millisecond)
		if err == nil {
			c.Close()
			up = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !up {
		t.Error("pprof HTTP server did not start within 3 s")
	}
}
