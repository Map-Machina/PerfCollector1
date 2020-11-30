package socketapi

const (
	SocketFilename = ".socket" // socket filename

	SCVersion = 1      // socket API version
	SCPing    = "ping" // ID for SocketCommandPing
)

// SocketCommandID identifies the command that follows.
type SocketCommandID struct {
	Version uint   `json:"version"`
	Command string `json:"command"`
}

// SocketCommandPing is a test command to test the API.
type SocketCommandPing struct {
	Timestamp int64 `json:"timestamp"` // Client timestamp
}

// SocketCommandPingReply is the reply to a ping command.
type SocketCommandPingReply struct {
	Timestamp int64 `json:"timestamp"` // Server timestamp
}
