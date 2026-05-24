package client

import (
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"TCPchat/internal/shared"
)

type ClientGlobalData struct {
	userDataBase    shared.UserData
	myID            uint32
	senderChannel   chan shared.InternalMessageData
	receiverChannel chan shared.Message
	clientConn      net.Conn
	recentChanges   atomic.Bool
}

var clientState ClientGlobalData

func receiveMessage() {
	decoder := gob.NewDecoder(clientState.clientConn)
	for {
		var msg shared.Message
		err := decoder.Decode(&msg)
		if err != nil {
			println(err.Error())
			return
		}
		clientState.receiverChannel <- msg
	}
}

func receivedMessageManager() {
	for {
		msg, ok := <-clientState.receiverChannel
		if ok {
			fmt.Printf("Received message: %+v \n", msg)
			senderId := msg.SenderId
			opCode := msg.Payload.OPcode
			client, isInitialized := clientState.userDataBase.ClientUsers[senderId]

			switch opCode {
			case shared.DATA_MESSAGE:
				if !isInitialized {
					panic("uninitialized client")
				}
				if !client.IsOnline {
					panic("client not online")
				}
				broadcastToGUI(fmt.Sprintf(`<span class="name">%s</span>: %s`,
					html.EscapeString(client.Name),
					html.EscapeString(string(msg.Payload.Contents))))

			case shared.NEW_USER_ONLINE:
				if !isInitialized {
					panic("uninitialized client")
				}
				client.IsOnline = true
				broadcastToGUI(fmt.Sprintf(`<span class="sys">%s joined</span>`,
					html.EscapeString(client.Name)))
				clientState.userDataBase.ClientUsers[senderId] = client

			case shared.NEW_USERNAME:
				userName := string(msg.Payload.Contents)
				newClient := shared.Client{IsOnline: true, Name: userName}
				clientState.userDataBase.ClientUsers[senderId] = newClient
				clientState.recentChanges.Store(true)
				broadcastToGUI(fmt.Sprintf(`<span class="sys">%s is a new user who joined</span>`,
					html.EscapeString(userName)))

			case shared.EXISTING_USER:
				newClient := clientState.userDataBase.ClientUsers[senderId]
				newClient.IsOnline = true
				clientState.userDataBase.ClientUsers[senderId] = newClient
				broadcastToGUI(fmt.Sprintf(`<span class="sys">%s is online </span>`,
					html.EscapeString(newClient.Name)))

			case shared.NEW_USER_WHO_WAS_ONLINE:
				userName := string(msg.Payload.Contents)
				newClient := shared.Client{IsOnline: true, Name: userName}
				clientState.userDataBase.ClientUsers[senderId] = newClient
				clientState.recentChanges.Store(true)
				broadcastToGUI(fmt.Sprintf(`<span class="sys">%s is a new user who was online before you</span>`,
					html.EscapeString(userName)))

			default:
				panic("unknown opcode")
			}
		}
	}
}

func sendMessage() {
	encoder := gob.NewEncoder(clientState.clientConn)
	clientState.senderChannel <- shared.InternalMessageData{
		OPcode:   shared.DATA_MESSAGE,
		Contents: []byte(clientState.userDataBase.ClientUsers[clientState.myID].Name),
	}
	for {
		messageData, ok := <-clientState.senderChannel
		if ok {
		}
		message := shared.Message{
			SenderId: clientState.myID,
			Payload:  messageData,
		}
		err := encoder.Encode(message)
		if err != nil {
			panic(err)
		}
	}
}

func writeToClientSideJson() {
	for {
		time.Sleep(4 * time.Second)
		recentChanges := clientState.recentChanges.Swap(false)
		if recentChanges {
			updated, err := json.MarshalIndent(clientState.userDataBase, "", "  ")
			if err != nil {
				panic(err)
			}
			err = os.WriteFile(filepath.Join(shared.Cfg.DataDir, "data.json"), updated, 0644)
			if err != nil {
				panic(err)
			}
		}
	}
}

func initJSON() {
	path := filepath.Join(shared.Cfg.DataDir, "data.json")
	_, err := os.Stat(path)
	clientState.userDataBase.ClientUsers = make(map[uint32]shared.Client)

	if err == nil {
		bytes, err := os.ReadFile(path)
		if err != nil {
			panic(err)
		}
		if err = json.Unmarshal(bytes, &clientState.userDataBase); err != nil {
			log.Fatal(err)
		}
		tempID, err := strconv.Atoi(clientState.userDataBase.ClientUsers[0].Name)
		clientState.myID = uint32(tempID)
		_ = err

	} else if errors.Is(err, os.ErrNotExist) {
		file, err := os.Create(path)
		if err != nil {
			log.Fatal(err)
		}
		clientState.myID = rand.Uint32()
		clientState.userDataBase.ClientUsers[0] = shared.Client{
			IsOnline: false,
			Name:     strconv.Itoa(int(clientState.myID)),
		}

		var name string
		println("What is your username")
		_, err = fmt.Scanln(&name)
		if err != nil {
			panic(err)
		}
		clientState.userDataBase.ClientUsers[clientState.myID] = shared.Client{
			IsOnline: true,
			Name:     name,
		}
		initial, err := json.MarshalIndent(clientState.userDataBase, "", "  ")
		if err != nil {
			panic(err)
		}
		if err = os.WriteFile(path, initial, 0644); err != nil {
			panic(err)
		}
		file.Close()
	} else {
		fmt.Println("Other error:", err)
	}
}

