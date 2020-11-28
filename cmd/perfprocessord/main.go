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
	"strings"
	"sync"
	"syscall"
	"time"

	ch "github.com/businessperformancetuning/perfcollector/channel"
	"github.com/businessperformancetuning/perfcollector/cmd/perfprocessord/journal"
	"github.com/businessperformancetuning/perfcollector/database"
	"github.com/businessperformancetuning/perfcollector/database/postgres"
	"github.com/businessperformancetuning/perfcollector/parser"
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
	sync.RWMutex
	tag  uint                      // Last used tag
	tags map[uint]chan interface{} // Tag callback

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
	sessions map[string]*session // Mutex is only for map insert/delete.

	cfg *config

	db database.Database
}

func (p *PerfCtl) send(s *session, cmd types.PCCommand, callback chan interface{}) error {
	log.Tracef("send %v %v", cmd.Cmd, s.address)
	defer log.Tracef("send exit %v %v", cmd.Cmd, s.address)

	// Hnadle tag
	s.Lock()
	tag := s.tag
	if _, ok := s.tags[tag]; ok {
		p.Unlock()
		return fmt.Errorf("duplicate tag: %v", tag)
	}
	s.tags[tag] = callback
	s.tag++
	s.Unlock()

	// Send OOB
	cmd.Version = types.PCVersion // Set version
	cmd.Tag = tag                 // Set tag

	// Do expensive encode first
	blob, err := types.Encode(cmd)
	if err != nil {
		return err
	}

	log.Tracef("send %v: %v", s.address, spew.Sdump(cmd))

	_, err = s.channel.SendRequest(types.PCCmd, false, blob)

	return err
}

func (p *PerfCtl) sendAndWait(ctx context.Context, s *session, cmd types.PCCommand) (interface{}, error) {
	log.Tracef("sendAndWait %v", s.address)
	defer log.Tracef("sendAndWait exit %v", s.address)

	// Callback channel
	c := make(chan interface{})

	err := p.send(s, cmd, c)
	if err != nil {
		return nil, err
	}

	reply, readErr := ch.Read(ctx, c)
	if readErr != nil {
		panic(readErr)
		return nil, readErr
	}

	// See if we got a remote error back.
	if r, ok := reply.(error); ok {
		return nil, r
	}

	return reply, nil
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
		s.Lock()
		callback, ok := s.tags[cmd.Tag]
		if !ok {
			s.Unlock()
			log.Errorf("oobHandler unknown tag %v: %v",
				s.address, cmd.Tag)
			continue
		}
		delete(s.tags, cmd.Tag)
		s.Unlock()

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

		case types.PCCollectDirectoriesReplyCmd:
			d, ok := cmd.Payload.(types.PCCollectDirectoriesReply)
			if ok {
				reply = d
			} else {
				// Should not happen
				log.Errorf("type assertion error %v: %T",
					s.address, cmd.Payload)
			}

		case types.PCStatusCollectionReplyCmd:
			status, ok := cmd.Payload.(types.PCStatusCollectionReply)
			if ok {
				reply = status
			} else {
				// Should not happen
				log.Errorf("type assertion error %v: %T",
					s.address, cmd.Payload)
			}

		case types.PCStartReplayReplyCmd:
			rr, ok := cmd.Payload.(types.PCStartReplayReply)
			if ok {
				reply = rr
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
			err = p.send(s, reply, nil)
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
		tags:     make(map[uint]chan interface{}),
	}
	p.register(address, session)

	return session, nil
}

func (p *PerfCtl) handleNetCache(ctx context.Context, s *session, h HostIdentifier, run uint64) error {
	log.Tracef("handleNetCache %v", s.address)
	defer log.Tracef("handleNetCache exit %v", s.address)

	// Figure out NICs that exist on remote systems.
	reply, err := p.sendAndWait(ctx, s, types.PCCommand{
		Cmd: types.PCCollectDirectoriesCmd,
		Payload: types.PCCollectDirectories{
			Directories: []string{"/sys/class/net/"},
		},
	})
	if err != nil {
		return err
	}
	dirsReply, ok := reply.(types.PCCollectDirectoriesReply)
	if !ok {
		return fmt.Errorf("directories reply invalid type: %T",
			reply)
	}

	// Remove localhost
	nics := make([]string, 0, len(dirsReply.Values[0]))
	for _, v := range dirsReply.Values[0] {
		if v == "lo" {
			continue
		}
		nics = append(nics, v)
	}

	// Obtain speed and duplex for all NICs.
	nicsReply, err := p.getNetDevices(ctx, s, nics)
	if err != nil {
		return err
	}

	// Create cache
	nicCache := make(map[string]parser.NIC)
	for k := range nics {
		nicCache[nics[k]] = nicsReply[k]
	}

	// Construct JSON to fill the NIC cache.
	ts := time.Now()
	for k, v := range nicCache {
		// Duplex
		wc := journal.WrapPCCollection{
			Site: h.Site,
			Host: h.Host,
			Run:  run,
			Measurement: &types.PCCollection{
				Timestamp: ts,
				System: fmt.Sprintf("/sys/class/net/%v/duplex",
					k),
				Measurement: v.Duplex + "\n",
			},
		}
		b, err := json.Marshal(wc)
		if err != nil {
			return err
		}
		fmt.Printf("%v\n", string(b))

		// Reuse wc for speed
		wc.Measurement.System = fmt.Sprintf("/sys/class/net/%v/speed", k)
		wc.Measurement.Measurement = strconv.FormatUint(v.Speed, 10) +
			"\n"
		b, err = json.Marshal(wc)
		if err != nil {
			return err
		}
		fmt.Printf("%v\n", string(b))
	}

	return nil
}

