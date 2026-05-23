package main

import (
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"
)

type ClientInfo struct {
	conn          net.Conn
	userID        uint32
	channel       chan Message
	isOnline      bool
	recentChanges *atomic.Bool
	UserName      string   `json:"name"`
	UserDataBase  UserData `json:"-"`
}
type ServerData struct {
	ServerUsers map[uint32]ClientInfo `json:"users"`
}

// global hashmap for easy lookup of ids
var serverUsers = ServerData{}

var serverRecentChanges atomic.Bool

// channel for all serverUsers to push messages
var globalChannel chan Message

// This is a go routine that reads messages in and pushes them up to a big channel to be sent to everyone else
func perClientReceiver(userId uint32, decoder *gob.Decoder) {

	for {
		// wait for a message on the other side of the network
		var msg Message
		err := decoder.Decode(&msg)

		if err != nil {
			return
		}

		//		fmt.Printf("Server recieved messages: %+v \n", msg)

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
func perClientSender(userId uint32, encoder *gob.Encoder) {

	info := serverUsers.ServerUsers[userId]

	for {
		// receive a message
		message := <-info.channel

		senderID := message.SenderId

		if senderID == userId {
			continue
		}

		client, isInitialized := serverUsers.ServerUsers[userId].UserDataBase.ClientUsers[senderID]

		if !isInitialized {
			senderName := serverUsers.ServerUsers[senderID].UserName

			serverUsers.ServerUsers[userId].UserDataBase.ClientUsers[senderID] = Client{
				isOnline: true,
				Name:     senderName,
			}
			info.recentChanges.Store(true)

			serverMessage := Message{
				SenderId: senderID,
				Payload: InternalMessageData{
					OPcode:   NEW_USERNAME,
					Contents: []byte(senderName),
				},
			}
			fmt.Printf("%+v \n", serverMessage)
			err := encoder.Encode(serverMessage)

			if err != nil {
				panic(err)
			}
			continue

		} else if !client.isOnline {

			serverMessage := Message{
				SenderId: senderID,
				Payload: InternalMessageData{
					OPcode:   NEW_USER_ONLINE,
					Contents: nil,
				},
			}
			fmt.Printf("Serverside message: %+v \n", serverMessage)
			err := encoder.Encode(serverMessage)

			if err != nil {
				panic(err)
			}
			continue
		}

		err := encoder.Encode(message)

		if err != nil {
			panic(err)
		}

		// reset buffer

	}
}

// Every login, read in the json to data
func initializeServerSideClientJson(id uint32, database *UserData) {
	fileName := filepath.Join(cfg.DataDir, "friendsList"+strconv.Itoa(int(id))+".json")

	_, err := os.Stat(fileName)

	// If the file exists
	if err == nil {
		bytes, err := os.ReadFile(fileName)
		if err != nil {
			log.Fatal(err)
		}
		if err = json.Unmarshal(bytes, database); err != nil {
			log.Fatal(err)
		}

		// IF the file does not exist
	} else if errors.Is(err, os.ErrNotExist) {
		initial, err := json.MarshalIndent(database, "", "  ")
		if err != nil {
			log.Fatal(err)
		}
		if err = os.WriteFile(fileName, initial, 0644); err != nil {
			log.Fatal(err)
		}
	} else {
		fmt.Println("Other error:", err)
	}
}

// handles a new connection to the server
func handleConn(conn net.Conn) {

	var clientInfo ClientInfo
	var msg Message
	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	//read in the starter to data the client sends
	err := decoder.Decode(&msg)

	if err != nil {
		return
	}

	fmt.Printf("firstMessage: %+v\n", msg)

	clientInfo.userID = msg.SenderId

	_, ok := serverUsers.ServerUsers[clientInfo.userID]

	//initialize all the temporary client objects
	clientInfo.conn = conn
	clientInfo.recentChanges = new(atomic.Bool)
	clientInfo.channel = make(chan Message, 16)
	clientInfo.isOnline = true
	clientInfo.UserDataBase.ClientUsers = make(map[uint32]Client)
	clientInfo.UserName = string(msg.Payload.Contents)
	initializeServerSideClientJson(clientInfo.userID, &clientInfo.UserDataBase)

	// send back the current list of users
	// this line has to be in this exact location, because we need the channel to exist, but we cannot be in the hashmap yet, so there is no race condtion
	// this message will always be the first one recieved back
	//they will not know we are online because we are not identified as online, and we will not know if they are online

	for key, value := range serverUsers.ServerUsers {
		if value.isOnline {

			var msg Message
			_, knownUser := clientInfo.UserDataBase.ClientUsers[key]

			if knownUser {
				msg = Message{
					SenderId: key,
					Payload: InternalMessageData{
						OPcode:   EXISTING_USER,
						Contents: nil,
					},
				}

				newClient := Client{
					isOnline: true,
					Name:     value.UserName,
				}

				clientInfo.UserDataBase.ClientUsers[key] = newClient

			} else {
				name := value.UserName

				newClient := Client{
					isOnline: true,
					Name:     name,
				}

				clientInfo.UserDataBase.ClientUsers[key] = newClient

				msg = Message{
					SenderId: key,
					Payload: InternalMessageData{
						OPcode:   NEW_USER_WHO_WAS_ONLINE,
						Contents: []byte(name),
					},
				}
			}
			err := encoder.Encode(msg)
			if err != nil {
				panic(err)
			}
		}
	}

	// This line has to be here because if this line was after us and we were a new user, nobody would know our name
	serverUsers.ServerUsers[clientInfo.userID] = clientInfo

	// send a message to everyone of our id, they will internally check if the know us
	initialMessage := Message{
		SenderId: clientInfo.userID,
		Payload: InternalMessageData{
			OPcode:   NEW_USER_ONLINE,
			Contents: nil,
		},
	}
	globalChannel <- initialMessage

	// send a message back of all the current users ids->name

	if !ok {
		serverRecentChanges.Store(true)
	}

	// now that the thread is initialized, we can launch the two new goroutines and exit
	go perClientSender(clientInfo.userID, encoder)
	go perClientReceiver(clientInfo.userID, decoder)
	go writeToPerClientJson(clientInfo.userID)
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

func writeToPerClientJson(userId uint32) {
	for {
		time.Sleep(4 * time.Second)

		recentChanges := serverUsers.ServerUsers[userId].recentChanges.Swap(false)

		if recentChanges {
			updated, err := json.MarshalIndent(serverUsers.ServerUsers[userId].UserDataBase, "", "  ")
			if err != nil {
				panic(err)
			}

			fileName := filepath.Join(cfg.DataDir, "friendsList"+strconv.Itoa(int(userId))+".json")
			err = os.WriteFile(fileName, updated, 0644)
			if err != nil {
				panic(err)
			}
		}
	}
}

func writeToServerJson() {
	for {
		time.Sleep(4 * time.Second)

		recentChanges := serverRecentChanges.Swap(false)

		if recentChanges {
			updated, err := json.MarshalIndent(serverUsers, "", "  ")
			if err != nil {
				panic(err)
			}

			err = os.WriteFile(filepath.Join(cfg.DataDir, "data.json"), updated, 0644)
			if err != nil {
				panic(err)
			}
		}
	}
}

func initServerJSON() {
	path := filepath.Join(cfg.DataDir, "data.json")
	_, err := os.Stat(path)

	//instantiate local database
	serverUsers.ServerUsers = make(map[uint32]ClientInfo)

	// If the file exists
	if err == nil {
		bytes, err := os.ReadFile(path)
		if err != nil {
			log.Fatal(err)
		}
		if err = json.Unmarshal(bytes, &serverUsers); err != nil {
			log.Fatal(err)
		}

		// IF the file does not exist
	} else if errors.Is(err, os.ErrNotExist) {
		initial, err := json.MarshalIndent(serverUsers, "", "  ")
		if err != nil {
			log.Fatal(err)
		}
		if err = os.WriteFile(path, initial, 0644); err != nil {
			log.Fatal(err)
		}
	} else {
		fmt.Println("Other error:", err)
	}
}

// This function is the main orchestrator of the server
func serverMain() {

	globalChannel = make(chan Message, 128)

	initServerJSON()

	go writeToServerJson()

	ln, err := net.Listen("tcp", port)

	if err != nil {
		panic(err)
	}

	// initialize all new users
	go newUserListener(ln)

	for {
		// pull messages off the channel, then send it to everyone in existence
		message, ok := <-globalChannel

		if ok {
			fmt.Printf("Global Channel: %+v \n", message)
			sendMessageToEveryOne(message)
		}
	}

}
