package main

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/businessperformancetuning/perfcollector/database"
	"github.com/businessperformancetuning/perfcollector/database/postgres"
	"github.com/businessperformancetuning/perfcollector/types"
	"github.com/businessperformancetuning/perfcollector/util"
	"github.com/davecgh/go-spew/spew"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

type terminalError struct {
	err error
}

func (te terminalError) Error() string {
	return te.err.Error()
}

type session struct {
	address  string
	conn     *ssh.Client
	channel  ssh.Channel
	requests <-chan *ssh.Request
}

func (p *PerfCtl) register(address string, s *session) error {
	log.Tracef("register: %v", address)
	defer log.Tracef("register exit: %v ", address)

	p.Lock()
	defer p.Unlock()

	if _, ok := p.sessions[address]; ok {
		return fmt.Errorf("session already registered: %v", address)
	}
	p.sessions[address] = s

	return nil
}

func (p *PerfCtl) unregister(address string) error {
	log.Tracef("unregister: %v", address)
	defer log.Tracef("unregister exit: %v ", address)

	p.Lock()
	defer p.Unlock()

	if s, ok := p.sessions[address]; ok {
		s.conn.Close()
		s.channel.Close()
		delete(p.sessions, address)
	} else {
		return fmt.Errorf("session not registered: %v", address)
	}

	return nil
}

func (p *PerfCtl) unregisterAll() {
	log.Tracef("unregisterAll")
	defer log.Tracef("unregisterAll exit")

	p.Lock()
	defer p.Unlock()

	for k, v := range p.sessions {
		v.conn.Close()
		v.channel.Close()
		delete(p.sessions, k)
	}
}

type PerfCtl struct {
	sync.RWMutex

	cfg *config

	db database.Database

	sessions map[string]*session // XXX ugh pointer, fix

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
	blob, err := types.Encode(cmd)
	if err != nil {
		return nil
	}

	log.Tracef("send %v", spew.Sdump(cmd))

	_, err = channel.SendRequest(types.PCCmd, false, blob)

	return err
}

func (p *PerfCtl) sendAndWait(ctx context.Context, channel ssh.Channel, cmd types.PCCommand) (interface{}, error) {
	log.Tracef("sendAndWait")
	defer log.Tracef("sendAndWait exit")

	// Callback channel
	c := make(chan interface{})

	err := p.send(channel, cmd, c)
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("sendAndWait abnormal termination")
	case reply, ok := <-c:
		if !ok {
			return nil, fmt.Errorf("sendAndWait channel closed")
		}

		// Check to see if we got an error
		if e, ok := reply.(error); ok {
			return nil, e
		}

		return reply, nil
	}
}

