package main

import (
	"fmt"
	"net"
)

func listen(conn net.Conn) {

}

func readMessage(conn net.Conn) {
	message := make([]byte, 1024)

	numbytes, err := conn.Read(message)

	if err != nil {
		panic(err)
	}

	println(string(message[:numbytes]))
}

func writeMessage(conn net.Conn, data []byte) {
	n, err := conn.Write(data)
	if err != nil {
		panic(err)
	}
	if n != len(data) {
		panic("short write")
	}
}

func client_main() {

	conn, err := net.Dial("tcp", ":1000")

	for {
		if err != nil {
			conn, err = net.Dial("tcp", ":1000")
		} else {
			break
		}
	}

	message := make([]byte, 1024)

	// wait until this client is initialized in the server
	for {
		n, err := conn.Read(message)

		if err != nil {
			panic(err)
		}
		if string(message[:n]) == "initialized" {
			break
		}
	}

	go readMessage(conn)

	for {
		println("Enter a message")

		length, err := fmt.Scanln(&message)

		if err != nil {
			panic(err)
		}

		writeMessage(conn, message[:length])

	}

	err = conn.Close()

	if err != nil {
		println(err.Error())
	}

}
