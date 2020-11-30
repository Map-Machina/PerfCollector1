package types

import (
	"bytes"
	"encoding/gob"
	"time"
)

const (
	PCVersion = 1 // Protocol version.

	// Command identifiers
	PCCmd      = "cmd"   // Generic encapsulating command
	PCAck      = "ack"   // Acknowledge for commands that don't have a reply
	PCErrorCmd = "error" // Error reply to a command

	// Commands that have a reply.
	PCCollectOnceCmd             = "collectonce"             // Collect single measurement
	PCCollectOnceReplyCmd        = "collectoncereply"        // Reply to collect once
	PCCollectDirectoriesCmd      = "collectdirectories"      // Collect directory content
	PCCollectDirectoriesReplyCmd = "collectdirectoriesreply" // Reply to collectdirectories
	PCStatusCollectionCmd        = "statuscollection"        // Collection status
	PCStatusCollectionReplyCmd   = "statuscollectionreply"   // Collection status reply
	PCPrepareReplayCmd           = "preparereplay"           // Prepare replay
	PCPrepareReplayReplyCmd      = "preparereplayreply"      // Prepare replay reply

	// Commands that do not have a reply.
	PCStartCollectionCmd = "startcollection" // Start collecting measurements
	PCStopCollectionCmd  = "stopcollection"  // Stop collecting measurements
	PCRegisterSinkCmd    = "registersink"    // Register a sink

	PCChannel = "collector" // SSH channel name
)

// PCCommand encapsulates commands with a version and a tag.
type PCCommand struct {
	Version uint   // Protocol version
	Tag     uint   // Tag of command
	Cmd     string // Payload identifier
	Payload interface{}
}

// PCError is returned when an error occurs..
type PCError struct {
	Error string
}

// PCCollectOnce is a one time pull of prformance data.
type PCCollectOnce struct {
	Systems []string // Systems to grab
}

// PCCollectOnceReply is the reply to PCCollectOnce.
type PCCollectOnceReply struct {
	Values [][]byte // Index is same as PCCmdCollectOnce.System
}

// PCCollectDirectories is a pull of directories content.
type PCCollectDirectories struct {
	Directories []string // Directories to grab
}

// PCCollectDirectoriesReply is the reply to PCCollectDirectories. If a Value
// is a directory it has a trailing slash to indicate that.
type PCCollectDirectoriesReply struct {
	Values [][]string // Index is same as PCCmdCollectDirectories.Directories
}

// PCStartCollection instructs the collector to start gathering data with the
// provided parameters.
type PCStartCollection struct {
	Frequency  time.Duration // Collect performance data with this frequency
	Systems    []string      // Performance statistics to grab.
	QueueDepth int           // Max measurements before spilling
}

// PCPrepareReplay instructs the collector to start replaying a load that is
// coming in over the sink channel.
type PCPrepareReplay struct {
	Systems   []string      // Systems that will be exercised
	Frequency time.Duration // Speed at which the sync will be fed
}

// PCPrepareReplayReply returns the training data.
type PCPrepareReplayReply struct {
	Training map[int]int // Training data in 10% increments
}

// PCStatusCollectionReply is the status of the collection.
type PCStatusCollectionReply struct {
	StartCollection    *PCStartCollection // Original start collection dommand
	QueueFree          int                // Number of open slots on queue
	SinkEnabled        bool               // Is the sink enabled
	MeasurementEnabled bool               // Are measurements enabled
}

// PCCollection is a raw measurement that is sunk into the network.
type PCCollection struct {
	Timestamp   time.Time     // Time of *overall* collection
	Start       time.Time     // Start time of *this* collection
	Duration    time.Duration // Time collection took
	Frequency   time.Duration // Collection frequency
	System      string        // System that was measured
	Measurement string        // Raw measurement
}

// Encode encodes an interface with gob. This should only be called with types
// in this file.
func Encode(x interface{}) ([]byte, error) {
	var blob bytes.Buffer
	enc := gob.NewEncoder(&blob)
	err := enc.Encode(x)
	if err != nil {
		return nil, err
	}
	return blob.Bytes(), nil
}

// Decode decodes an interface with gob. This should only be called with types
// in this file.
func Decode(t string, blob []byte) (interface{}, error) {
	var s PCCommand
	dec := gob.NewDecoder(bytes.NewReader(blob))
	err := dec.Decode(&s)
	if err != nil {
		return nil, err
	}
	return s, err
}

// init registers all gob types.
func init() {
	gob.Register(PCError{})
	gob.Register(PCCollectOnce{})
	gob.Register(PCCollectOnceReply{})
	gob.Register(PCCollectDirectories{})
	gob.Register(PCCollectDirectoriesReply{})
	gob.Register(PCStartCollection{})
	gob.Register(PCStatusCollectionReply{})
	gob.Register(PCPrepareReplay{})
	gob.Register(PCPrepareReplayReply{})
}
