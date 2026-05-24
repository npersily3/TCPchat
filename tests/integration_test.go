//go:build integration

// Run with: go test -tags integration -v -count=1 -timeout 120s TCPchat/tests
// To attach a debugger, use GoLand's "Integration Tests" run configuration in debug mode.
// The server binary is built with -gcflags="all=-N -l" so you can step into it.

package tests

import (
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"TCPchat/internal/shared"
)

func TestMain(m *testing.M) {
	bin, err := os.CreateTemp("", "tcpchat-test-*.exe")
	if err != nil {
		panic("create temp bin: " + err.Error())
	}
	bin.Close()
	testBin = bin.Name()
	defer os.Remove(testBin)

	// -a bypasses the build cache so source changes are always picked up.
	// Optimizations and inlining are disabled for debugger attachment.
	out, buildErr := exec.Command(
		"go", "build",
		"-a",
		"-gcflags", "all=-N -l",
		"-o", testBin,
		"TCPchat/cmd/tcpchat",
	).CombinedOutput()
	if buildErr != nil {
		panic("build failed:\n" + string(out) + "\n" + buildErr.Error())
	}

	os.Exit(m.Run())
}

// TestFirstConnect verifies the first user to connect receives no unsolicited messages.
func TestFirstConnect(t *testing.T) {
	cleanup := spawnServer(t, t.TempDir())
	defer cleanup()

	alice := connect(t, rand.Uint32(), "alice")
	defer alice.conn.Close()

	msgs := alice.drain(5, 500*time.Millisecond)
	if len(msgs) != 0 {
		t.Errorf("first user should receive no initial messages; got %d: %+v", len(msgs), msgs)
	}
}

