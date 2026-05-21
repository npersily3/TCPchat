package main

import (
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
)

type ClientInfo struct {
	conn         net.Conn
	userID       uint32
	channel      chan Message
	isOnline     bool
	userDataBase UserData

	// have this be last because it is of variable size
	userName string
}
type ServerData struct {
	ServerUsers map[uint32]ClientInfo `json:"users"`
}

// global hashmap for easy lookup of ids
var serverUsers = ServerData{}

// channel for all serverUsers to push messages
var globalChannel chan Message

// This is a go routine that reads messages in and pushes them up to a big channel
func perClientReceiver(userId uint32, decoder *gob.Decoder) {

	for {
		// wait for a message on the other side of the network
		var msg Message
		err := decoder.Decode(&msg)

		if err != nil {
			panic(err)
		}

		// allocate a space where the message can live
		//This prevents the msg variable from being overwritten
		contents := make([]byte, len(msg.Payload.Contents))
		copy(contents, msg.Payload.Contents)

		message := Message{
			SenderId: userId,
			Payload: InternalMessageData{
				OPcode:   msg.Payload.OPcode,
				Contents: contents,
			},
		}

		// push the message into the channel to be read
		globalChannel <- message
	}
}

// This is a per client function that
func perClientSender(userId uint32) {

	info := serverUsers[userId]
	encoder := gob.NewEncoder(serverUsers[userId].conn)

	for {
		// receive a message
		message := <-info.channel

		senderID := message.SenderId

		client, isInitialized := serverUsers[userId].userDataBase.ClientUsers[senderID]

		if !isInitialized {
			senderName := serverUsers[senderID].userName

			serverUsers[userId].userDataBase.ClientUsers[senderID] = Client{
				isOnline: true,
				Name:     senderName,
			}

			serverMessage := Message{
				SenderId: senderID,
				Payload: InternalMessageData{
					OPcode:   NEW_USERNAME,
					Contents: []byte(senderName),
				},
			}
			err := encoder.Encode(serverMessage)

			if err != nil {
				panic(err)
			}

		} else if !client.isOnline {

			serverMessage := Message{
				SenderId: senderID,
				Payload: InternalMessageData{
					OPcode:   NEW_USERNAME,
					Contents: nil,
				},
			}
			err := encoder.Encode(serverMessage)

			if err != nil {
				panic(err)
			}
		}

		err := encoder.Encode(message)

		if err != nil {
			panic(err)
		}

		// reset buffer

	}
}

func getCurrentlyOnlineUsers() []byte {

	//TODO need local slice

	for key, value := range serverUsers.ServerUsers {
		if value.isOnline {

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

	fmt.Printf("firstMessage: %+v\n", msg)

	clientInfo.userID = msg.SenderId

	oldClientInfo, ok := serverUsers.ServerUsers[clientInfo.userID]

	// if we exist in the hashmap (have been online before
	if ok {
		oldClientInfo.conn = conn
		// if  we are a new user,
	} else {

		// initialize user name and channel, then add to hashmap
		clientInfo.conn = conn
		clientInfo.userName = string(msg.Payload.Contents)
		clientInfo.channel = make(chan Message, 16)
		clientInfo.isOnline = true
		//TODO initialize the JSON of Usernames

		var otherActiveUsers []byte = getCurrentlyOnlineUsers()
		// send back the current list of users
		// this line has to be in this exact location, because we need the channel to exist, but we cannot be in the hashmap yet, so there is no race condtion
		// this message will always be the first one recieved back
		clientInfo.channel <- Message{
			SenderId: clientInfo.userID,
			Contents: otherActiveUsers,
		}

		serverUsers[clientInfo.userID] = clientInfo

		// send a message to everyone of our username and id, since this message will inevitably be sent back to us
		// it also serves as an acknowledgment
		initialMessage := Message{
			SenderId: clientInfo.userID,
			Contents: clientInfo.userName,
		}
		globalChannel <- initialMessage

		// send a message back of all the current users ids->name

	}

	// now that the thread is initialized, we can launch the two new goroutines and exit
	go perClientSender(clientInfo.userID)
	go perClientReceiver(clientInfo.userID, decoder)
}

// Constantly listens for new users, and initializes them

func newUserListener(ln net.Listener) {

	// FUTURE projects (rate limiter)
	for {
		conn, err := ln.Accept()

		if err != nil {
			// assume an error mean the server is over
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
	for _, value := range serverUsers.ServerUsers {
		value.channel <- message
	}
}

func initServerJSON() {
	_, err := os.Stat("data.json")

	//instantiate local database
	serverUsers.ServerUsers = make(map[uint32]ClientInfo)

	// If the file exists
	if err == nil {

		bytes, err := os.ReadFile("data.json")

		err = json.Unmarshal(bytes, &serverUsers)

		if err != nil {
			log.Fatal(err)
		}

		// IF the file does not exist
	} else if errors.Is(err, os.ErrNotExist) {
		//file Exists we do not need to create new json file
		file, err := os.Create("data.json")
		if err != nil {
			log.Fatal(err)
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

// This function is the main orchestrator of the server
func serverMain() {

	globalChannel = make(chan Message, 128)

	initServerJSON()

	ln, err := net.Listen("tcp", ":1000")

	if err != nil {
		panic(err)
	}

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
