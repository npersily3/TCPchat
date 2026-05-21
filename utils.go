package main

//list of opcodes to communicate with

const NEW_USER_ONLINE = 2
const NEW_USERNAME = 1
const DATA_MESSAGE = 0

type InternalMessageData struct {
	OPcode int
	//Unenforced Rule in the code, if the message is a control,
	//the first byte is a server op code
	Contents []byte
}
type Message struct {
	SenderId uint32
	Payload  InternalMessageData
}

type Client struct {
	isOnline bool
	Name     string
}

// UserData we reserve ID 0 for ourselves which holds to value of our id as a string
// then we know at the start what our ID is, ID 0 also means it is the server communicating to us what happens
type UserData struct {
	ClientUsers map[uint32]Client `json:"users"`
}