func (p *PerfCtl) singleCommand(ctx context.Context, s *session, h HostIdentifier, args []string) error {
	log.Tracef("singleCommand %v: args %v", s.address, args)
	defer func() {
		log.Tracef("singleCommand exit %v: args %v", s.address, args)
	}()

	if len(args) == 0 {
		return fmt.Errorf("impossible args length")
	}

	// Parse arguments
	a, err := util.ParseArgs(args)
	if err != nil {
		return err
	}

	switch args[0] {
	case "status":
		reply, err := p.sendAndWait(ctx, s, types.PCCommand{
			Cmd: types.PCStatusCollectionCmd,
		})
		if err != nil {
			return err
		}
		r, ok := reply.(types.PCStatusCollectionReply)
		if !ok {
			return fmt.Errorf("status reply invalid type: %T", reply)
		}
		fmt.Printf("Status             : %v\n", s.address)
		fmt.Printf("Sink enabled       : %v\n", r.SinkEnabled)
		fmt.Printf("Measurement enabled: %v\n", r.MeasurementEnabled)
		if r.MeasurementEnabled && r.StartCollection != nil {
			fmt.Printf("Frequency          : %v\n",
				r.StartCollection.Frequency)
			fmt.Printf("Queue depth        : %v\n",
				r.StartCollection.QueueDepth)
			fmt.Printf("Queue free         : %v\n",
				r.QueueFree)
			fmt.Printf("Systems            : %v\n",
				r.StartCollection.Systems)
		}

	case "start":
		frequency, err := util.ArgAsInt("frequency", a)
		if err != nil {
			frequency = 5
		}
		queueDepth, err := util.ArgAsInt("depth", a)
		if err != nil {
			queueDepth = 1000
		}
		systems, err := util.ArgAsStringSlice("systems", a)
		if err != nil {
			systems = []string{
				"/proc/stat",
				"/proc/meminfo",
				"/proc/net/dev",
				"/proc/diskstats",
			}
		}
		_, err = p.sendAndWait(ctx, s, types.PCCommand{
			Cmd: types.PCStartCollectionCmd,
			Payload: types.PCStartCollection{
				Frequency:  time.Duration(frequency) * time.Second,
				QueueDepth: queueDepth,
				Systems:    systems,
			},
		})
		if err != nil {
			return err
		}

	case "stop":
		_, err := p.sendAndWait(ctx, s, types.PCCommand{
			Cmd: types.PCStopCollectionCmd,
		})
		if err != nil {
			return err
		}

	case "replay":
		filename, err := util.ArgAsString("filename", a)
		if err != nil {
			return err
		}
		_ = filename
		_, err = p.sendAndWait(ctx, s, types.PCCommand{
			Cmd: types.PCStartReplayCmd,
			Payload: types.PCStartReplay{
				Frequency: 5 * time.Second,
			},
		})
		if err != nil {
			return err
		}

	case "once":
		systems, err := util.ArgAsStringSlice("systems", a)
		if err != nil {
			systems = []string{
				"/proc/cpuinfo",
				"/proc/uptime",
				"/proc/version",
			}
		}
		err = util.HasTrailingSlashes(systems)
		if err != nil {
			return err
		}
		reply, err := p.sendAndWait(ctx, s, types.PCCommand{
			Cmd: types.PCCollectOnceCmd,
			Payload: types.PCCollectOnce{
				Systems: systems,
			},
		})
		if err != nil {
			return err
		}
		onceReply, ok := reply.(types.PCCollectOnceReply)
		if !ok {
			return fmt.Errorf("once reply invalid type: %T", reply)
		}
		for k := range onceReply.Values {
			fmt.Printf("System: %v\n", systems[k])
			fmt.Printf("%v", string(onceReply.Values[k]))
			if k < len(onceReply.Values)-1 {
				fmt.Printf("\n")
			}
		}

	case "dir":
		// XXX disallow anything outside of /proc and /sys.
		if len(a) == 1 {
			return fmt.Errorf("must provide directories")
		}
		directories, err := util.ArgAsStringSlice("directories", a)
		if err != nil {
			return err
		}
		err = util.DoesNotHaveTrailingSlashes(directories)
		if err != nil {
			return err
		}
		reply, err := p.sendAndWait(ctx, s, types.PCCommand{
			Cmd: types.PCCollectDirectoriesCmd,
			Payload: types.PCCollectDirectories{
				Directories: directories,
			},
		})
		if err != nil {
			return err
		}
		dirsReply, ok := reply.(types.PCCollectDirectoriesReply)
		spew.Dump(dirsReply)
		if !ok {
			return fmt.Errorf("directories reply invalid type: %T",
				reply)
		}
		for k := range dirsReply.Values {
			fmt.Printf("Directory: %v\n", directories[k])
			for i := range dirsReply.Values[k] {
				fmt.Printf("\t%v\n", dirsReply.Values[k][i])
			}
			if k < len(dirsReply.Values)-1 {
				fmt.Printf("\n")
			}
		}

	case "netcache":
		run, err := util.ArgAsInt("run", a)
		if err != nil {
			run = 0
		}
		return p.handleNetCache(ctx, s, h, uint64(run))

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
	case "once":
	case "dir":
	case "netcache":
	case "replay":
	default:
		return fmt.Errorf("unknown command: %v", args[0])
	}

	// Context.
	ctx, cancel := context.WithCancel(context.Background())

	var eg errgroup.Group
	for k := range p.cfg.HostsId {
		// Copy hosts because it'll race in the go routine.
		h := p.cfg.HostsId[k]
		hh := k
		log.Debugf("Connecting %v:%v/%v", h.Site, h.Host, hh)

		session, err := p.connect(ctx, hh)
		if err != nil {
			log.Errorf("connect: %v", err)
			continue
		}

		log.Debugf("Connected to: %v:%v/%v", h.Site, h.Host, hh)

		// Setup out of band handler.
		eg.Go(func() error {
			err := p.oobHandler(session)
			if err != nil {
				if err != io.EOF {
					log.Errorf("handleArgs oobHandler: %v",
						err)
				}
			}
			return err
		})

		eg.Go(func() error {
			err := p.singleCommand(ctx, session, h,
				args)
			if err != nil {
				log.Errorf("handleArgs singleCommand: %v", err)
			}
			session.channel.Close()
			return err
		})
	}

	// Wait for exit
	log.Debugf("Waiting for commands to complete")
	eg.Wait()

	// Shut...It...All...Down...
	cancel()

	return nil
}