func initClient() {
	var err error
	initJSON()
	clientState.senderChannel = make(chan shared.InternalMessageData, 16)
	clientState.receiverChannel = make(chan shared.Message, 16)
	for {
		clientState.clientConn, err = net.Dial("tcp", shared.Port)
		if err == nil {
			break
		}
	}
	initGUI()
}

func initGUI() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, chatHTML)
	})

	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ch := make(chan string, 32)
		guiClientsMu.Lock()
		guiClients[ch] = struct{}{}
		guiClientsMu.Unlock()
		defer func() {
			guiClientsMu.Lock()
			delete(guiClients, ch)
			guiClientsMu.Unlock()
		}()

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		guiHistoryMu.Lock()
		snapshot := make([]string, len(guiHistory))
		copy(snapshot, guiHistory)
		guiHistoryMu.Unlock()
		for _, msg := range snapshot {
			fmt.Fprintf(w, "data: %s\n\n", msg)
		}
		flusher.Flush()

		for {
			select {
			case msg := <-ch:
				fmt.Fprintf(w, "data: %s\n\n", msg)
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})

	mux.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if text := r.FormValue("m"); text != "" {
			myName := clientState.userDataBase.ClientUsers[clientState.myID].Name
			clientState.senderChannel <- shared.InternalMessageData{OPcode: shared.DATA_MESSAGE, Contents: []byte(text)}
			broadcastToGUI(fmt.Sprintf(`<span class="name">%s</span>: %s`,
				html.EscapeString(myName),
				html.EscapeString(text)))
		}
		w.WriteHeader(http.StatusNoContent)
	})

	addr := "localhost:" + shared.Cfg.GUIPort
	log.Printf("Chat UI → http://%s", addr)
	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Fatal(err)
		}
	}()
}

func Main() {
	initClient()
	go receiveMessage()
	go sendMessage()
	go receivedMessageManager()
	go writeToClientSideJson()
	select {}
}

var (
	guiClients   = make(map[chan string]struct{})
	guiClientsMu sync.Mutex

	guiHistory   []string
	guiHistoryMu sync.Mutex
)

const guiHistoryMax = 200

func broadcastToGUI(htmlSnippet string) {
	guiHistoryMu.Lock()
	guiHistory = append(guiHistory, htmlSnippet)
	if len(guiHistory) > guiHistoryMax {
		guiHistory = guiHistory[len(guiHistory)-guiHistoryMax:]
	}
	guiHistoryMu.Unlock()

	guiClientsMu.Lock()
	defer guiClientsMu.Unlock()
	for ch := range guiClients {
		select {
		case ch <- htmlSnippet:
		default:
		}
	}
}

const chatHTML = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>TCPChat</title>
<style>
  body{font-family:monospace;margin:0;padding:10px;background:#1e1e1e;color:#d4d4d4;display:flex;flex-direction:column;height:100vh;box-sizing:border-box}
  #msgs{flex:1;overflow-y:auto;border:1px solid #444;padding:8px;margin-bottom:8px}
  #row{display:flex;gap:6px}
  #inp{flex:1;background:#2d2d2d;color:#d4d4d4;border:1px solid #555;padding:6px;font-family:monospace;font-size:14px}
  button{background:#0e639c;color:#fff;border:none;padding:6px 14px;cursor:pointer;font-size:14px}
  .sys{color:#dcdcaa}
  .name{color:#4ec9b0;font-weight:bold}
</style>
</head>
<body>
<div id="msgs"></div>
<div id="row">
  <input id="inp" type="text" placeholder="Type a message…" autofocus>
  <button onclick="send()">Send</button>
</div>
<script>
const msgs=document.getElementById('msgs');
const inp=document.getElementById('inp');
new EventSource('/events').onmessage=e=>{
  const d=document.createElement('div');
  d.innerHTML=e.data;
  msgs.appendChild(d);
  msgs.scrollTop=msgs.scrollHeight;
};
function send(){
  const t=inp.value.trim();
  if(!t)return;
  fetch('/send',{method:'POST',body:new URLSearchParams({m:t})});
  inp.value='';
}
inp.onkeydown=e=>{if(e.key==='Enter')send();};
</script>
</body>
</html>`
