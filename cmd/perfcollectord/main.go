package main

import (
	"bufio"
	"context"
	"encoding/gob"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/businessperformancetuning/perfcollector/types"
	"github.com/businessperformancetuning/perfcollector/util"
	"github.com/davecgh/go-spew/spew"
	"golang.org/x/crypto/ssh"
)

type PerfCollector struct {
	sync.Mutex
	newEncoder      chan *gob.Encoder             // New sink encoder
	newMeasurements chan chan *types.PCCollection // New measurements channel
	sinkRegistered  bool

	// Sink status channels
	sinkStatus  chan struct{}
	sinkStatusR chan *types.PCSinkStatus

	stopCollection    chan struct{} // collection stop channel
	collectionEnabled *types.PCStartCollection

	cfg *config

	allowedKeys map[string]struct{}
}

func protocolError(tag uint, format string, args ...interface{}) ([]byte, error) {
	return types.Encode(types.PCCommand{
		Tag: tag,
		Cmd: types.PCErrorCmd,
		Payload: types.PCError{
			Error: fmt.Sprintf(format, args...),
		},
	})
}

func (p *PerfCollector) setSinkRegistered(s bool) {
	p.Lock()
	p.sinkRegistered = s
	p.Unlock()
}

func (p *PerfCollector) getSinkRegistered() bool {
	p.Lock()
	defer p.Unlock()
	return p.sinkRegistered
}

func (p *PerfCollector) setCollectionEnabled(sc *types.PCStartCollection) {
	p.Lock()
	if sc == nil {
		p.collectionEnabled = nil
	} else {

		// Save copy
		s := *sc
		p.collectionEnabled = &s
	}
	p.Unlock()
}

func (p *PerfCollector) getCollectionEnabled() *types.PCStartCollection {
	p.Lock()
	defer p.Unlock()

	if p.collectionEnabled == nil {
		return nil
	}

	// Return copy
	sc := *p.collectionEnabled
	return &sc
}

func (p *PerfCollector) publicKeyCallback(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
	fp := ssh.FingerprintSHA256(key)
	log.Tracef("publicKeyCallback %v", fp)
	log.Tracef("publicKeyCallback %v exit", fp)

	if _, ok := p.allowedKeys[fp]; !ok {
		log.Errorf("Rejecting unknown key user %v address %v "+
			"fingerprint %v", conn.User(), conn.RemoteAddr(), fp)
		return nil, fmt.Errorf("unknown key: %v", fp)
	}

	return &ssh.Permissions{}, nil
}

func (p *PerfCollector) sink() {
	log.Tracef("sink")
	defer log.Tracef("sink exit")

	// This code is a bit hard to read but the idea is that we only allow
	// one stream sink and when the sink goes away we wait for a new
	// encoder to show up. When the new encoder arrives we flush all
	// existing measurements.
	var (
		encoder      *gob.Encoder
		measurements chan *types.PCCollection
	)

	// sinkStatus is a closure that signals the sink status.
	sinkStatus := func() {
		currentDepth := 0
		if measurements != nil {
			currentDepth = len(measurements)
		}
		p.sinkStatusR <- &types.PCSinkStatus{
			QueueUsed:          currentDepth,
			SinkEnabled:        encoder != nil,
			MeasurementEnabled: measurements != nil,
		}

	}

	newEncoder := func(e *gob.Encoder) {
		encoder = e
		p.setSinkRegistered(e != nil)
	}

	for {
		select {
		case e, ok := <-p.newEncoder:
			if !ok {
				return
			}
			newEncoder(e)
			continue

		case mc, ok := <-p.newMeasurements:
			if !ok {
				return
			}
			measurements = mc
			continue

		case _, ok := <-p.sinkStatus:
			if !ok {
				return
			}
			sinkStatus()

		case m, ok := <-measurements:
			if !ok {
				return
			}

			// Loop in order to no lose measurement.
			for {
				// If there is no encoder wait for a new one to
				// appear.
				if encoder == nil {
					p.setSinkRegistered(false)
					select {
					case e, ok := <-p.newEncoder:
						if !ok {
							return
						}
						newEncoder(e)
					case mc, ok := <-p.newMeasurements:
						if !ok {
							return
						}
						measurements = mc
						continue
					case _, ok := <-p.sinkStatus:
						if !ok {
							return
						}
						sinkStatus()
						continue
					}
				}

				// Send measurement to sream.
				err := encoder.Encode(*m)
				if err != nil {
					// Wait for new encoder
					encoder = nil
					continue
				}

				// Drain measurements
				select {
				case m, ok = <-measurements:
				default:
					goto done
				}
			}
		done:
		}
	}
}