func (p *PerfCtl) getNetDevices(ctx context.Context, s *session, devices []string) ([]parser.NIC, error) {
	systems := make([]string, 0, len(devices)*2)
	for k := range devices {
		systems = append(systems, filepath.Join("/sys", "class", "net",
			devices[k], "duplex"))
		systems = append(systems, filepath.Join("/sys", "class", "net",
			devices[k], "speed"))
	}

	reply, err := p.sendAndWait(ctx, s, types.PCCommand{
		Cmd: types.PCCollectOnceCmd,
		Payload: types.PCCollectOnce{
			Systems: systems,
		},
	})
	if err != nil {
		return nil, err
	}
	r, ok := reply.(types.PCCollectOnceReply)
	if !ok {
		return nil, fmt.Errorf("invalid reply type: %T", reply)
	}

	// Make sure we got enough responses
	if len(r.Values) != len(devices)*2 {
		return nil, fmt.Errorf("invalid number of responses: %v %v",
			len(r.Values), len(devices)*2)
	}

	nics := make([]parser.NIC, 0, len(devices)*2)
	for i := 0; i < len(r.Values); i += 2 {
		var duplex string
		switch strings.TrimSpace(string(r.Values[i])) {
		case "half":
			duplex = "half"
		case "full":
			duplex = "full"
		default:
			return nil, fmt.Errorf("Invalid duplex: '%v'",
				string(r.Values[i]))
		}

		speed, err := strconv.ParseUint(strings.TrimSpace(string(r.Values[i+1])),
			10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid speed: %v", err)
		}

		nics = append(nics, parser.NIC{
			Duplex: duplex,
			Speed:  speed,
		})
	}

	return nics, nil
}

