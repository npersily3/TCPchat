package main

import (
	"flag"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
)

type Config struct {
	Addr      string
	DataDir   string
	PProfAddr string
	Headless  bool
	Name      string
	GUIPort   string
}

const port = ":1000"

// cfg is the active runtime configuration. All files in the package can read it.
var cfg Config

func main() {
	mode := flag.String("mode", "client", "run as 'server' or 'client'")
	addr := flag.String("addr", port, "TCP address to listen on (server) or connect to (client)")
	dataDir := flag.String("data-dir", ".", "directory for JSON persistence files")
	pprofAddr := flag.String("pprof", "", "start pprof HTTP server on this addr, e.g. :6060")
	headless := flag.Bool("headless", false, "client: disable TUI, write to stdout (for testing/scripting)")
	name := flag.String("name", "", "client: username; skips interactive prompt when set")
	guiPort := flag.String("gui-port", "8080", "client: port for the browser chat UI")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))

	if *pprofAddr != "" {
		go func() {
			slog.Info("pprof listening", "addr", *pprofAddr)
			if err := http.ListenAndServe(*pprofAddr, nil); err != nil {
				slog.Error("pprof failed", "err", err)
			}
		}()
	}

	cfg = Config{
		Addr:      *addr,
		DataDir:   *dataDir,
		PProfAddr: *pprofAddr,
		Headless:  *headless,
		Name:      *name,
		GUIPort:   *guiPort,
	}

	if *mode == "server" {
		serverMain()
	} else {
		clientMain()
	}
}