func (p *PerfCtl) oobHandler(s *session) error {
	log.Tracef("oobHandler: %v", s.address)
	defer func() {
		log.Tracef("oobHandler exit: %v", s.address)
	}()

	for req := range s.requests {
		log.Tracef("oobHandler req.Type %v: %v", s.address, req.Type)

		// Always reply or else the other side may hang.
		req.Reply(true, nil)

		// Handle command.
		if req.Type != types.PCCmd {
			log.Errorf("oobHandler unknown request %v: %v",
				s.address, req.Type)
			continue
		}

		c, err := types.Decode(req.Type, req.Payload)
		if err != nil {
			log.Errorf("oobHandler decode error %v: %v",
				s.address, err)
			continue
		}
		cmd, ok := c.(types.PCCommand)
		if !ok {
			// Should not happen
			log.Errorf("oobHandler type assertion error %v: %T",
				s.address, c)
			continue
		}

		log.Tracef("oobHandler tag %v: %v %T", s.address, cmd.Tag,
			cmd.Payload)

		// Free tag
		p.Lock()
		callback, ok := p.tags[cmd.Tag]
		if !ok {
			p.Unlock()
			log.Errorf("oobHandler unknown tag %v: %v",
				s.address, cmd.Tag)
			continue
		}
		delete(p.tags, cmd.Tag)
		p.Unlock()

		var reply interface{}
		switch cmd.Cmd {
		case types.PCAck:
			log.Tracef("oobHandler ack %v: %v", s.address, cmd.Tag)
		case types.PCErrorCmd:
			// Log error and move on.
			e, ok := cmd.Payload.(types.PCError)
			if ok {
				reply = fmt.Errorf("oobHandler remote error "+
					"%v: version: %v tag: %v cmd: '%v' "+
					"error: %v", s.address, cmd.Version,
					cmd.Tag, cmd.Cmd, e.Error)
			} else {
				// Should not happen
				log.Errorf("oobHandler command type assertion "+
					"error %v: %T", s.address, cmd.Payload)
			}

		case types.PCCollectOnceReplyCmd:
			o, ok := cmd.Payload.(types.PCCollectOnceReply)
			if ok {
				reply = o
			} else {
				// Should not happen
				log.Errorf("type assertion error %v: %T",
					s.address, cmd.Payload)
			}

		case types.PCStatusCollectionCmd:
			status, ok := cmd.Payload.(types.PCStatusCollectionReply)
			if ok {
				reply = status
				spew.Dump(reply)
			} else {
				// Should not happen
				log.Errorf("type assertion error %v: %T",
					s.address, cmd.Payload)
			}

		default:
			log.Errorf("oobHandler unknown request %v: %v",
				s.address, cmd.Cmd)
			reply := types.PCCommand{
				Version: types.PCVersion,
				Tag:     cmd.Tag,
				Cmd:     types.PCErrorCmd,
				Payload: types.PCError{
					Error: "unknown OOB request: " + cmd.Cmd,
				},
			}
			// Send payload to server.
			err = p.send(s.channel, reply, nil)
			if err != nil {
				log.Errorf("oobHandler SendRequest %v: %v",
					s.address, err)
			}
		}

		if callback != nil {
			callback <- reply
		}
	}

	return io.EOF
}

func (p *PerfCtl) singleCommand(ctx context.Context, s *session, args []string) error {
	log.Tracef("singleCommand: args %v", args)
	defer func() {
		log.Tracef("singleCommand exit: args %v", args)
	}()

	if len(args) == 0 {
		return fmt.Errorf("impossible args length")
	}

	switch args[0] {
	case "status":
		r, err := p.sendAndWait(ctx, s.channel, types.PCCommand{
			Cmd: types.PCStatusCollectionCmd,
		})
		if err != nil {
			return err
		}
		log.Infof("%v", r)

	case "start":
		_, err := p.sendAndWait(ctx, s.channel, types.PCCommand{
			Cmd: types.PCStartCollectionCmd,
			Payload: types.PCStartCollection{
				Frequency:  5 * time.Second,
				QueueDepth: 3, //10000,
				Systems: []string{
					"/proc/stat",
					"/proc/meminfo"},
			},
		})
		if err != nil {
			return err
		}

	case "stop":
		_, err := p.sendAndWait(ctx, s.channel, types.PCCommand{
			Cmd: types.PCStopCollectionCmd,
		})
		if err != nil {
			return err
		}

	default:
		return fmt.Errorf("unknown command: %v", args[0])
	}
	return nil
}

func (p *PerfCtl) handleArgs(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("impossible args length")
	}

	// Validate args before doing expensive things.
	switch args[0] {
	case "status":
	case "start":
	case "stop":
	default:
		return fmt.Errorf("unknown command: %v", args[0])
	}

	// Context.
	ctx, cancel := context.WithCancel(context.Background())

	var eg errgroup.Group
	for k, v := range p.cfg.HostsId {
		log.Infof("Connecting %v:%v/%v", v.Site, v.Host, k)

		session, err := p.connect(ctx, k)
		if err != nil {
			log.Errorf("connect: %v", err)
			continue
		}

		// XXX this is probably not right with a single failing command

		// Setup out of band handler.
		eg.Go(func() error {
			err := p.oobHandler(session)
			if err != nil {
				log.Errorf("handleArgs oobHandler: %v", err)
				cancel()
			}
			return err
		})

		eg.Go(func() error {
			err := p.singleCommand(ctx, session, args)
			if err != nil {
				log.Errorf("handleArgs singleCommand: %v", err)
			}
			session.channel.Close()
			return err
		})
	}

	// Wait for exit
	log.Infof("Waiting for commands to complete")
	eg.Wait()

	return nil
}