func (p *PerfCtl) journal(site, host, run uint64, measurement types.PCCollection) error {
	if !util.ValidSystem(measurement.System) {
		return fmt.Errorf("journal unsupported system: %v",
			measurement.System)
	}

	// We only allow encrypted journals.
	if true {
		return journal.Journal(p.cfg.journalFilename, p.cfg.aead,
			journal.WrapPCCollection{
				Site:        site,
				Host:        host,
				Run:         run,
				Measurement: &measurement,
			})
	}

	// This code cannot be reached, compile time debug only to journal in
	// plaintext.
	filename := filepath.Join(p.cfg.DataDir, strconv.Itoa(int(site)),
		strconv.Itoa(int(host)), strconv.Itoa(int(run)),
		measurement.System)
	dir := filepath.Dir(filename)
	err := os.MkdirAll(dir, 0750)
	if err != nil {
		return err
	}

	// Journal in JSON to retain human readability.
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0640)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(measurement)
}

func (p *PerfCtl) sinkLoop(ctx context.Context, site, host uint64, address string) error {
	log.Tracef("sinkLoop %v:%v", site, host)
	defer log.Tracef("sinkLoop exit %v:%v", site, host)

	s, err := p.connect(ctx, address)
	if err != nil {
		log.Errorf("sinkLoop connect %v:%v: %v", site, host, err)
		return err
	}

	log.Infof("Connected to: %v:%v/%v", site, host, address)

	defer func() {
		if err := p.unregister(address); err != nil {
			log.Errorf("sink exit unregister %v:%v: %v",
				site, host, err)
		}
	}()

	// Setup out of band handler.
	go p.oobHandler(s)

	// Register sinkLoop.
	_, err = p.sendAndWait(ctx, s, types.PCCommand{
		Cmd: types.PCRegisterSinkCmd,
	})
	if err != nil {
		log.Errorf("sinkLoop sendAndWait connect %v:%v: %v",
			site, host, err)
		return terminalError{err: err}
	}

	// Create nicCache that holds duplex and speed.
	// XXX this is incorrect; this overwrites nics from different hosts
	nicCache := make(map[string]parser.NIC)
	cacheFilled := false
	fillCache := func(n parser.NetDev) error {
		if cacheFilled {
			return nil
		}
		nics := make([]string, 0, len(n))
		for k := range n {
			if _, ok := nicCache[k]; ok {
				continue
			}
			if k == "lo" {
				// lo is invalid so insert zero value
				nicCache[k] = parser.NIC{}
				continue
			}
			nics = append(nics, k)
		}

		// Get specifics
		reply, err := p.getNetDevices(ctx, s, nics)
		if err != nil {
			return err
		}

		// Cache values
		for k := range reply {
			nicCache[nics[k]] = reply[k]
		}
		cacheFilled = true

		return nil
	}

	runID := uint64(0)
	var (
		previousStat *parser.Stat
		previousNet  parser.NetDev
		previousDisk []parser.Diskstats
	)
	// We are in sinkLoop mode. Register sinkLoop and process measurements.
	dec := gob.NewDecoder(s.channel)
	for {
		var m types.PCCollection
		err := dec.Decode(&m)
		if err != nil {
			return fmt.Errorf("sinkLoop Decode %v:%v: %v",
				site, host, err)
		}

		// XXX consider reading more than one measurement at a time and
		// batch the writes.

		if p.cfg.Journal {
			log.Tracef("sinkLoop journal %v:%v: %v",
				site, host, m.System)
			err := p.journal(site, host, runID, m)
			if err != nil {
				return fmt.Errorf("sinkLoop journal %v:%v: %v",
					site, host, err)
			}
			continue
		}

		// Post process
		switch m.System {
		case "/proc/stat":
			s, err := parser.ProcessStat([]byte(m.Measurement))
			if err != nil {
				log.Errorf("sinkLoop could not process stat "+
					"%v:%v: %v", site, host, err)
				continue
			}
			if previousStat == nil {
				previousStat = &s
				continue
			}
			cs, err := parser.CubeStat(runID, m.Timestamp.Unix(),
				m.Start.Unix(), int64(m.Duration), previousStat,
				&s)
			if err != nil {
				log.Errorf("sinkLoop CubeStat %v:%v: %v",
					site, host, err)
				continue
			}
			previousStat = &s

			err = p.db.StatInsert(ctx, cs)
			if err != nil {
				log.Errorf("sinkLoop CubeStat insert %v:%v: %v",
					site, host, err)
			}
			continue

		case "/proc/meminfo":
			s, err := parser.ProcessMeminfo([]byte(m.Measurement))
			if err != nil {
				log.Errorf("sinkLoop could not process "+
					"meminfo %v:%v: %v", site, host, err)
				continue
			}
			mi, err := parser.CubeMeminfo(runID, m.Timestamp.Unix(),
				m.Start.Unix(), int64(m.Duration), &s)
			if err != nil {
				log.Errorf("sinkLoop CubeMeminfo %v:%v: %v",
					site, host, err)
				continue
			}

			err = p.db.MeminfoInsert(ctx, mi)
			if err != nil {
				log.Errorf("sinkLoop MeminfoInsert insert "+
					"%v:%v: %v", site, host, err)
			}
			continue

		case "/proc/net/dev":
			n, err := parser.ProcessNetDev([]byte(m.Measurement))
			if err != nil {
				log.Errorf("sinkLoop could not process netdev "+
					"%v:%v: %v", site, host, err)
				continue
			}

			// See if we need to cache NIC details
			err = fillCache(n)
			if err != nil {
				log.Errorf("sinkLoop could not process "+
					"fillCache %v:%v: %v", site, host, err)
				continue
			}

			if previousNet == nil {
				previousNet = n
				continue
			}
			tvi := uint64(m.Frequency.Seconds()) * parser.UserHZ
			nd, err := parser.CubeNetDev(site, host, runID, m.Timestamp.Unix(),
				m.Start.Unix(), int64(m.Duration),
				previousNet, n, tvi, nicCache)
			if err != nil {
				log.Errorf("sigkLoop CubeNetDev %v:%v: %v",
					site, host, err)
				continue
			}
			previousNet = n

			err = p.db.NetDevInsert(ctx, nd)
			if err != nil {
				log.Errorf("sinkLoop NetDevInsert insert "+
					"%v:%v: %v", site, host, err)
			}
			continue

		case "/proc/diskstats":
			d, err := parser.ProcessDiskstats([]byte(m.Measurement))
			if err != nil {
				log.Errorf("sinkLoop could not process "+
					"diskstats %v:%v: %v", site, host, err)
				continue
			}
			if previousDisk == nil {
				previousDisk = d
				continue
			}
			tvi := uint64(m.Frequency.Seconds()) * parser.UserHZ
			ds, err := parser.CubeDiskstats(runID,
				m.Timestamp.Unix(), m.Start.Unix(),
				int64(m.Duration), previousDisk, d, tvi)
			if err != nil {
				log.Errorf("sigkLoop CubeDiskstats %v:%v: %v",
					site, host, err)
				continue
			}
			previousDisk = d

			err = p.db.DiskstatInsert(ctx, ds)
			if err != nil {
				log.Errorf("sinkLoop DiskstatInsert insert "+
					"%v:%v: %v", site, host, err)
			}
			continue

		default:
			log.Errorf("unknown system %v:%v: %v",
				site, host, m.System)
		}
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
		case <-time.After(5 * time.Second): // XXX this should be 30 or so seconds
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
		sessions: make(map[string]*session),
	}

	// Execute, this needs to come out
	if len(args) != 0 {
		return p.handleArgs(args)
	}

	// Prepare database
	switch p.cfg.DB {
	case "postgres":
		postgres.UseLogger(dbLog)
		p.db, err = postgres.New(database.Name, p.cfg.DBURI)
	case "":
		// Allow no db if we are journaling.
		if !p.cfg.Journal {
			return fmt.Errorf("Must specify data recording method" +
				" selected (journal and/or database")
		}
	default:
		return fmt.Errorf("Invalid database type: %v", p.cfg.DB)
	}
	if err != nil {
		return err
	}

	log.Infof("Version: %v", version())
	log.Infof("Home dir: %v", p.cfg.HomeDir)
	log.Infof("SSH fingerprint: %v", p.cfg.fingerprint)

	if p.db != nil {
		// Open and Close db on exit.
		if err := p.db.Open(); err != nil {
			return err
		}
		defer p.db.Close()
		log.Infof("Database version: %v", database.Version)
	}

	if p.cfg.Journal {
		log.Infof("Journal: %v", p.cfg.journalFilename)
	}

	// Context.
	ctx, cancel := context.WithCancel(context.Background())

	var eg errgroup.Group
	for k := range p.cfg.HostsId {
		// Copy hosts because it'll race in the go routine.
		h := p.cfg.HostsId[k]
		hh := k
		log.Infof("Connecting %v:%v/%v", h.Site, h.Host, hh)
		eg.Go(func() error {
			err := p.sink(ctx, h.Site, h.Host, hh)
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
