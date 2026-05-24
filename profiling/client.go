package main

import (
	"encoding/gob"
	"log"
	"net"
	"time"

	"TCPchat/internal/shared"
)

type profClient struct {
	id   uint32
	name string
	conn net.Conn
	enc  *gob.Encoder
	dec  *gob.Decoder
}

func profConnect(id uint32, name string) *profClient {
	conn, err := net.Dial("tcp", "127.0.0.1"+shared.Port)
	if err != nil {
		log.Fatalf("profConnect %q: %v", name, err)
	}
	c := &profClient{
		id:   id,
		name: name,
		conn: conn,
		enc:  gob.NewEncoder(conn),
		dec:  gob.NewDecoder(conn),
	}
	if err := c.enc.Encode(shared.Message{
		SenderId: id,
		Payload:  shared.InternalMessageData{OPcode: shared.DATA_MESSAGE, Contents: []byte(name)},
	}); err != nil {
		conn.Close()
		log.Fatalf("profConnect %q: handshake: %v", name, err)
	}
	return c
}

func (c *profClient) drain(n int, timeout time.Duration) []shared.Message {
	deadline := time.Now().Add(timeout)
	var msgs []shared.Message
	for len(msgs) < n {
		c.conn.SetReadDeadline(deadline)
		var m shared.Message
		if err := c.dec.Decode(&m); err != nil {
			break
		}
		msgs = append(msgs, m)
	}
	c.conn.SetReadDeadline(time.Time{})
	return msgs
}

func (c *profClient) sendMsg(text string) {
	if err := c.enc.Encode(shared.Message{
		SenderId: c.id,
		Payload:  shared.InternalMessageData{OPcode: shared.DATA_MESSAGE, Contents: []byte(text)},
	}); err != nil {
		log.Fatalf("%s sendMsg: %v", c.name, err)
	}
}
