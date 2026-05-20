package main

import (
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Client struct {
	isOnline bool
	Name     string
}

// UserData we reserve ID 0 for ourselves which holds to value of our id as a string
// then we know at the start what our ID is, ID 0 also means it is the server communicating to us what happens
type UserData struct {
	ClientUsers map[uint32]Client `json:"users"`
}

var userDataBase UserData

// Recieves message from server, decodes it, and pushes it to the channel
func receiveMessage() {

	decoder := gob.NewDecoder(clientConn)

	for {
		// recieve a message
		var msg Message

		err := decoder.Decode(&msg)

		if err != nil {
			println(err.Error())
			return
		}

		receiverChannel <- msg
	}
}

func dealWithServerMessages(msg Message) {

}

func getUserName(id uint32) {

	var numberAsBytes []byte

	binary.BigEndian.PutUint32(numberAsBytes, id)

	newMessage := InternalMessageData{
		CONTROL_MESSAGE,
		numberAsBytes,
	}

	senderChannel <- newMessage

}

// pull messages off of the channel and prints them out
// TODO run a benchmark and see if we should parrallelize this (array of channels)
func receivedMessageManager() {

	for {
		msg, ok := <-receiverChannel

		// if there is a message
		if ok {
			senderId := msg.SenderId

			// if the system sent us back a message then do stuff
			if msg.Payload.MessageType == CONTROL_MESSAGE {
				dealWithServerMessages(msg)
				continue
			}

			client, isInitialized := userDataBase.ClientUsers[senderId]

			// initialize a user, there should be no other message
			if !isInitialized {
				//TODO should this block? I think it must, I could table it to another thread called the name waiter thread but that seems od
				getUserName(senderId)
			}

			//if it is a first log on, display the message and update the entry
			if client.isOnline == false {
				client.isOnline = true

				app.QueueUpdateDraw(func() {
					fmt.Fprintf(msgView, "[yellow]%s joined[-]\n", client.Name)
				})

				//
				userDataBase.ClientUsers[senderId] = client
			}

			app.QueueUpdateDraw(func() {
				fmt.Fprintf(msgView, "[green]%s[-]: %s\n", client.Name, msg.Payload.Contents)
			})

		}

	}
}

// senders a message to the server
func sendMessage() {

	// initialize

	encoder := gob.NewEncoder(clientConn)

	//the first message should be our name to the database
	senderChannel <- InternalMessageData{CONTROL_MESSAGE, []byte((userDataBase.ClientUsers[myID].Name))}

	for {
		messageData, ok := <-senderChannel

		// if the user sends a message
		if ok {

		}
		message := Message{
			SenderId: myID,
			Payload:  messageData,
		}

		// convert string pointer in message to real data
		err := encoder.Encode(message)

		if err != nil {
			panic(err)
		}
	}
}

// global variables

var myID uint32
var senderChannel chan InternalMessageData
var receiverChannel chan Message
var clientConn net.Conn

func initJSON() {

	_, err := os.Stat("data.json")

	//instantiate local database
	userDataBase.ClientUsers = make(map[uint32]Client)

	// If the file exists
	if err == nil {

		bytes, err := os.ReadFile("data.json")

		err = json.Unmarshal(bytes, &userDataBase)

		if err != nil {
			log.Fatal(err)
		}

		// read in the user ID from index 0
		tempID, err := strconv.Atoi(userDataBase.ClientUsers[0].Name)
		myID = uint32(tempID)

		//we have finished reading our id in and we have everything locally, we are good

		// IF the file does not exist
	} else if errors.Is(err, os.ErrNotExist) {
		//file Exists we do not need to create new json file
		file, err := os.Create("data.json")
		if err != nil {
			log.Fatal(err)
		}

		myID = rand.Uint32()

		//right my id to index 0, for safe keeping when we close and save.
		userDataBase.ClientUsers[0] = Client{
			false,
			strconv.Itoa(int(myID)),
		}

		var name string

		println("What is your username")

		_, err = fmt.Scanln(&name)

		if err != nil {
			panic(err)
		}

		userDataBase.ClientUsers[myID] = Client{
			true,
			name,
		}

		//TODO find a good way to periodically save data to a file
		err = file.Close()
		if err != nil {
			panic(err)
		}
	} else {
		fmt.Println("Other error:", err)
	}
}

func initClient() {

	var err error
	var waitGroup sync.WaitGroup

	//Here we are reading in the user hashmap from disk while connecting to servers concurrently
	waitGroup.Go(initJSON)

	senderChannel = make(chan InternalMessageData, 16)
	receiverChannel = make(chan Message, 16)

	for {
		clientConn, err = net.Dial("tcp", ":1000")

		if err == nil {
			break
		}
	}

	waitGroup.Wait()

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
		senderChannel <- InternalMessageData{
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

	go receiveMessage()
	go sendMessage()
	go receivedMessageManager()

	if err := app.SetRoot(layout, true).Run(); err != nil {
		panic(err)
	}
}

var app *tview.Application
var msgView *tview.TextView
var layout *tview.Flex