func (p *PerfCollector) handleChannels(ctx context.Context, conn *ssh.ServerConn, chans <-chan ssh.NewChannel) {
	log.Tracef("handleChannels")
	defer log.Tracef("handleChannels exit")

	for newChannel := range chans {
		log.Tracef("handleChannels: %v", newChannel.ChannelType())
		go p.handleChannel(ctx, conn, newChannel)
	}
}

func (p *PerfCollector) handleRegisterSink(cmd types.PCCommand, channel ssh.Channel) ([]byte, error) {
	log.Tracef("handleRegisterSink %v", cmd.Tag)
	defer log.Tracef("handleRegisterSink %v exit", cmd.Tag)

	// Register sink
	if p.getSinkRegistered() {
		return protocolError(cmd.Tag, "sink already registered")
	}
	select {
	case p.newEncoder <- gob.NewEncoder(channel):
	default:
		panic("shouldn't happen")
	}

	reply := types.PCCommand{
		Version: types.PCVersion,
		Tag:     cmd.Tag,
		Cmd:     types.PCAck,
	}

	return types.Encode(reply)
}

func (p *PerfCollector) handleOnce(cmd types.PCCommand) ([]byte, error) {
	log.Tracef("handleOnce %v", cmd.Cmd)
	defer log.Tracef("handleOnce %v exit", cmd.Cmd)

	co, ok := cmd.Payload.(types.PCCollectOnce)
	if !ok {
		// Should not happen
		return nil, fmt.Errorf("handleOnce: type assertion error %T",
			co)
	}

	payload := types.PCCollectOnceReply{
		Values: make([][]byte, len(co.Systems)),
	}
	var err error
	for k, v := range co.Systems {
		log.Tracef("handleOnce: %v", v)
		payload.Values[k], err = util.Measure(v)
		if err != nil {
			log.Errorf("handleOnce ReadFile: %v", err)
			return protocolError(cmd.Tag, "invalid system: %v", v)
		}
	}

	reply := types.PCCommand{
		Version: types.PCVersion,
		Tag:     cmd.Tag,
		Cmd:     types.PCCollectOnceReplyCmd,
		Payload: payload,
	}

	return types.Encode(reply)
}

func (p *PerfCollector) startCollection(ctx context.Context, sc types.PCStartCollection) {
	log.Tracef("startCollection %v", sc.Frequency)
	defer log.Tracef("startCollection %v exit", sc.Frequency)

	// Message new measurements channel
	measurements := make(chan *types.PCCollection, sc.QueueDepth)
	select {
	case p.newMeasurements <- measurements:
	default:
		panic("should not happen")
	}

	p.setCollectionEnabled(&sc)

	t := time.Tick(sc.Frequency) // XXX Replace this with an elapsed time counter
	for {
		select {
		case <-t:
			//case <-ctx.Done():
			//	return
		case <-p.stopCollection:
			return
		}

		var err error
		timestamp := time.Now()
		for _, v := range sc.Systems {
			m := types.PCCollection{
				System:    v,
				Timestamp: timestamp,  // Overall timestamp
				Start:     time.Now(), // This timestamp
			}

			m.Measurement, err = util.Measure(v)
			if err != nil {
				log.Errorf("startCollection: %v", err)
				// Abort measurement.
				continue
			}

			m.Duration = time.Now().Sub(m.Timestamp)

			// Spill last measurement if queue depth is reached
			select {
			case measurements <- &m:
				log.Tracef("startCollection: recording %v", v)

			default:
				log.Tracef("startCollection: spill %v",
					len(measurements))
			}
		}
	}
}

func (p *PerfCollector) handleStatusCollection(ctx context.Context, cmd types.PCCommand, channel ssh.Channel) ([]byte, error) {
	log.Tracef("handleStatusCollection %v", cmd.Cmd)
	defer log.Tracef("handleStatusCollection %v exit", cmd.Cmd)

	if cmd.Payload != nil {
		return protocolError(cmd.Tag, "invalid status collector payload")
	}

	sc := p.getCollectionEnabled()
	if sc == nil {
		// No collection running.
		sc = &types.PCStartCollection{}
	}

	select {
	case p.sinkStatus <- struct{}{}:
	default:
		panic("shouldn't happen")
	}
	sinkStatus := <-p.sinkStatusR

	reply := types.PCCommand{
		Version: types.PCVersion,
		Tag:     cmd.Tag,
		Cmd:     types.PCStatusCollectionCmd,
		Payload: types.PCStatusCollectionReply{
			Frequency:  sc.Frequency,
			Systems:    sc.Systems,
			QueueDepth: sc.QueueDepth,
			SinkStatus: *sinkStatus,
		},
	}
	return types.Encode(reply)
}

