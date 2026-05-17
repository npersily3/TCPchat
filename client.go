package main

import "net"

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

	conn.Read(message)

	println(string(message))

	conn.Close()
}
