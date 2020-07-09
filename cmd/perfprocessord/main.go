package main

import (
	"encoding/gob"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/businessperformancetuning/sizer/parser"
	"github.com/businessperformancetuning/sizer/types"
	"github.com/businessperformancetuning/sizer/util"
	"github.com/davecgh/go-spew/spew"
	"golang.org/x/crypto/ssh"
)

type PerfCtl struct {
	sync.RWMutex

	cfg *config

	tag  uint                      // Last used tag
	tags map[uint]chan interface{} // Tag callback
}

func (p *PerfCtl) send(channel ssh.Channel, cmd types.PCCommand, callback chan interface{}) error {
	log.Tracef("send")
	defer log.Tracef("send exit")

	// Hnadle tag
	p.Lock()
	tag := p.tag
	if _, ok := p.tags[tag]; ok {
		p.Unlock()
		return fmt.Errorf("duplicate tag: %v", tag)
	}
	p.tags[tag] = callback
	p.tag++
	p.Unlock()

	// Send OOB
	cmd.Version = types.PCVersion // Set version
	cmd.Tag = tag                 // Set tag

	// Do expensive encode first
	reply, err := types.Encode(cmd)
	if err != nil {
		return nil
	}

	log.Tracef("send %v", spew.Sdump(cmd))

	_, err = channel.SendRequest(types.PCCmd, false, reply)

	return err
}

func (p *PerfCtl) sendAndWait(channel ssh.Channel, cmd types.PCCommand) (interface{}, error) {
	log.Tracef("sendAndWait")
	defer log.Tracef("sendAndWait exit")

	// Callback channel
	c := make(chan interface{})

	err := p.send(channel, cmd, c)
	if err != nil {
		return nil, err
	}
	reply := <-c
	// Check to see if we got an error
	if e, ok := reply.(error); ok {
		return nil, e
	}

	return reply, nil
}

func (p *PerfCtl) oobHandler(channel ssh.Channel, requests <-chan *ssh.Request) {
	log.Tracef("oobHandler")
	defer log.Tracef("oobHandler exit")

	for req := range requests {
		log.Tracef("oobHandler req.Type: %v", req.Type)

		// Always reply or else the other side may hang.
		req.Reply(true, nil)

		// Handle command.
		if req.Type != types.PCCmd {
			log.Errorf("oobHandler unknown request: %v", req.Type)
			continue
		}

		c, err := types.Decode(req.Type, req.Payload)
		if err != nil {
			log.Errorf("oobHandler decode error: %v", err)
			continue
		}
		cmd, ok := c.(types.PCCommand)
		if !ok {
			// Should not happen
			log.Errorf("oobHandler type assertion error %T", c)
			continue
		}

		log.Tracef("oobHandler tag %v", cmd.Tag)
		// Free tag
		p.Lock()
		callback, ok := p.tags[cmd.Tag]
		if !ok {
			p.Unlock()
			log.Errorf("oobHandler unknown tag: %v", cmd.Tag)
			continue
		}
		delete(p.tags, cmd.Tag)
		p.Unlock()

		var reply interface{}
		switch cmd.Cmd {
		case types.PCAck:
			log.Tracef("oobHandler ack %v", cmd.Tag)
		case types.PCErrorCmd:
			// Log error and move on.
			e, ok := cmd.Payload.(types.PCError)
			if ok {
				reply = fmt.Errorf("oobHandler remote error: "+
					"version: %v tag: %v cmd: '%v' error: %v",
					cmd.Version, cmd.Tag, cmd.Cmd, e.Error)
			} else {
				// Should not happen
				log.Errorf("oobHandler command type assertion "+
					"error: %T", cmd.Payload)
			}

		case types.PCCollectOnceReplyCmd:
			o, ok := cmd.Payload.(types.PCCollectOnceReply)
			if ok {
				reply = o
			} else {
				// Should not happen
				log.Errorf("type assertion error %T", cmd.Payload)
			}

		default:
			log.Errorf("oobHandler unknown request: %v", cmd.Cmd)
			reply := types.PCCommand{
				Version: types.PCVersion,
				Tag:     cmd.Tag,
				Cmd:     types.PCErrorCmd,
				Payload: types.PCError{
					Error: "unknown OOB request: " + cmd.Cmd,
				},
			}
			// Send payload to server.
			err = p.send(channel, reply, nil)
			if err != nil {
				log.Errorf("oobHandler SendRequest: %v", err)
			}
		}

		if callback != nil {
			callback <- reply
		}

	}
}

func _main() error {
	// Load configuration and parse command line.  This function also
	// initializes logging and configures it accordingly.
	loadedCfg, _, err := loadConfig()
	if err != nil {
		return fmt.Errorf("Could not load configuration file: %v", err)
	}
	defer func() {
		if logRotator != nil {
			logRotator.Close()
		}
	}()

	pc := &PerfCtl{
		cfg:  loadedCfg,
		tags: make(map[uint]chan interface{}),
	}

	log.Debugf("Version      : %v", version())
	log.Debugf("Home dir     : %v", pc.cfg.HomeDir)

	pk, err := util.PublicKeyFile(pc.cfg.SSHKeyFile)
	if err != nil {
		return err
	}
	config := &ssh.ClientConfig{
		User: pc.cfg.User,
		Auth: []ssh.AuthMethod{pk},
		//HostKeyCallback: ssh.FixedHostKey(hostKey),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// Connect to ssh server
	conn, err := ssh.Dial("tcp", pc.cfg.Host, config)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Setup channel.
	channel, requests, err := conn.OpenChannel(types.PCChannel, nil)
	if err != nil {
		return err
	}
	defer channel.Close()

	// Setup out of band handler.
	go pc.oobHandler(channel, requests)

	// Do one time collection
	//reply, err := pc.sendAndWait(channel, types.PCCommand{
	//	Cmd: types.PCCollectOnceCmd,
	//	Payload: types.PCCollectOnce{
	//		Systems: []string{"version", "uptime"},
	//	},
	//})
	//if err != nil {
	//	return err
	//}
	//spew.Dump(reply)
	// Register this connection as the flusher
	_, err = pc.sendAndWait(channel, types.PCCommand{
		Cmd: types.PCRegisterSink,
	})
	if err != nil {
		return err
	}

	_, err = pc.sendAndWait(channel, types.PCCommand{
		Cmd: types.PCStartCollectionCmd,
		Payload: types.PCStartCollection{
			Frequency:  5 * time.Second,
			QueueDepth: 3, //10000,
			Systems:    []string{"stat", "meminfo"},
		},
	})
	if err != nil {
		return err
	}

	// Setup streaming
	_, err = channel.Write([]byte("Hello world from client\n"))
	if err != nil {
		return err
	}

	dec := gob.NewDecoder(channel)
	for {
		var m types.PCCollection
		err := dec.Decode(&m)
		if err != nil {
			return err
		}

		// Post process
		switch m.System {
		case "stat":
			s, err := parser.ProcessStat(m.Measurement)
			if err != nil {
				log.Errorf("could not process stat: %v", err)
				continue
			}
			spew.Dump(s)
		case "meminfo":
			s, err := parser.ProcessMeminfo(m.Measurement)
			if err != nil {
				log.Errorf("could not process meminfo: %v", err)
				continue
			}
			spew.Dump(s)
		default:
			log.Errorf("unknown system: %v", m.System)
		}
	}

	return nil
}

func main() {
	err := _main()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
