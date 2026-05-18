package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"math/rand"
	"net"
)

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

func messageManager() {
	for {
		msg := <-receiverChannel

		senderId := msg.senderId

		userName, ok := clientUsers[senderId]

		// initialize a user
		if !ok {
			name := msg.contents
			clientUsers[senderId] = name
			return
		}

		print(userName)
		println(":  " + msg.contents)

		//TODO make a gui to interface with that prints out messages
		//if sender Id = my sender Id think of it as an acknowledgment and update status
	}
}

func sendMessage() {

	// initialize
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)

	for {
		messageContents, ok := <-senderChannel

		if ok {
			message := Message{
				senderId: myID,
				contents: messageContents,
			}

			// convert pointers to real data
			err := encoder.Encode(message)

			if err != nil {
				panic(err)
			}

			// send it over the net
			_, err = clientConn.Write(buffer.Bytes())

			if err != nil {
				panic(err)
			}

			// reset buffer
			buffer.Reset()
		}

	}
}

var clientUsers map[uint32]string
var myID uint32
var senderChannel chan string
var receiverChannel chan Message

var clientConn net.Conn

func initClient() {

	clientUsers = make(map[uint32]string)
	myID = rand.Uint32()

	senderChannel = make(chan string)
	receiverChannel = make(chan Message)

	var name string

	println("What is your username")

	_, err := fmt.Scanln(&name)

	if err != nil {
		panic(err)
	}

	clientUsers[myID] = name

	clientConn, err = net.Dial("tcp", ":1000")

	for {
		if err != nil {
			clientConn, err = net.Dial("tcp", ":1000")
		} else {
			break
		}
	}

	return
}

func getUserInput() {
	for {
		var input string
		println("What do you want to say")
		_, err := fmt.Scanln(&input)
		if err != nil {
			panic(err)
		}

		//TODO make a special code that quits if neccesary

		senderChannel <- input

	}
}

func clientMain() {

	// why is this not compiling
	initClient()
	go receiveMessage()
	go sendMessage()
	getUserInput()

}
