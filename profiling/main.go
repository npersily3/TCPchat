// ============================================================
// HOW TO PROFILE THIS IN GOLAND
// ============================================================
//
// There are two separate workflows. Use Method A for GoLand's
// built-in flame graphs. Use Method B for pprof / trace files.
//
// ------------------------------------------------------------
// METHOD A — GoLand built-in CPU profiler (flame graphs)
// ------------------------------------------------------------
//  1. In the top-right run config dropdown, select "Profiler".
//  2. Click the dropdown arrow next to the Run button (▶) and
//     choose "Profile 'Profiler'" (the icon looks like a clock).
//     Alternatively: Run menu → Profile 'Profiler'.
//  3. The program runs and GoLand samples the CPU automatically.
//  4. When it finishes, GoLand opens the "CPU Profiler" tab at
//     the bottom with a flame graph and call tree.
//  5. No output files are needed — GoLand handles everything.
//
//  Tip: the hot load phase sleeps at the end so GoLand has time
//  to collect enough samples. Do not reduce that sleep.
//
// ------------------------------------------------------------
// METHOD B — pprof + trace files (more detail, Go-native)
// ------------------------------------------------------------
//  1. Run the "Profiler" config normally (▶, not the profiler).
//  2. Three files are written to profiles/ in the project root:
//       profiles/cpu.prof   — CPU hotspots
//       profiles/mem.prof   — heap allocations at end of run
//       profiles/trace.out  — goroutine scheduling, GC, network waits
//
//  View cpu.prof or mem.prof as a flame graph in your browser:
//    go tool pprof -http=:8080 profiles/cpu.prof
//    go tool pprof -http=:8080 profiles/mem.prof
//
//  View the execution trace (goroutine timeline, GC, blocking):
//    go tool trace profiles/trace.out
//
//  Open pprof files inside GoLand (2023.1+):
//    Right-click profiles/cpu.prof → Open In → Profiler
//
// ============================================================

package main

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"time"

	"TCPchat/internal/server"
	"TCPchat/internal/shared"
)

func main() {
	dir, err := os.MkdirTemp("", "tcpchat-profiler-*")
	if err != nil {
		log.Fatal("MkdirTemp:", err)
	}
	defer os.RemoveAll(dir)
	shared.Cfg.DataDir = dir

	// Run server in-process so the profiler captures all server goroutines.
	go server.Main()

	deadline := time.Now().Add(3 * time.Second)
	var ready bool
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", "127.0.0.1"+shared.Port, 50*time.Millisecond)
		if err == nil {
			c.Close()
			ready = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		log.Fatal("server did not become ready within 3 s")
	}

	const numUsers = 30
	const msgsPerUser = 20

	users := make([]*profClient, numUsers)
	for i := 0; i < numUsers; i++ {
		users[i] = profConnect(rand.Uint32(), fmt.Sprintf("user-%02d", i))
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

	// ---- start profiling just before the hot phase ----

	if err := os.MkdirAll("profiles", 0755); err != nil {
		log.Fatal("MkdirAll profiles:", err)
	}

	cpuFile, err := os.Create("profiles/cpu.prof")
	if err != nil {
		log.Fatal("create cpu.prof:", err)
	}
	if err := pprof.StartCPUProfile(cpuFile); err != nil {
		log.Fatal("StartCPUProfile:", err)
	}

	traceFile, err := os.Create("profiles/trace.out")
	if err != nil {
		log.Fatal("create trace.out:", err)
	}
	if err := trace.Start(traceFile); err != nil {
		log.Fatal("trace.Start:", err)
	}

	// Hot load phase — profiler samples here.
	for i, u := range users {
		for j := 0; j < msgsPerUser; j++ {
			u.sendMsg(fmt.Sprintf("u%02d-msg%02d", i, j))
			time.Sleep(5 * time.Millisecond)
		}
	}

	// Hold steady so GoLand's sampler (Method A) collects enough data.
	time.Sleep(10 * time.Second)
	for _, u := range users {
		u.drain(1000, 100*time.Millisecond)
	}

	// ---- stop profiling and flush files ----

	pprof.StopCPUProfile()
	cpuFile.Close()

	trace.Stop()
	traceFile.Close()

	// Heap profile — snapshot after the load has settled.
	memFile, err := os.Create("profiles/mem.prof")
	if err != nil {
		log.Fatal("create mem.prof:", err)
	}
	runtime.GC()
	if err := pprof.WriteHeapProfile(memFile); err != nil {
		log.Fatal("WriteHeapProfile:", err)
	}
	memFile.Close()

	log.Println("profile files written: profiles/cpu.prof  profiles/mem.prof  profiles/trace.out")

	probe := profConnect(rand.Uint32(), "probe")
	defer probe.conn.Close()
	probeStartMsgs := probe.drain(numUsers+5, 5*time.Second)
	if len(probeStartMsgs) < numUsers {
		log.Printf("WARNING: probe received %d initial messages, want at least %d", len(probeStartMsgs), numUsers)
	} else {
		log.Printf("server healthy: probe received %d messages", len(probeStartMsgs))
	}

	log.Println("profiling run complete")
}
