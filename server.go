package main

import "net"

func server_main() {
	ln, err := net.Listen("tcp", ":1000")

	if err != nil {
		panic(err)
	}

	conn, err := ln.Accept()

	if err != nil {
		panic(err)
	}

	conn.Write([]byte("hello user, I am server"))

	conn.Close()
}
