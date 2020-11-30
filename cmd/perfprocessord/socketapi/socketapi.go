package socketapi

const (
	SocketFilename = ".socket" // socket filename

	SCVersion            = 1                    // socket API version
	SCPing               = "ping"               // ID for SocketCommandPing
	SCPingReply          = "pingreply"          // ID for SocketCommandPingReply
	SCPrepareReplay      = "preparereplay"      // ID for SocketCommandPrepareReplay
	SCPrepareReplayReply = "preparereplayreply" // ID for SocketCommandPrepareReplayReply
)

// SocketCommandID identifies the command that follows.
type SocketCommandID struct {
	Version uint
	Command string
}

// SocketCommandPing is a test command to test the API.
type SocketCommandPing struct {
	Timestamp int64 // Client timestamp
}

// SocketCommandPingReply is the reply to a ping command.
type SocketCommandPingReply struct {
	Timestamp int64 // Server timestamp
}

// SocketCommandPrepareReplay
type SocketCommandPrepareReplay struct {
	Filename string
}

// SocketCommandPrepareReplay
type SocketCommandPrepareReplayReply struct {
	Error []error
}
