package shared

const Port = ":1000"

type Config struct {
	Addr      string
	DataDir   string
	PProfAddr string
	Headless  bool
	Name      string
	GUIPort   string
}

// Cfg is populated by cmd/tcpchat/main.go before calling server.Main or client.Main.
var Cfg Config
