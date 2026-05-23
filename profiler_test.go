//go:build profiler

// Run with: go test -tags profiler -v -count=1 -timeout 300s
// While the test is running, connect to http://127.0.0.1:16061/debug/pprof/ to inspect the server.

package main

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	bin, err := os.CreateTemp("", "tcpchat-prof-*.exe")
	if err != nil {
		panic("create temp bin: " + err.Error())
	}
	bin.Close()
	testBin = bin.Name()
	defer os.Remove(testBin)

	// -a bypasses the build cache so source changes are always picked up.
	out, buildErr := exec.Command("go", "build", "-a", "-o", testBin, ".").CombinedOutput()
	if buildErr != nil {
		panic("build failed:\n" + string(out) + "\n" + buildErr.Error())
	}

	os.Exit(m.Run())
}

// TestProfileMediumScale runs 30 concurrent users each sending 20 messages under pprof.
// The pprof endpoint stays live for the duration so you can capture CPU/heap profiles.
func TestProfileMediumScale(t *testing.T) {
	const pprofAddr = "127.0.0.1:16061"

	cleanup := spawnServer(t, t.TempDir(), "-pprof="+pprofAddr)
	defer cleanup()

	deadline := time.Now().Add(3 * time.Second)
	var pprofUp bool
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", pprofAddr, 50*time.Millisecond)
		if err == nil {
			c.Close()
			pprofUp = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !pprofUp {
		t.Fatal("pprof did not start within 3 s")
	}
	t.Logf("pprof live at http://%s/debug/pprof/", pprofAddr)

	const numUsers = 30
	const msgsPerUser = 20

	users := make([]*rawClient, numUsers)
	for i := 0; i < numUsers; i++ {
		users[i] = connect(t, rand.Uint32(), fmt.Sprintf("user-%02d", i))
		time.Sleep(80 * time.Millisecond)
	}
	defer func() {
		for _, u := range users {
			u.conn.Close()
		}
	}()

	time.Sleep(2 * time.Second)
	for _, u := range users {
		u.drain(500, 100*time.Millisecond)
	}

	for i, u := range users {
		for j := 0; j < msgsPerUser; j++ {
			u.send(t, fmt.Sprintf("u%02d-msg%02d", i, j))
			time.Sleep(5 * time.Millisecond)
		}
	}

	time.Sleep(5 * time.Second)
	for _, u := range users {
		u.drain(1000, 100*time.Millisecond)
	}

	// Verify pprof endpoint is still reachable after the load.
	resp, err := http.Get("http://" + pprofAddr + "/debug/pprof/")
	if err != nil {
		t.Errorf("pprof not reachable after load: %v", err)
	} else {
		resp.Body.Close()
	}

	// A new probe connection verifies the server is alive and still routing properly.
	probe := connect(t, rand.Uint32(), "probe")
	defer probe.conn.Close()
	probeStartMsgs := probe.drain(numUsers+5, 5*time.Second)
	if len(probeStartMsgs) < numUsers {
		t.Errorf("probe received %d initial messages, want at least %d (one per online user)",
			len(probeStartMsgs), numUsers)
	}
}
