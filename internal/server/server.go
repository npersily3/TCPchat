package server

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

	"TCPchat/internal/shared"
)

type ClientInfo struct {
	conn          net.Conn
	userID        uint32
	channel       chan shared.Message
	isOnline      bool
	recentChanges *atomic.Bool
	UserName      string          `json:"name"`
	UserDataBase  shared.UserData `json:"-"`
}

type ServerData struct {
	ServerUsers map[uint32]ClientInfo `json:"users"`
}

var serverUsers = ServerData{}
var serverRecentChanges atomic.Bool
var globalChannel chan shared.Message

func perClientReceiver(userId uint32, decoder *gob.Decoder) {
	for {
		var msg shared.Message
		err := decoder.Decode(&msg)
		if err != nil {
			return
		}

		contents := make([]byte, len(msg.Payload.Contents))
		copy(contents, msg.Payload.Contents)

		message := shared.Message{
			SenderId: userId,
			Payload: shared.InternalMessageData{
				OPcode:   msg.Payload.OPcode,
				Contents: contents,
			},
		}
		globalChannel <- message
	}
}

func perClientSender(userId uint32, encoder *gob.Encoder) {
	info := serverUsers.ServerUsers[userId]

	for {
		message := <-info.channel
		senderID := message.SenderId

		if senderID == userId {
			continue
		}

		client, isInitialized := serverUsers.ServerUsers[userId].UserDataBase.ClientUsers[senderID]

		if !isInitialized {
			senderName := serverUsers.ServerUsers[senderID].UserName
			serverUsers.ServerUsers[userId].UserDataBase.ClientUsers[senderID] = shared.Client{
				IsOnline: true,
				Name:     senderName,
			}
			info.recentChanges.Store(true)

			serverMessage := shared.Message{
				SenderId: senderID,
				Payload: shared.InternalMessageData{
					OPcode:   shared.NEW_USERNAME,
					Contents: []byte(senderName),
				},
			}
			err := encoder.Encode(serverMessage)
			if err != nil {
				panic(err)
			}
			continue

		} else if !client.IsOnline {
			serverMessage := shared.Message{
				SenderId: senderID,
				Payload: shared.InternalMessageData{
					OPcode:   shared.NEW_USER_ONLINE,
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
	}
}

func initializeServerSideClientJson(id uint32, database *shared.UserData) {
	fileName := filepath.Join(shared.Cfg.DataDir, "friendsList"+strconv.Itoa(int(id))+".json")
	_, err := os.Stat(fileName)

	if err == nil {
		bytes, err := os.ReadFile(fileName)
		if err != nil {
			log.Fatal(err)
		}
		if err = json.Unmarshal(bytes, database); err != nil {
			log.Fatal(err)
		}
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

func handleConn(conn net.Conn) {
	var clientInfo ClientInfo
	var msg shared.Message
	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	err := decoder.Decode(&msg)
	if err != nil {
		return
	}

	clientInfo.userID = msg.SenderId
	_, ok := serverUsers.ServerUsers[clientInfo.userID]

	clientInfo.conn = conn
	clientInfo.recentChanges = new(atomic.Bool)
	clientInfo.channel = make(chan shared.Message, 16)
	clientInfo.isOnline = true
	clientInfo.UserDataBase.ClientUsers = make(map[uint32]shared.Client)
	clientInfo.UserName = string(msg.Payload.Contents)
	initializeServerSideClientJson(clientInfo.userID, &clientInfo.UserDataBase)

	// Send the current online user list before registering ourselves, so there
	// is no race between our channel existing and us appearing in the map.
	for key, value := range serverUsers.ServerUsers {
		if value.isOnline {
			var m shared.Message
			_, knownUser := clientInfo.UserDataBase.ClientUsers[key]

			if knownUser {
				m = shared.Message{
					SenderId: key,
					Payload: shared.InternalMessageData{
						OPcode:   shared.EXISTING_USER,
						Contents: nil,
					},
				}
				clientInfo.UserDataBase.ClientUsers[key] = shared.Client{
					IsOnline: true,
					Name:     value.UserName,
				}
			} else {
				name := value.UserName
				clientInfo.UserDataBase.ClientUsers[key] = shared.Client{
					IsOnline: true,
					Name:     name,
				}
				m = shared.Message{
					SenderId: key,
					Payload: shared.InternalMessageData{
						OPcode:   shared.NEW_USER_WHO_WAS_ONLINE,
						Contents: []byte(name),
					},
				}
			}
			err := encoder.Encode(m)
			if err != nil {
				panic(err)
			}
		}
	}

	serverUsers.ServerUsers[clientInfo.userID] = clientInfo

	initialMessage := shared.Message{
		SenderId: clientInfo.userID,
		Payload: shared.InternalMessageData{
			OPcode:   shared.NEW_USER_ONLINE,
			Contents: nil,
		},
	}
	globalChannel <- initialMessage

	if !ok {
		serverRecentChanges.Store(true)
	}

	go perClientSender(clientInfo.userID, encoder)
	go perClientReceiver(clientInfo.userID, decoder)
	go writeToPerClientJson(clientInfo.userID)
}

func newUserListener(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			panic(err)
		}
		go handleConn(conn)
	}
}

func sendMessageToEveryOne(message shared.Message) {
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
			fileName := filepath.Join(shared.Cfg.DataDir, "friendsList"+strconv.Itoa(int(userId))+".json")
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
			err = os.WriteFile(filepath.Join(shared.Cfg.DataDir, "data.json"), updated, 0644)
			if err != nil {
				panic(err)
			}
		}
	}
}

func initServerJSON() {
	path := filepath.Join(shared.Cfg.DataDir, "data.json")
	_, err := os.Stat(path)
	serverUsers.ServerUsers = make(map[uint32]ClientInfo)

	if err == nil {
		bytes, err := os.ReadFile(path)
		if err != nil {
			log.Fatal(err)
		}
		if err = json.Unmarshal(bytes, &serverUsers); err != nil {
			log.Fatal(err)
		}
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

func Main() {
	globalChannel = make(chan shared.Message, 128)
	initServerJSON()
	go writeToServerJson()

	ln, err := net.Listen("tcp", shared.Port)
	if err != nil {
		panic(err)
	}

	go newUserListener(ln)

	for {
		message, ok := <-globalChannel
		if ok {
			sendMessageToEveryOne(message)
		}
	}
}