func (p *PerfCollector) handleStartCollection(ctx context.Context, cmd types.PCCommand, channel ssh.Channel) ([]byte, error) {
	log.Tracef("handleStartCollection %v", cmd.Cmd)
	defer log.Tracef("handleStartCollection %v exit", cmd.Cmd)

	sc, ok := cmd.Payload.(types.PCStartCollection)
	if !ok {
		return protocolError(cmd.Tag, "command type "+
			"assertion error %v, %T", cmd.Cmd, sc)
	}

	// Verify frequency.
	if sc.Frequency < time.Second {
		return protocolError(cmd.Tag, "bad frequency")
	}

	// Verify that all systems exist.
	for _, v := range sc.Systems {
		if util.FileExists(v) {
			continue
		}
		return protocolError(cmd.Tag, "invalid system %v", v)
	}

	// Only allow one collection to run.
	if p.getCollectionEnabled() != nil {
		return protocolError(cmd.Tag, "collector already running")
	}

	p.stopCollection = make(chan struct{})
	go p.startCollection(ctx, sc)

	// Ack remote.
	reply := types.PCCommand{
		Version: types.PCVersion,
		Tag:     cmd.Tag,
		Cmd:     types.PCAck,
	}
	return types.Encode(reply)
}

func (p *PerfCollector) handleStopCollection(ctx context.Context, cmd types.PCCommand, channel ssh.Channel) ([]byte, error) {
	log.Tracef("handleStopCollection %v", cmd.Cmd)
	defer log.Tracef("handleStopCollection %v exit", cmd.Cmd)

	if cmd.Payload != nil {
		return protocolError(cmd.Tag, "invalid stop collector payload")
	}

	// Return error if collection isn't running.
	if p.getCollectionEnabled() == nil {
		return protocolError(cmd.Tag, "collector not running")
	}

	select {
	case p.newMeasurements <- nil:
	default:
		panic("should not happen")
	}
	close(p.stopCollection)
	p.setCollectionEnabled(nil)

	// Ack remote.
	reply := types.PCCommand{
		Version: types.PCVersion,
		Tag:     cmd.Tag,
		Cmd:     types.PCAck,
	}
	return types.Encode(reply)
}

func (p *PerfCollector) oobHandler(ctx context.Context, channel ssh.Channel, requests <-chan *ssh.Request) {
	log.Tracef("oobHandler")

	ctx, cancel := context.WithCancel(ctx)

	defer func() {
		cancel()
		log.Tracef("oobHandler exit")
	}()

	for req := range requests {
		// Always reply or else the other end may hang.
		req.Reply(true, nil)
		log.Tracef("oobHandler loop")

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

		log.Tracef("oobHandler %v", spew.Sdump(cmd))

		// From here on out we must ack every command incoming command.
		// Replies do not need to be acked.

		var (
			reply []byte
		)
		switch cmd.Cmd {
		// COmmands that don't require ack.
		case types.PCErrorCmd:
			// Log error and move on.
			e, ok := cmd.Payload.(types.PCError)
			if !ok {
				// Should not happen
				log.Errorf("oobHandler command type assertion "+
					"error: %T", cmd.Payload)
				continue
			}
			log.Errorf("oobHandler remote error: version %v tag %v"+
				" cmd %v error %v", cmd.Version, cmd.Tag,
				cmd.Cmd, e.Error)
			continue

			// Commands that require ack
		case types.PCRegisterSink:
			reply, err = p.handleRegisterSink(cmd, channel)

		case types.PCCollectOnceCmd:
			reply, err = p.handleOnce(cmd)

		case types.PCStatusCollectionCmd:
			reply, err = p.handleStatusCollection(ctx, cmd, channel)

		case types.PCStartCollectionCmd:
			reply, err = p.handleStartCollection(ctx, cmd, channel)

		case types.PCStopCollectionCmd:
			reply, err = p.handleStopCollection(ctx, cmd, channel)

		default:
			reply, err = protocolError(cmd.Tag, "unknown OOB "+
				"command: %v", cmd.Cmd)
		}

		// Deal with internal errors
		if err != nil {
			t := time.Now()
			log.Errorf("oobHandler internal error cmd %v tag %v "+
				"timestamp %v: %v",
				cmd.Cmd, cmd.Tag, t.Unix(), err)
			log.Debugf("oobHandler internal error command: %v",
				spew.Sdump(cmd))
			reply, err = protocolError(cmd.Tag, "internal "+
				"error: %v", strconv.Itoa(int(t.Unix())))
			if err != nil {
				log.Errorf("oobHandler encode: %v", err)
			}
		}

		// Send payload to server.
		if reply == nil {
			// Nothing to do
			continue
		}
		_, err = channel.SendRequest(types.PCCmd, false, reply)
		if err != nil {
			log.Errorf("oobHandler SendRequest: %v", err)
		}
	}
}

