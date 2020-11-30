package main

import (
	"encoding/gob"
	"net"
	"time"

	"github.com/businessperformancetuning/perfcollector/cmd/perfprocessord/socketapi"
)

func (p *PerfCtl) socketDial() (net.Conn, error) {
	return net.Dial("unix", p.cfg.SocketFilename)
}

func (p *PerfCtl) ping() (*socketapi.SocketCommandPingReply, error) {
	c, err := p.socketDial()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	// send identifier
	ge := gob.NewEncoder(c)
	err = ge.Encode(socketapi.SocketCommandID{
		Version: socketapi.SCVersion,
		Command: socketapi.SCPing,
	})
	if err != nil {
		return nil, err
	}

	err = ge.Encode(socketapi.SocketCommandPing{
		Timestamp: time.Now().Unix(),
	})
	if err != nil {
		return nil, err
	}

	// read reply
	gd := gob.NewDecoder(c)
	var pr socketapi.SocketCommandPingReply
	err = gd.Decode(&pr)
	if err != nil {
		return nil, err
	}

	return &pr, nil
}

func (p *PerfCtl) socketPrepareReply(filename string) (*socketapi.SocketCommandPrepareReplayReply, error) {
	c, err := p.socketDial()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	// send identifier
	ge := gob.NewEncoder(c)
	err = ge.Encode(socketapi.SocketCommandID{
		Version: socketapi.SCVersion,
		Command: socketapi.SCPrepareReplay,
	})
	if err != nil {
		return nil, err
	}

	err = ge.Encode(socketapi.SocketCommandPrepareReplay{
		Filename: filename,
	})
	if err != nil {
		return nil, err
	}

	// read reply
	gd := gob.NewDecoder(c)
	var pr socketapi.SocketCommandPrepareReplayReply
	err = gd.Decode(&pr)
	if err != nil {
		return nil, err
	}

	return &pr, nil
}
