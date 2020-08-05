package main

import (
	"context"
	"encoding/gob"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/businessperformancetuning/perfcollector/database"
	"github.com/businessperformancetuning/perfcollector/database/postgres"
	"github.com/businessperformancetuning/perfcollector/types"
	"github.com/businessperformancetuning/perfcollector/util"
	"github.com/davecgh/go-spew/spew"
	"golang.org/x/crypto/ssh"
)

type session struct {
	conn    *ssh.Client
	channel ssh.Channel
}

func (p *PerfCtl) register(address string, s session) error {
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
	wg sync.WaitGroup

	cfg *config

	db database.Database

	sessions map[string]session

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

func (p *PerfCtl) sendAndWait(ctx context.Context, channel ssh.Channel, cmd types.PCCommand) (interface{}, error) {
	log.Tracef("sendAndWait")
	defer log.Tracef("sendAndWait exit")

	// Callback channel
	c := make(chan interface{})

	err := p.send(channel, cmd, c)
	if err != nil {
		return nil, err
	}

	var reply interface{}
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("sendAndWait abnormal termination")
	case reply = <-c:
	}

	// Check to see if we got an error
	if e, ok := reply.(error); ok {
		return nil, e
	}

	return reply, nil
}

func (p *PerfCtl) oobHandler(ctx context.Context, address string, channel ssh.Channel, requests <-chan *ssh.Request) {
	log.Tracef("oobHandler: %v", address)
	_, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		log.Tracef("oobHandler exit: %v", address)
	}()

	for req := range requests {
		log.Tracef("oobHandler req.Type %v: %v", address, req.Type)

		// Always reply or else the other side may hang.
		req.Reply(true, nil)

		// Handle command.
		if req.Type != types.PCCmd {
			log.Errorf("oobHandler unknown request %v: %v",
				address, req.Type)
			continue
		}

		c, err := types.Decode(req.Type, req.Payload)
		if err != nil {
			log.Errorf("oobHandler decode error %v: %v",
				address, err)
			continue
		}
		cmd, ok := c.(types.PCCommand)
		if !ok {
			// Should not happen
			log.Errorf("oobHandler type assertion error %v: %T",
				address, c)
			continue
		}

		log.Tracef("oobHandler tag %v: %v", address, cmd.Tag)
		// Free tag
		p.Lock()
		callback, ok := p.tags[cmd.Tag]
		if !ok {
			p.Unlock()
			log.Errorf("oobHandler unknown tag %v: %v",
				address, cmd.Tag)
			continue
		}
		delete(p.tags, cmd.Tag)
		p.Unlock()

		var reply interface{}
		switch cmd.Cmd {
		case types.PCAck:
			log.Tracef("oobHandler ack %v: %v", address, cmd.Tag)
		case types.PCErrorCmd:
			// Log error and move on.
			e, ok := cmd.Payload.(types.PCError)
			if ok {
				reply = fmt.Errorf("oobHandler remote error "+
					"%v: version: %v tag: %v cmd: '%v' "+
					"error: %v", address, cmd.Version,
					cmd.Tag, cmd.Cmd, e.Error)
			} else {
				// Should not happen
				log.Errorf("oobHandler command type assertion "+
					"error %v: %T", address, cmd.Payload)
			}

		case types.PCCollectOnceReplyCmd:
			o, ok := cmd.Payload.(types.PCCollectOnceReply)
			if ok {
				reply = o
			} else {
				// Should not happen
				log.Errorf("type assertion error %v: %T",
					address, cmd.Payload)
			}

		case types.PCStatusCollectionCmd:
			s, ok := cmd.Payload.(types.PCStatusCollectionReply)
			if ok {
				reply = s
				spew.Dump(reply)
			} else {
				// Should not happen
				log.Errorf("type assertion error %v: %T",
					address, cmd.Payload)
			}

		default:
			log.Errorf("oobHandler unknown request %v: %v",
				address, cmd.Cmd)
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
				log.Errorf("oobHandler SendRequest %v: %v",
					address, err)
			}
		}

		if callback != nil {
			callback <- reply
		}
	}
}

