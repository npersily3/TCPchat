package shared

const (
	DATA_MESSAGE          = 0
	NEW_USERNAME          = 1
	NEW_USER_ONLINE       = 2
	EXISTING_USER         = 3
	NEW_USER_WHO_WAS_ONLINE = 4
)

type InternalMessageData struct {
	OPcode int
	// If the message is a control message, the first byte is a server op code.
	Contents []byte
}

type Message struct {
	SenderId uint32
	Payload  InternalMessageData
}

type Client struct {
	IsOnline bool
	Name     string
}

// UserData reserves ID 0 for the local user (stores their own ID as a string),
// then maps all other known peer IDs to their Client records.
type UserData struct {
	ClientUsers map[uint32]Client `json:"users"`
}