func (p *PerfCtl) connect(ctx context.Context, address string) (*session, error) {
	log.Tracef("connect: %v", address)
	defer log.Tracef("connect exit: %v", address)

	pk, err := util.PublicKeyFile(p.cfg.SSHKeyFile)
	if err != nil {
		return nil, err
	}
	config := &ssh.ClientConfig{
		Auth: []ssh.AuthMethod{pk},
		//HostKeyCallback: ssh.FixedHostKey(hostKey),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // XXX security issue
		Timeout:         5 * time.Second,
	}

	// Connect to ssh server
	conn, err := ssh.Dial("tcp", address, config)
	if err != nil {
		return nil, err
	}

	// Setup channel.
	channel, requests, err := conn.OpenChannel(types.PCChannel, nil)
	if err != nil {
		return nil, err
	}

	session := &session{
		conn:     conn,
		channel:  channel,
		requests: requests,
		address:  address,
	}
	p.register(address, session)

	return session, nil
}

func (p *PerfCtl) journal(site, host, run uint64, measurement types.PCCollection) error {
	if !util.ValidSystem(measurement.System) {
		return fmt.Errorf("journal unsupported system: %v",
			measurement.System)
	}

	filename := filepath.Join(p.cfg.DataDir, strconv.Itoa(int(site)),
		strconv.Itoa(int(host)), strconv.Itoa(int(run)), measurement.System)
	dir := filepath.Dir(filename)
	err := os.MkdirAll(dir, 0750)
	if err != nil {
		return err
	}

	// Journal in JSON to remain human readability.
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0640)
	if err != nil {
		return err
	}
	defer f.Close()

	// Create encoder
	e := json.NewEncoder(f)
	err = e.Encode(measurement)
	if err != nil {
		return err
	}

	return nil
}

func (p *PerfCtl) sinkLoop(ctx context.Context, site, host uint64, address string) error {
	log.Tracef("sinkLoop %v:%v", site, host)
	defer log.Tracef("sinkLoop exit %v:%v", site, host)

	s, err := p.connect(ctx, address)
	if err != nil {
		log.Errorf("sendAndWait connect: %v", err)
		return err
	}

	defer func() {
		if err := p.unregister(address); err != nil {
			log.Errorf("sink exit unregister: %v", err)
		}
	}()

	// Setup out of band handler.
	go p.oobHandler(s)

	// Register sinkLoop.
	_, err = p.sendAndWait(ctx, s.channel, types.PCCommand{
		Cmd: types.PCRegisterSink,
	})
	if err != nil {
		log.Errorf("sendAndWait connect: %v", err)
		return terminalError{err: err}
	}

	run := uint64(0)
	// We are in sinkLoop mode. Register sinkLoop and process measurements.
	dec := gob.NewDecoder(s.channel)
	for {
		var m types.PCCollection
		err := dec.Decode(&m)
		if err != nil {
			return fmt.Errorf("sinkLoop Decode %v:%v: %v",
				site, host, err)
		}
		log.Tracef("Received record")
		if p.cfg.Journal {
			err := p.journal(site, host, run, m)
			if err != nil {
				return fmt.Errorf("sinkLoop journal %v:%v: %v",
					site, host, err)
			}
			continue
		}

		//// Post process
		//switch m.System {
		//case "/proc/stat":
		//	s, err := parser.ProcessStat(m.Measurement)
		//	if err != nil {
		//		log.Errorf("could not process stat: %v", err)
		//		continue
		//	}
		//	//spew.Dump(s)

		//case "/proc/meminfo":
		//	s, err := parser.ProcessMeminfo(m.Measurement)
		//	if err != nil {
		//		log.Errorf("could not process meminfo: %v", err)
		//		continue
		//	}
		//	//spew.Dump(s)

		//	//// Insert runid
		//	//m := database.Measurements{
		//	//	SiteID: 1, // User provided
		//	//	HostID: 2, // User provided
		//	//}
		//	//runId, err := db.MeasurementsInsert(&m)
		//	//if err != nil {
		//	//	log.Errorf("could not insert measurement: %v", err)
		//	//	continue
		//	//}

		//	//// Insert meminfo
		//	//ss := database.Meminfo2{
		//	//	database.MeminfoIdentifiers{
		//	//		12,
		//	//	},
		//	//	database.Collection{
		//	//		Timestamp: m.Timestamp.UnixNano(),
		//	//		Duration:  m.Duration,
		//	//	},
		//	//	s,
		//	//}
		//	//err = pc.db.MeminfoInsert(&ss)
		//	//if err != nil {
		//	//	log.Errorf("sink MeminfoInsert: %v", err)
		//	//}

		//	//// Insert stat

		//	//// Insert net IO

		//	//// Insert block IO
		//default:
		//	log.Errorf("unknown system: %v", m.System)
		//}
	}
}

