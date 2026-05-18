package main

import (
	"bytes"
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
	SenderId uint32
	Contents string
}

// global hashmap for easy lookup of ids
var serverUsers = map[uint32]ClientInfo{}

// channel for all serverUsers to push messages
var globalChannel chan Message

// This is a go routine that reads messages in and pushes them up to a big channel
func perClientReader(userId uint32) {

	info := serverUsers[userId]
	decoder := gob.NewDecoder(info.conn)

	// repeat until conn is closed
	//TODO handle if CONN is closed
	for {

		// wait for a message on the other side of the network
		var msg Message

		err := decoder.Decode(&msg)

		if err != nil {
			println(err.Error())
			return
		}

		// allocate a space where the message can live to prevent over writing
		//TODO this logic can be simplified
		contents := make([]byte, len(msg.Contents))
		copy(contents, msg.Contents)

		message := Message{
			SenderId: userId,
			Contents: string(contents),
		}

		// push the message into the channel to be read
		globalChannel <- message
	}
}

// This is a per client function that
func perClientWriter(userId uint32) {

	// initialize
	var buffer bytes.Buffer
	info := serverUsers[userId]
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
	var msg Message
	decoder := gob.NewDecoder(conn)

	//read in the starter to data the client sends
	err := decoder.Decode(&msg)

	if err != nil {
		panic(err)
	}

	clientInfo.uniqueId = msg.SenderId

	_, ok := serverUsers[clientInfo.uniqueId]
	println("I am here")

	// if we exist in the hashmap (have been online before
	if ok {

		// if  we are a new user,
	} else {

		// initialize user name and channel, then add to hashmap
		clientInfo.conn = conn
		clientInfo.userName = msg.Contents
		clientInfo.channel = make(chan Message, 16)
		serverUsers[clientInfo.uniqueId] = clientInfo

		// send a message to everyone of our username and id, since this message will inevitably be sent back to us
		// it also serves as an acknowledgment
		initialMessage := Message{
			SenderId: clientInfo.uniqueId,
			Contents: clientInfo.userName,
		}
		globalChannel <- initialMessage

	}

	// now that the thread is initialized, we can launch the two new goroutines and exit
	go perClientWriter(clientInfo.uniqueId)
	go perClientReader(clientInfo.uniqueId)
}

// Constantly listens for new users, and initializes them

func newUserListener(ln net.Listener) {

	// FUTURE projects (rate limiter)
	for {
		conn, err := ln.Accept()

		if err != nil {
			// assume an error mean the server is over
			println(err)
		}
		println("accepted connection")

		go handleConn(conn)

	}
}

// Sends a message to everyone whether they are online or not
func sendMessageToEveryOne(message Message) {

	// iterate through every channel and push the message for their own go routines to handle
	// we do not care about resending the message to sender
	// it actually works in our favor as a form of acknowledgment
	for _, value := range serverUsers {
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
	serverUsers = make(map[uint32]ClientInfo)
	globalChannel = make(chan Message, 128)

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
