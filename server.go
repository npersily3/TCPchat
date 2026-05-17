package main

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"net"
)

type ClientInfo struct {
	conn     net.Conn
	uniqueId uint32
	channel  chan Message

	// have this be last because it is of variable size
	userName string
}

// simple message and sender structs
type Message struct {
	senderId uint32
	contents string
}

// global hashmap for easy lookup of ids
var users = map[uint32]ClientInfo{}

// channel for all users to push messages
var globalChannel chan Message

// This is a go routine that reads messages in and pushes them up to a big channel
func perClientReader(userId uint32) {

	info := users[userId]
	messageBuffer := make([]byte, 1024)

	// repeat until conn is closed
	//TODO handle if CONN is closed
	for {

		// wait for a message on the other side of the network
		messageLength, err := info.conn.Read(messageBuffer)

		//TODO you will likely have to use the gob decoder to decode messages into their structs

		if err != nil {
			panic(err)
		}

		// allocate a space where the message can live to prevent over writing
		contents := make([]byte, messageLength)
		copy(contents, messageBuffer[:messageLength])

		message := Message{
			senderId: userId,
			contents: string(contents),
		}

		// push the message into the channel to be read
		globalChannel <- message
	}
}

// This is a per client function that
func perClientWriter(userId uint32) {

	// initialize
	var buffer bytes.Buffer
	info := users[userId]
	encoder := gob.NewEncoder(&buffer)

	for {
		// recieve a message
		message, ok := <-info.channel

		if ok {

			// convert pointers to real data
			err := encoder.Encode(message)

			if err != nil {
				panic(err)
			}

			// send it over the net
			_, err = info.conn.Write(buffer.Bytes())

			if err != nil {
				panic(err)
			}

			// reset buffer
			buffer.Reset()
		}
	}
}

// handles a new connection to the server
func handleConn(conn net.Conn) {

	var clientInfo ClientInfo
	var rawData []byte

	//read in the starter to data the client sends
	n, err := conn.Read(rawData)

	if err != nil {
		panic(err)
	}
	// you only care about the data that it sends
	rawData = rawData[:n]

	clientInfo.conn = conn

	// read the first four bytes which the client agrees have to be the id
	id := binary.BigEndian.Uint32(rawData[:4])
	clientInfo.uniqueId = id

	_, ok := users[clientInfo.uniqueId]

	// if we exist in the hashmap (have been online before
	if ok {

		// if  we are a new user,
	} else {
		//TODO find a way if I can send previous messages

		// initialize user name and channel, then add to hashmap
		clientInfo.userName = string(rawData[4:n])
		clientInfo.channel = make(chan Message)
		users[clientInfo.uniqueId] = clientInfo

		// send a message to everyone of our username and id, since this message will inevitably be sent back to us
		// it also serves as an acknowledgment
		initialMessage := Message{
			senderId: clientInfo.uniqueId,
			contents: clientInfo.userName,
		}
		globalChannel <- initialMessage

	}

	// now that the thread is initialized, we can launch the two new goroutines and exit
	go perClientWriter(id)
	go perClientReader(id)
}

// Constantly listens for new users, and initializes them
func newUserListener(ln net.Listener) {

	//TODO see if this is always spin or something else
	for {
		conn, err := ln.Accept()

		if err != nil {
			panic(err)
		}
		go handleConn(conn)

	}
}

// Sends a message to everyone whether they are online or not
func sendMessageToEveryOne(message Message) {

	// iterate through every channel and push the message for their own go routines to handle
	// we do not care about resending the message to sender
	// it actually works in our favor as a form of acknowledgment
	for _, value := range users {
		value.channel <- message
	}
}

// This function is the main orchestrator of the server
func serverMain() {
	ln, err := net.Listen("tcp", ":1000")

	if err != nil {
		panic(err)
	}
	// initialize global stuff
	users = make(map[uint32]ClientInfo)
	globalChannel = make(chan Message)

	// initialize all new users
	go newUserListener(ln)

	for {
		// pull messages off the channel, then send it to everyone in existence
		message, ok := <-globalChannel

		if ok {
			go sendMessageToEveryOne(message)
		}
	}

}