func (p *PerfCtl) sink(ctx context.Context, site, host uint64, address string) error {
	log.Tracef("sink %v:%v", site, host)

	defer func() {
		log.Tracef("sink exit %v:%v", site, host)
	}()
	// Always reconnect unless canceled
	for {
		err := p.sinkLoop(ctx, site, host, address)
		if err != nil {
			if _, ok := err.(terminalError); ok {
				log.Errorf("sink error: %v", err)
				return err
			}
			// This may be too loud
			log.Errorf("sink %v:%v: %v", site, host, err)
		}

		select {
		case <-ctx.Done():
			break
		case <-time.After(5 * time.Second):
		}
	}

	return nil
}

func _main() error {
	// Load configuration and parse command line.  This function also
	// initializes logging and configures it accordingly.
	loadedCfg, args, err := loadConfig()
	if err != nil {
		return fmt.Errorf("Could not load configuration file: %v", err)
	}
	defer func() {
		if logRotator != nil {
			logRotator.Close()
		}
	}()

	p := &PerfCtl{
		cfg:      loadedCfg,
		tags:     make(map[uint]chan interface{}),
		sessions: make(map[string]*session),
	}

	log.Infof("Version         : %v", version())
	log.Infof("Home dir        : %v", p.cfg.HomeDir)

	// Execute, this needs to come out
	if len(args) != 0 {
		return p.handleArgs(args)
	}

	// Prepare database
	switch p.cfg.DB {
	case "postgres":
		postgres.UseLogger(dbLog)
		p.db, err = postgres.New(database.Name, p.cfg.DBURI)
	default:
		return fmt.Errorf("Invalid database type: %v", p.cfg.DB)
	}
	if err != nil {
		return err
	}

	// Open and Close db on exit.
	if err := p.db.Open(); err != nil {
		return err
	}
	defer p.db.Close()
	log.Infof("Database version: %v", database.Version)

	// Context.
	ctx, cancel := context.WithCancel(context.Background())

	var eg errgroup.Group
	for k, v := range p.cfg.HostsId {
		log.Infof("Connecting %v:%v/%v", v.Site, v.Host, k)
		eg.Go(func() error {
			err := p.sink(ctx, v.Site, v.Host, k)
			if err != nil {
				cancel()
			}
			return err
		})
	}

	// Setup OS signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGINT)
	for {
		select {
		case sig := <-sigs:
			log.Infof("Terminating with %v", sig)
			cancel()
			p.unregisterAll()
			goto done
		case <-ctx.Done():
			goto done
		}
	}
done:

	// Wait for exit
	log.Infof("Waiting to exit")

	return nil
}

func main() {
	err := _main()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