func (p *PerfCollector) handleChannel(ctx context.Context, conn *ssh.ServerConn, newChannel ssh.NewChannel) {
	log.Tracef("handleChannel")
	defer log.Tracef("handleChannel exit")

	if t := newChannel.ChannelType(); t != types.PCChannel {
		_ = newChannel.Reject(ssh.UnknownChannelType,
			fmt.Sprintf("unknown channel type: %s", t))
		return
	}

	channel, requests, err := newChannel.Accept()
	if err != nil {
		log.Errorf("could not accept channel (%s)", err)
		return
	}
	defer channel.Close()

	go p.oobHandler(ctx, channel, requests)

	// XXX if a sink was registered in this session we need to signal the
	// sink function that the encoder has become nil.
	// This bug happens when you rehister a sink. Close it. Register again.
	// This will result in a sink already registered error.
	for {
		r := bufio.NewReader(channel)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				log.Tracef("handleChannel read error: %v", err)
				return
			}
			log.Infof(line)
		}
	}
}

func (p *PerfCollector) sshServe(listen string, signer ssh.Signer) error {
	log.Tracef("sshServe %v", listen)
	defer log.Tracef("sshServe %v exit", listen)

	// Setup SSH listener.
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}

	// Setup SSH config.
	sshConfig := &ssh.ServerConfig{
		PublicKeyCallback: p.publicKeyCallback,
	}
	sshConfig.AddHostKey(signer)

	ctx := context.Background()

	log.Infof("Listen: %v", listen)
	for {
		tcpConn, err := listener.Accept()
		if err != nil {
			return err
		}

		sshConn, chans, reqs, err := ssh.NewServerConn(tcpConn, sshConfig)
		if err != nil {
			// Don't exit on handshake failure
			log.Errorf("sshServe handshake failed (%s)", err)
			continue
		}

		go ssh.DiscardRequests(reqs)
		go p.handleChannels(ctx, sshConn, chans)
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

	pc := &PerfCollector{
		cfg:             loadedCfg,
		allowedKeys:     make(map[string]struct{}),
		newEncoder:      make(chan *gob.Encoder),
		newMeasurements: make(chan chan *types.PCCollection),
		sinkStatus:      make(chan struct{}),
		sinkStatusR:     make(chan *types.PCSinkStatus),
	}
	for _, v := range pc.cfg.AllowedKeys {
		pc.allowedKeys[v] = struct{}{}
	}

	log.Infof("Version      : %v", version())
	log.Infof("Home dir     : %v", pc.cfg.HomeDir)

	// Create the data directory in case it does not exist.
	err = os.MkdirAll(loadedCfg.DataDir, 0700)
	if err != nil {
		return err
	}

	// SSH key.
	signer, err := util.SSHKey(loadedCfg.SSHKeyFile)
	if err != nil {
		return err
	}

	// Prepare sink
	go pc.sink()

	// Listen for incoming SSH connections.
	listenC := make(chan error)
	for _, listener := range loadedCfg.Listeners {
		listen := listener
		go func() {
			listenC <- pc.sshServe(listen, signer)
		}()
	}

	// Tell user we are ready to go.
	log.Infof("Start of day")

	// Setup OS signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGINT)
	for {
		select {
		case sig := <-sigs:
			log.Infof("Terminating with %v", sig)
			goto done
		case err := <-listenC:
			log.Errorf("%v", err)
			goto done
		}
	}
done:
	log.Infof("Waiting on subsystems to shut down")

	log.Infof("Exiting")

	return nil
}

func main() {
	err := _main()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
