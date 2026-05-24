//go:build integration

package tests

import (
	"encoding/gob"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"TCPchat/internal/shared"
)

// testBin is set by TestMain in integration_test.go.
var testBin string

type rawClient struct {
	id   uint32
	name string
	conn net.Conn
	enc  *gob.Encoder
	dec  *gob.Decoder
}

func connect(t *testing.T, id uint32, name string) *rawClient {
	t.Helper()
	conn, err := net.Dial("tcp", "127.0.0.1"+shared.Port)
	if err != nil {
		t.Fatalf("connect %q: %v", name, err)
	}
	c := &rawClient{
		id:   id,
		name: name,
		conn: conn,
		enc:  gob.NewEncoder(conn),
		dec:  gob.NewDecoder(conn),
	}
	if err := c.enc.Encode(shared.Message{
		SenderId: id,
		Payload:  shared.InternalMessageData{OPcode: shared.DATA_MESSAGE, Contents: []byte(name)},
	}); err != nil {
		conn.Close()
		t.Fatalf("connect %q: handshake: %v", name, err)
	}
	return c
}

func (c *rawClient) drain(n int, timeout time.Duration) []shared.Message {
	deadline := time.Now().Add(timeout)
	var msgs []shared.Message
	for len(msgs) < n {
		c.conn.SetReadDeadline(deadline)
		var m shared.Message
		if err := c.dec.Decode(&m); err != nil {
			break
		}
		msgs = append(msgs, m)
	}
	c.conn.SetReadDeadline(time.Time{})
	return msgs
}

func (c *rawClient) send(t *testing.T, text string) {
	t.Helper()
	if err := c.enc.Encode(shared.Message{
		SenderId: c.id,
		Payload:  shared.InternalMessageData{OPcode: shared.DATA_MESSAGE, Contents: []byte(text)},
	}); err != nil {
		t.Fatalf("%s send: %v", c.name, err)
	}
}

func spawnServer(t *testing.T, dir string, extraArgs ...string) func() {
	t.Helper()
	args := append([]string{"-mode=server", "-data-dir=" + dir}, extraArgs...)
	cmd := exec.Command(testBin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawnServer: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", "127.0.0.1"+shared.Port, 50*time.Millisecond)
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
