package types

import (
	"bytes"
	"encoding/gob"
	"time"
)

const (
	PCVersion = 1 // Protocol version.

	// Command identifiers
	PCCmd                 = "cmd" // Generic encapsulating command
	PCAck                 = "ack" // Acknowledge command
	PCErrorCmd            = "error"
	PCCollectOnceCmd      = "collectonce"
	PCCollectOnceReplyCmd = "collectoncereply"
	PCStartCollectionCmd  = "startcollection"
	PCStopCollectionCmd   = "stopcollection"
	PCRegisterSink        = "registersink"

	PCChannel = "collector" // SSH channel name
)

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

// PCStartCollection instructs the collector to start gathering data with the
// provided parameters.
type PCStartCollection struct {
	Frequency  time.Duration // Collect performance data with this frequency
	Systems    []string      // Performance statistics to grab.
	QueueDepth int           // Max measurements before spilling
}

// PCStopCollectionCmd instructs the collector to stop collecting measurements.
type PCStopCollection struct {
	Destroy bool // When set the uncollected measurements will be discarded
}

// PCStatus is the status of the collection.
type PCCollectionStatus struct {
	Frequency time.Time // Frequency of the collection
	Systems   []string  // Systems that are being polled
}

type PCCollection struct {
	Timestamp   time.Time     // Time of collection
	Duration    time.Duration // Time collection took
	System      string        // System tha was measured
	Measurement []byte        // Raw measurement
}

func Encode(x interface{}) ([]byte, error) {
	var blob bytes.Buffer
	enc := gob.NewEncoder(&blob)
	err := enc.Encode(x)
	if err != nil {
		return nil, err
	}
	return blob.Bytes(), nil
}

func Decode(t string, blob []byte) (interface{}, error) {
	var s PCCommand
	dec := gob.NewDecoder(bytes.NewReader(blob))
	err := dec.Decode(&s)
	if err != nil {
		return nil, err
	}
	return s, err
}

func init() {
	gob.Register(PCError{})
	gob.Register(PCCollectOnce{})
	gob.Register(PCCollectOnceReply{})
	gob.Register(PCStartCollection{})
	gob.Register(PCStopCollection{})
	gob.Register(PCCollectionStatus{})
}