func (p *PerfCtl) singleCommand(ctx context.Context, channel ssh.Channel, args []string) error {
	log.Tracef("singleCommand: args %v", args)
	defer func() {
		p.wg.Done()
		log.Tracef("singleCommand exit: args %v", args)
	}()

	if len(args) == 0 {
		return fmt.Errorf("impossible args length")
	}

	switch args[0] {
	case "status":
		_, err := p.sendAndWait(ctx, channel, types.PCCommand{
			Cmd: types.PCStatusCollectionCmd,
		})
		if err != nil {
			return err
		}

	case "start":
		_, err := p.sendAndWait(ctx, channel, types.PCCommand{
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
		_, err := p.sendAndWait(ctx, channel, types.PCCommand{
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
	_ = cancel

	for k, v := range p.cfg.HostsId {
		log.Infof("Connecting %v:%v/%v", v.Site, v.Host, k)

		channel, err := p.connect(ctx, k)
		if err != nil {
			log.Errorf("connect: %v", err)
			continue
		}

		go p.singleCommand(ctx, channel, args)
		p.wg.Add(1)
	}

	// Wait for exit
	log.Infof("Waiting for commands to complete")
	p.wg.Wait()

	return nil
}

func (pc *PerfCtl) connect(ctx context.Context, address string) (ssh.Channel, error) {
	log.Tracef("connect: %v", address)
	defer log.Tracef("connect exit: %v", address)

	pk, err := util.PublicKeyFile(pc.cfg.SSHKeyFile)
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

	pc.register(address, session{
		conn:    conn,
		channel: channel,
	})

	// Setup out of band handler.
	go pc.oobHandler(ctx, address, channel, requests)

	return channel, nil
}

func (pc *PerfCtl) journal(site, host, run uint64, measurement types.PCCollection) error {
	return fmt.Errorf("not yet")
}

func (pc *PerfCtl) sinkLoop(ctx context.Context, site, host uint64, address string) error {
	log.Tracef("sinkLoop %v:%v", site, host)
	defer log.Tracef("sinkLoop exit %v:%v", site, host)

	channel, err := pc.connect(ctx, address)
	if err != nil {
		log.Errorf("sendAndWait connect: %v", err)
		return err
	}

	defer func() {
		if err := pc.unregister(address); err != nil {
			log.Errorf("sink exit unregister: %v", err)
		}
	}()

	// Register sinkLoop.
	_, err = pc.sendAndWait(ctx, channel, types.PCCommand{
		Cmd: types.PCRegisterSink,
	})
	if err != nil {
		log.Errorf("sendAndWait connect: %v", err)
		return err
	}

	run := uint64(0)
	// We are in sinkLoop mode. Register sinkLoop and process measurements.
	dec := gob.NewDecoder(channel)
	for {
		var m types.PCCollection
		err := dec.Decode(&m)
		if err != nil {
			return fmt.Errorf("sinkLoop Decode %v:%v: %v",
				site, host, err)
		}

		if pc.cfg.Journal {
			err := pc.journal(site, host, run, m)
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

func (pc *PerfCtl) sink(ctx context.Context, site, host uint64, address string) {
	log.Tracef("sink %v:%v", site, host)

	defer func() {
		log.Tracef("sink exit %v:%v", site, host)
		pc.wg.Done()
	}()
	// Always reconnect unless canceled
	for {
		err := pc.sinkLoop(ctx, site, host, address)
		if err != nil {
			// This may be too loud
			log.Errorf("sink %v:%v: %v", site, host, err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
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

	pc := &PerfCtl{
		cfg:      loadedCfg,
		tags:     make(map[uint]chan interface{}),
		sessions: make(map[string]session),
	}

	log.Infof("Version         : %v", version())
	log.Infof("Home dir        : %v", pc.cfg.HomeDir)

	// Execute, this needs to come out
	if len(args) != 0 {
		return pc.handleArgs(args)
	}

	// Prepare database
	switch pc.cfg.DB {
	case "postgres":
		postgres.UseLogger(dbLog)
		pc.db, err = postgres.New(database.Name, pc.cfg.DBURI)
	default:
		return fmt.Errorf("Invalid database type: %v", pc.cfg.DB)
	}
	if err != nil {
		return err
	}

	// Open and Close db on exit.
	if err := pc.db.Open(); err != nil {
		return err
	}
	defer pc.db.Close()
	log.Infof("Database version: %v", database.Version)

	// Context.
	ctx, cancel := context.WithCancel(context.Background())

	for k, v := range pc.cfg.HostsId {
		log.Infof("Connecting %v:%v/%v", v.Site, v.Host, k)
		pc.wg.Add(1)
		go pc.sink(ctx, v.Site, v.Host, k)
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
			pc.unregisterAll()
			goto done
		}
	}
done:

	// Wait for exit
	log.Infof("Waiting to exit")
	pc.wg.Wait()

	return nil
}

func main() {
	err := _main()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
