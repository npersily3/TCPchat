package main

// type Message interface {
// }
//
//	type PayloadMessage struct {
//		SenderId uint32
//		Contents string
//	}
//
//	type ControlMessage struct {
//		Payload []byte
//	}
type Message struct {
	SenderId uint32
	Contents string
}
