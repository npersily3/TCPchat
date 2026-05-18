package main

import (
	"encoding/gob"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

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

// pull messages off of the channel and prints them out
func messageManager() {
	for {
		msg, ok := <-receiverChannel

		// if there is a message
		if ok {
			senderId := msg.SenderId

			userName, isInitialized := clientUsers[senderId]

			// initialize a user, there should be no other message
			if !isInitialized {
				name := msg.Contents
				clientUsers[senderId] = name

				//GUI specific thing
				{
					app.QueueUpdateDraw(func() {
						fmt.Fprintf(msgView, "[yellow]%s joined[-]\n", name)
					})
				}
				continue
			}
			app.QueueUpdateDraw(func() {
				fmt.Fprintf(msgView, "[green]%s[-]: %s\n", userName, msg.Contents)
			})

			//print(userName)
			//println(":  " + msg.contents)

			//TODO make a gui to interface with that prints out messages
			//if sender Id = my sender Id think of it as an acknowledgment and update status (sent vs sending)
		}

	}
}

// senders a message to the server
func sendMessage() {

	// initialize

	encoder := gob.NewEncoder(clientConn)

	initialString := strconv.Itoa(int(myID)) + clientUsers[myID]
	senderChannel <- initialString

	for {
		messageContents, ok := <-senderChannel

		// if the user sends a message
		if ok {
			message := Message{
				SenderId: myID,
				Contents: messageContents,
			}

			// convert string pointer in message to real data
			err := encoder.Encode(message)

			if err != nil {
				panic(err)
			}
		}
	}
}

// global variables
var clientUsers map[uint32]string
var myID uint32
var senderChannel chan string
var receiverChannel chan Message
var clientConn net.Conn

func initClient() {

	f, _ := os.OpenFile("debug.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	log.SetOutput(f)

	clientUsers = make(map[uint32]string)
	myID = rand.Uint32()

	senderChannel = make(chan string, 16)
	receiverChannel = make(chan Message, 16)

	var name string

	println("What is your username")

	_, err := fmt.Scanln(&name)

	if err != nil {
		panic(err)
	}

	clientUsers[myID] = name

	for {
		clientConn, err = net.Dial("tcp", ":1000")

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
		senderChannel <- text
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
	go messageManager()

	if err := app.SetRoot(layout, true).Run(); err != nil {
		panic(err)
	}
}

var app *tview.Application
var msgView *tview.TextView
var layout *tview.Flex
