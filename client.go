package main

import (
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type ClientGlobalData struct {
	userDataBase    UserData
	myID            uint32
	senderChannel   chan InternalMessageData
	receiverChannel chan Message
	clientConn      net.Conn
	recentChanges   atomic.Bool
}

var clientState ClientGlobalData

// Recieves message from server, decodes it, and pushes it to the channel
func receiveMessage() {

	decoder := gob.NewDecoder(clientState.clientConn)

	for {
		// recieve a message
		var msg Message

		err := decoder.Decode(&msg)

		if err != nil {
			println(err.Error())
			return
		}

		clientState.receiverChannel <- msg
	}
}

// pull messages off of the channel and prints them out
// TODO run a benchmark and see if we should parrallelize this (array of channels)
func receivedMessageManager() {

	for {
		msg, ok := <-clientState.receiverChannel

		// if there is a message
		if ok {
			senderId := msg.SenderId
			opCode := msg.Payload.OPcode

			client, isInitialized := clientState.userDataBase.ClientUsers[senderId]

			switch opCode {

			case DATA_MESSAGE:

				if !isInitialized {
					panic("uninitialized client")
				}
				if !client.isOnline {
					panic("client not online")
				}

				app.QueueUpdateDraw(func() {
					fmt.Fprintf(msgView, "[green]%s[-]: %s\n", client.Name, msg.Payload.Contents)
				})
			case NEW_USER_ONLINE:

				if !isInitialized {
					panic("uninitialized client")
				}
				if client.isOnline {
					panic("client online")
				}

				client.isOnline = true

				app.QueueUpdateDraw(func() {
					fmt.Fprintf(msgView, "[yellow]%s joined[-]\n", client.Name)
				})

				// do not update the json, because the only field changed only pertains to the current state
				clientState.userDataBase.ClientUsers[senderId] = client

			// this is also new
			case NEW_USERNAME:
				userName := string(msg.Payload.Contents)
				newClient := Client{isOnline: true, Name: userName}
				clientState.userDataBase.ClientUsers[senderId] = newClient
				clientState.recentChanges.Store(true)

				app.QueueUpdateDraw(func() {
					fmt.Fprintf(msgView, "[yellow]%s is a new user who joined[-]\n", userName)
				})

			default:
				panic("unknown opcode")

			}

		}

	}
}

// senders a message to the server
func sendMessage() {

	encoder := gob.NewEncoder(clientState.clientConn)

	clientState.senderChannel <- InternalMessageData{
		OPcode:   DATA_MESSAGE,
		Contents: []byte(clientState.userDataBase.ClientUsers[clientState.myID].Name),
	}

	for {
		messageData, ok := <-clientState.senderChannel

		// if the user sends a message
		if ok {

		}
		message := Message{
			SenderId: clientState.myID,
			Payload:  messageData,
		}

		// convert string pointer in message to real data
		err := encoder.Encode(message)

		if err != nil {
			panic(err)
		}
	}
}

// periodically write to json
func writeToClientSideJson() {
	for {
		time.Sleep(4 * time.Second)

		recentChanges := clientState.recentChanges.Swap(false)

		if recentChanges {
			// Marshal back to JSON
			updated, err := json.MarshalIndent(clientState.userDataBase, "", "  ")
			if err != nil {
				panic(err)
			}

			// Write back to file
			//the 0644 is an octal code to specify permissions
			err = os.WriteFile(filepath.Join(cfg.DataDir, "data.json"), updated, 0644)

			if err != nil {
				panic(err)
			}
		}

	}
}

func initJSON() {
	path := filepath.Join(cfg.DataDir, "data.json")

	_, err := os.Stat(path)

	//instantiate local database
	clientState.userDataBase.ClientUsers = make(map[uint32]Client)

	// If the file exists
	if err == nil {
		bytes, err := os.ReadFile(path)
		if err != nil {
			panic(err)
		}
		if err = json.Unmarshal(bytes, &clientState.userDataBase); err != nil {
			log.Fatal(err)
		}

		// read in the user ID from index 0
		tempID, err := strconv.Atoi(clientState.userDataBase.ClientUsers[0].Name)
		clientState.myID = uint32(tempID)

		//we have finished reading our id in and we have everything locally, we are good

		// IF the file does not exist
	} else if errors.Is(err, os.ErrNotExist) {
		file, err := os.Create(path)
		if err != nil {
			log.Fatal(err)
		}

		clientState.myID = rand.Uint32()

		//right my id to index 0, for safe keeping when we close and save.
		clientState.userDataBase.ClientUsers[0] = Client{
			false,
			strconv.Itoa(int(clientState.myID)),
		}

		var name string

		println("What is your username")

		_, err = fmt.Scanln(&name)

		if err != nil {
			panic(err)
		}

		clientState.userDataBase.ClientUsers[clientState.myID] = Client{
			true,
			name,
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

	//Here we are reading in the user hashmap from disk while connecting to servers concurrently
	initJSON()

	clientState.senderChannel = make(chan InternalMessageData, 16)
	clientState.receiverChannel = make(chan Message, 16)

	for {
		clientState.clientConn, err = net.Dial("tcp", port)

		if err == nil {
			break
		}
	}

	initGUI()
}

func initGUI() {
	app = tview.NewApplication()

	msgView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetChangedFunc(func() { app.Draw() })
	msgView.SetBorder(true).SetTitle(" Messages ")

	inputField := tview.NewInputField().
		SetLabel("> ").
		SetFieldBackgroundColor(tcell.ColorDefault)
	inputField.SetDoneFunc(func(key tcell.Key) {
		if key != tcell.KeyEnter {
			return
		}
		text := inputField.GetText()
		if text == "" {
			return
		}
		clientState.senderChannel <- InternalMessageData{
			DATA_MESSAGE,
			[]byte(text),
		}
		inputField.SetText("")
	})

	layout = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(msgView, 0, 1, false).
		AddItem(inputField, 3, 0, true)
}

func clientMain() {

	initClient()

	// spawn all the relevant threads
	go receiveMessage()
	go sendMessage()
	go receivedMessageManager()
	go writeToClientSideJson()

	if err := app.SetRoot(layout, true).Run(); err != nil {
		panic(err)
	}
}

var app *tview.Application
var msgView *tview.TextView
var layout *tview.Flex
