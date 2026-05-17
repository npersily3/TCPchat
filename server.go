package main

import (
	"encoding/binary"
	"net"
)

type ClientInfo struct {
	conn     net.Conn
	uniqueId uint32
	channel  chan []byte

	// have this be last because it is of variable size
	userName string
}

type Message struct {
	senderId uint32
	contents string
}

var users = map[uint32]ClientInfo{}

func perClientReader(userId uint32) {

}
func perClientWriter(userId uint32) {

}

// assumes the first message is a
func handleConn(conn net.Conn) {

	var clientInfo ClientInfo
	var rawData []byte
	n, err := conn.Read(rawData)

	if err != nil {
		panic(err)
	}
	rawData = rawData[:n]

	clientInfo.conn = conn
	// read the first four bytes
	id := binary.BigEndian.Uint32(rawData[:4])
	clientInfo.uniqueId = id

	_, ok := users[clientInfo.uniqueId]

	// if we exist in the hashmap (have been online before
	if ok {

		// if  we are a new user, initialize, the name and send a message to send a key value of id to name
	} else {
		clientInfo.userName = string(rawData[4:n])
		users[clientInfo.uniqueId] = clientInfo

		//TODO send a key value of name and id for clients to keep
	}

	//TODO make maybe an enum or some version of a code
	n, err = conn.Write([]byte("initialized"))

	if err != nil {
		panic(err)
	}

	// now that the thread is initialized, we can pull off
	go perClientWriter(id)
	go perClientReader(id)
}

// Constantly listens for new users, and initializes them
func newUserListener(ln net.Listener) {

	// see if this is always spin or something else
	for {
		conn, err := ln.Accept()

		if err != nil {
			panic(err)
		}
		go handleConn(conn)

	}
}

func server_main() {
	ln, err := net.Listen("tcp", ":1000")

	if err != nil {
		panic(err)
	}

	// initialize all new users
	go newUserListener(ln)

}