// TestNoDoublePrint verifies that a joining user triggers exactly one notification per observer.
func TestNoDoublePrint(t *testing.T) {
	cleanup := spawnServer(t, t.TempDir())
	defer cleanup()

	alice := connect(t, rand.Uint32(), "alice")
	defer alice.conn.Close()
	alice.drain(10, 500*time.Millisecond)

	bob := connect(t, rand.Uint32(), "bob")
	defer bob.conn.Close()

	msgs := alice.drain(10, 2*time.Second)

	count := 0
	for _, m := range msgs {
		if m.SenderId == bob.id {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 message from bob, got %d: %+v", count, msgs)
	}
}

// TestNamePropagation verifies that NEW_USERNAME and NEW_USER_WHO_WAS_ONLINE carry the correct name.
func TestNamePropagation(t *testing.T) {
	cleanup := spawnServer(t, t.TempDir())
	defer cleanup()

	aliceID := rand.Uint32()
	alice := connect(t, aliceID, "alice")
	defer alice.conn.Close()
	alice.drain(10, 500*time.Millisecond)

	bobID := rand.Uint32()
	bob := connect(t, bobID, "bob")
	defer bob.conn.Close()

	aliceMsgs := alice.drain(5, 2*time.Second)
	var aliceGotName bool
	for _, m := range aliceMsgs {
		if m.SenderId == bobID && m.Payload.OPcode == shared.NEW_USERNAME {
			got := string(m.Payload.Contents)
			if got == "bob" {
				aliceGotName = true
			} else {
				t.Errorf("NEW_USERNAME for bob: want %q, got %q", "bob", got)
			}
		}
	}
	if !aliceGotName {
		t.Errorf("alice did not receive NEW_USERNAME for bob; got: %+v", aliceMsgs)
	}

	bobMsgs := bob.drain(5, 2*time.Second)
	var bobGotName bool
	for _, m := range bobMsgs {
		if m.SenderId == aliceID && m.Payload.OPcode == shared.NEW_USER_WHO_WAS_ONLINE {
			got := string(m.Payload.Contents)
			if got == "alice" {
				bobGotName = true
			} else {
				t.Errorf("NEW_USER_WHO_WAS_ONLINE for alice: want %q, got %q", "alice", got)
			}
		}
	}
	if !bobGotName {
		t.Errorf("bob did not receive NEW_USER_WHO_WAS_ONLINE for alice; got: %+v", bobMsgs)
	}
}

// TestJSONPersistence verifies that friend-lists survive a server restart (EXISTING_USER),
// and that deleting a user's JSON resets their view to NEW_USER_WHO_WAS_ONLINE.
func TestJSONPersistence(t *testing.T) {
	dir := t.TempDir()

	aliceID := uint32(10001)
	bobID := uint32(10002)

	// ── Session 1: establish Alice's friend-list with Bob ──────────────────
	cleanup := spawnServer(t, dir)

	alice := connect(t, aliceID, "alice")
	alice.drain(5, 500*time.Millisecond)

	bob := connect(t, bobID, "bob")
	bob.drain(5, 500*time.Millisecond)

	alice.drain(5, time.Second)

	t.Log("waiting 5 s for JSON flush (writeToPerClientJson fires at ~4 s)...")
	time.Sleep(5 * time.Second)

	cleanup()
	alice.conn.Close()
	bob.conn.Close()
	time.Sleep(200 * time.Millisecond)

	// ── Session 2: Alice's JSON has Bob → she receives EXISTING_USER ───────
	cleanup2 := spawnServer(t, dir)

	bob2 := connect(t, bobID, "bob")
	bob2.drain(5, 500*time.Millisecond)

	alice2 := connect(t, aliceID, "alice")
	aliceMsgs2 := alice2.drain(5, 2*time.Second)

	var gotExistingUser bool
	for _, m := range aliceMsgs2 {
		if m.SenderId == bobID && m.Payload.OPcode == shared.EXISTING_USER {
			gotExistingUser = true
		}
	}
	if !gotExistingUser {
		t.Errorf("session 2: alice expected EXISTING_USER for bob (JSON intact); got %+v", aliceMsgs2)
	}

	cleanup2()
	alice2.conn.Close()
	bob2.conn.Close()
	time.Sleep(200 * time.Millisecond)

	// ── Session 3: delete Alice's JSON → she receives NEW_USER_WHO_WAS_ONLINE ─
	jsonPath := filepath.Join(dir, fmt.Sprintf("friendsList%d.json", aliceID))
	if err := os.Remove(jsonPath); err != nil {
		t.Fatalf("remove alice JSON: %v", err)
	}

	cleanup3 := spawnServer(t, dir)
	defer cleanup3()

	bob3 := connect(t, bobID, "bob")
	defer bob3.conn.Close()
	bob3.drain(5, 500*time.Millisecond)

	alice3 := connect(t, aliceID, "alice")
	defer alice3.conn.Close()
	aliceMsgs3 := alice3.drain(5, 2*time.Second)

	var gotWasOnline bool
	for _, m := range aliceMsgs3 {
		if m.SenderId == bobID && m.Payload.OPcode == shared.NEW_USER_WHO_WAS_ONLINE {
			gotWasOnline = true
		}
	}
	if !gotWasOnline {
		t.Errorf("session 3: alice expected NEW_USER_WHO_WAS_ONLINE for bob (JSON deleted); got %+v", aliceMsgs3)
	}
}

// TestMessageOrdering verifies that messages from a single sender arrive in sent order.
func TestMessageOrdering(t *testing.T) {
	cleanup := spawnServer(t, t.TempDir())
	defer cleanup()

	alice := connect(t, rand.Uint32(), "alice")
	defer alice.conn.Close()

	bob := connect(t, rand.Uint32(), "bob")
	defer bob.conn.Close()

	alice.drain(10, time.Second)
	bob.drain(10, time.Second)
	time.Sleep(100 * time.Millisecond)
	alice.drain(5, 200*time.Millisecond)

	const n = 20
	for i := 0; i < n; i++ {
		alice.send(t, fmt.Sprintf("msg-%02d", i))
	}

	msgs := bob.drain(n+5, 5*time.Second)

	var received []string
	for _, m := range msgs {
		if m.SenderId == alice.id && m.Payload.OPcode == shared.DATA_MESSAGE {
			received = append(received, string(m.Payload.Contents))
		}
	}
	if len(received) != n {
		t.Errorf("expected %d messages from alice, got %d: %v", n, len(received), received)
		return
	}
	for i, text := range received {
		want := fmt.Sprintf("msg-%02d", i)
		if text != want {
			t.Errorf("message[%d]: want %q, got %q", i, want, text)
		}
	}
}

// TestMediumScale runs 15 concurrent users sending 5 messages each and verifies
// the server survives by accepting a probe connection with proper initial messages.
func TestMediumScale(t *testing.T) {
	cleanup := spawnServer(t, t.TempDir())
	defer cleanup()

	const numUsers = 15
	const msgsPerUser = 5

	users := make([]*rawClient, numUsers)
	for i := 0; i < numUsers; i++ {
		users[i] = connect(t, rand.Uint32(), fmt.Sprintf("user-%02d", i))
		time.Sleep(100 * time.Millisecond)
	}
	defer func() {
		for _, u := range users {
			u.conn.Close()
		}
	}()

	time.Sleep(2 * time.Second)
	for _, u := range users {
		u.drain(200, 100*time.Millisecond)
	}

	for i, u := range users {
		for j := 0; j < msgsPerUser; j++ {
			u.send(t, fmt.Sprintf("u%02d-msg%02d", i, j))
			time.Sleep(10 * time.Millisecond)
		}
	}

	time.Sleep(3 * time.Second)
	for _, u := range users {
		u.drain(500, 100*time.Millisecond)
	}

	probe := connect(t, rand.Uint32(), "probe")
	defer probe.conn.Close()
	probeStartMsgs := probe.drain(numUsers+5, 3*time.Second)
	if len(probeStartMsgs) < numUsers {
		t.Errorf("probe received %d initial messages, want at least %d (one per online user)",
			len(probeStartMsgs), numUsers)
	}
}

// TestServerWithPprof verifies that the -pprof flag starts an HTTP server.
func TestServerWithPprof(t *testing.T) {
	cleanup := spawnServer(t, t.TempDir(), "-pprof=:16060")
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
