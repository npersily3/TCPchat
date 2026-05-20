package main

const CONTROL_MESSAGE = 1
const DATA_MESSAGE = 0
const SERVER_ID = 0

type InternalMessageData struct {
	MessageType int
	//Unenforced Rule in the code, if the message is a control,
	//the first byte is a server op code
	Contents []byte
}
type Message struct {
	SenderId uint32
	Payload  InternalMessageData
}
