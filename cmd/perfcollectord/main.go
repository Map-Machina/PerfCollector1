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

	ch "github.com/businessperformancetuning/perfcollector/channel"
	"github.com/businessperformancetuning/perfcollector/types"
	"github.com/businessperformancetuning/perfcollector/util"
	"github.com/davecgh/go-spew/spew"
	"golang.org/x/crypto/ssh"
)

// protocolError returns an encoded protocol error.
func protocolError(tag uint, format string, args ...interface{}) ([]byte, error) {
	return types.Encode(types.PCCommand{
		Tag: tag,
		Cmd: types.PCErrorCmd,
		Payload: types.PCError{
			Error: fmt.Sprintf(format, args...),
		},
	})
}

// internalError returns an encoded internal error.
func internalError(cmd types.PCCommand, err error) ([]byte, error) {
	t := time.Now()
	log.Errorf("internal error cmd %v tag %v timestamp %v: %v",
		cmd.Cmd, cmd.Tag, t.Unix(), err)
	log.Debugf("internal error command: %v", spew.Sdump(cmd))
	// XXX add stack trace here
	reply, err := protocolError(cmd.Tag, "internal error: %v",
		strconv.Itoa(int(t.Unix())))
	if err != nil {
		return nil, err
	}
	return reply, err
}

type PerfCollector struct {
	sync.Mutex

	sinkC chan interface{}

	stopCollectionC chan struct{} // XXX this probably should go into the sink

	cfg *config

	allowedKeys map[string]struct{}
}

// sinkRegister registers the provided encoder as the sink.
type sinkRegister struct {
	encoder *gob.Encoder
	replyC  chan error
}

// sinkStatusReply is the reply to a sinkStatus request.
type sinkStatusReply struct {
	sink        bool // True if encoder is valid
	measurement bool // True if measurement queue is valid

	startCollection *types.PCStartCollection // Original command

	measurementsFree int // Number of free measurement slots
}

// sinkStatus requests a status update from the sink.
type sinkStatus struct {
	replyC chan sinkStatusReply
}

// measurementRegister registers te provided channel as te measurement queue.
type measurementRegister struct {
	startCollection types.PCStartCollection
	measurementC    chan *types.PCCollection
}

// measurementDrain instruct the sink to drain the measurement queue.
type measurementDrain struct{}

// getSinkStatus returns the current sink status.
func (p *PerfCollector) getSinkStatus(ctx context.Context) (*sinkStatusReply, error) {
	replyC := make(chan sinkStatusReply)
	err := ch.Write(ctx, p.sinkC, sinkStatus{
		replyC: replyC,
	})
	if err != nil {
		return nil, err
	}

	ssr, err := ch.Read(ctx, replyC)
	if err != nil {
		return nil, err
	}
	//log.Errorf("%T", ssr)
	s := ssr.(sinkStatusReply)
	return &s, nil
}

// collectorRunning return true if the collector is running.
func (p *PerfCollector) collectorRunning(ctx context.Context) (bool, error) {
	ss, err := p.getSinkStatus(ctx)
	if err != nil {
		return false, err
	}
	return ss.measurement, nil
}

func (p *PerfCollector) sink(ctx context.Context) {
	log.Tracef("sink")
	defer log.Tracef("sink exit")

	var (
		encoder         *gob.Encoder
		measurementC    chan *types.PCCollection
		startCollection types.PCStartCollection
	)

	for {
	restart:
		log.Tracef("sink loop")
		select {
		case <-ctx.Done():
		case cmd, ok := <-p.sinkC:
			if !ok {
				log.Errorf("sink: sink channel died")
				return
			}

			switch c := cmd.(type) {
			case sinkRegister:
				log.Tracef("sink: sinkRegister %p", c.encoder)
				if encoder != nil && c.encoder != nil {
					log.Tracef("sink: sink already registered")
					ch.WriteNB(ctx, c.replyC,
						fmt.Errorf("sink already registered"))
				} else {
					encoder = c.encoder
					ch.WriteNB(ctx, c.replyC, nil)
				}
				continue

			case measurementRegister:
				log.Tracef("sink: measurementRegister %p",
					c.measurementC)
				startCollection = c.startCollection
				measurementC = c.measurementC
				continue

			case sinkStatus:
				var sc *types.PCStartCollection
				if measurementC != nil {
					scCopy := startCollection
					sc = &scCopy
				}
				ch.WriteNB(ctx, c.replyC, sinkStatusReply{
					sink:            encoder != nil,
					measurement:     measurementC != nil,
					startCollection: sc,
					measurementsFree: cap(measurementC) -
						len(measurementC),
				})

			case measurementDrain:
				if encoder == nil {
					log.Tracef("sink: measurementDrain: "+
						"no encoder %v",
						len(measurementC))
					continue
				}
				if measurementC == nil {
					log.Tracef("sink: measurementDrain: " +
						"no channel")
					continue
				}
				// XXX this code needs to come out.
				// Queue measurement and drain elsewhere.
				for {
					m, err := ch.ReadNB(ctx, measurementC)
					if err == ch.ErrChannelBusy {
						log.Tracef("sink ReadNB: %v",
							err)
						goto restart
					} else if err != nil {
						log.Errorf("sink ReadNB: %v",
							err)
						goto restart
					}
					log.Tracef("Sent measurement")
					err = encoder.Encode(m)
					if err != nil {
						log.Errorf("sink encoder: %v",
							err)
						encoder = nil
						continue
					}
				}

			default:
				log.Errorf("sink: unknown type %T", cmd)
			}
		}
	}
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

func (p *PerfCollector) handleRegisterSink(ctx context.Context, cmd types.PCCommand, channel ssh.Channel) (func(), []byte, error) {
	log.Tracef("handleRegisterSink %v", cmd.Tag)
	defer log.Tracef("handleRegisterSink %v exit", cmd.Tag)

	// Instruct sink to register gob encoder.
	replyC := make(chan error)
	if err := ch.Write(ctx, p.sinkC, sinkRegister{
		encoder: gob.NewEncoder(channel),
		replyC:  replyC,
	}); err != nil {
		return nil, nil, err
	}

	// Non-block read of sink reply.
	err2, readErr := ch.Read(ctx, replyC)
	if readErr != nil {
		return nil, nil, readErr
	}
	if err2 != nil {
		reply, err := protocolError(cmd.Tag, "command %v: %v",
			cmd.Cmd, err2)
		return nil, reply, err
	}

	reply, err := types.Encode(types.PCCommand{
		Version: types.PCVersion,
		Tag:     cmd.Tag,
		Cmd:     types.PCAck,
	})

	var callback func()
	if err == nil {
		// Set unregister callback
		callback = func() {
			log.Tracef("handleRegisterSink: unregister callback")
			err := ch.Write(ctx, p.sinkC, sinkRegister{
				encoder: nil,
			})
			if err != nil {
				log.Errorf("handleRegisterSink unregister: %v",
					err)
			}
		}
	}

	return callback, reply, err
}

func (p *PerfCollector) startCollection(ctx context.Context, sc types.PCStartCollection) {
	log.Tracef("startCollection %v", sc.Frequency)
	defer log.Tracef("startCollection %v exit", sc.Frequency)

	// Message new measurements channel
	measurements := make(chan *types.PCCollection, sc.QueueDepth)
	err := ch.Write(ctx, p.sinkC, measurementRegister{
		startCollection: sc,
		measurementC:    measurements,
	})
	if err != nil {
		log.Errorf("startCollection Write: %v", err)
		return
	}

	p.stopCollectionC = make(chan struct{})

	t := time.Tick(sc.Frequency) // XXX Replace this with an elapsed time counter
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopCollectionC:
			err := ch.Write(ctx, p.sinkC, measurementRegister{
				measurementC: nil,
			})
			if err != nil {
				log.Errorf("startCollection Write2: %v", err)
			}
			p.stopCollectionC = nil
			return

		case <-t:
			timestamp := time.Now()
			for _, v := range sc.Systems {
				m := types.PCCollection{
					System:    v,
					Timestamp: timestamp,  // Overall timestamp
					Start:     time.Now(), // This timestamp
				}

				blob, err := util.Measure(v)
				if err != nil {
					log.Errorf("startCollection: %v", err)
					// Abort measurement.
					continue
				}
				m.Measurement = string(blob)

				m.Duration = time.Now().Sub(m.Timestamp)

				// Spill last measurement if queue depth is
				// reached
				err = ch.WriteNB(ctx, measurements, &m)
				if err != nil && err == ch.ErrChannelBusy {
					log.Tracef("startCollection spill: %v",
						len(measurements))
				} else if err != nil {
					// This may be fatal but let ctx deal
					// with that.
					log.Errorf("startCollection WriteNB: %v",
						err)
				}
			}
			// Signal once that there are measurements to drain.
			// XXX think about always draining. This may not be a
			// good idea when we are polling performance every
			// second.
			err = ch.WriteNB(ctx, p.sinkC, measurementDrain{})
			if err != nil {
				log.Tracef("measurementDrain signal failed: %v",
					err)
			}
		}
	}
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
		if util.ValidSystem(v) {
			continue
		}
		return protocolError(cmd.Tag, "invalid system %v", v)
	}

	// Only allow one collection to run.
	cr, err := p.collectorRunning(ctx)
	if err != nil {
		return internalError(cmd, err)
	}
	if cr {
		return protocolError(cmd.Tag, "collector already running")
	}

	reply := types.PCCommand{
		Version: types.PCVersion,
		Tag:     cmd.Tag,
		Cmd:     types.PCAck,
	}
	go p.startCollection(ctx, sc)

	// Ack remote.
	return types.Encode(reply)
}

func (p *PerfCollector) handleStopCollection(ctx context.Context, cmd types.PCCommand, channel ssh.Channel) ([]byte, error) {
	log.Tracef("handleStopCollection %v", cmd.Cmd)
	defer log.Tracef("handleStopCollection %v exit", cmd.Cmd)

	if cmd.Payload != nil {
		return protocolError(cmd.Tag, "invalid stop collector payload")
	}

	cr, err := p.collectorRunning(ctx)
	if err != nil {
		return internalError(cmd, err)
	}
	if !cr {
		return protocolError(cmd.Tag, "collector not running")
	}

	close(p.stopCollectionC)

	// Ack remote.
	reply := types.PCCommand{
		Version: types.PCVersion,
		Tag:     cmd.Tag,
		Cmd:     types.PCAck,
	}
	return types.Encode(reply)
}

func (p *PerfCollector) handleStatusCollection(ctx context.Context, cmd types.PCCommand, channel ssh.Channel) ([]byte, error) {
	log.Tracef("handleStatusCollection %v", cmd.Cmd)
	defer log.Tracef("handleStatusCollection %v exit", cmd.Cmd)

	ss, err := p.getSinkStatus(ctx)
	if err != nil {
		return internalError(cmd, err)
	}

	reply := types.PCCommand{
		Version: types.PCVersion,
		Tag:     cmd.Tag,
		Cmd:     types.PCStatusCollectionReplyCmd,
		Payload: types.PCStatusCollectionReply{
			StartCollection:    ss.startCollection,
			QueueFree:          ss.measurementsFree,
			SinkEnabled:        ss.sink,
			MeasurementEnabled: ss.measurement,
		},
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

func (p *PerfCollector) oobHandler(ctx context.Context, channel ssh.Channel, requests <-chan *ssh.Request) {
	log.Tracef("oobHandler")
	defer func() {
		log.Tracef("oobHandler exit")
	}()

	for req := range requests {
		_ = req
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
		case types.PCRegisterSinkCmd:
			var callback func()
			callback, reply, err = p.handleRegisterSink(ctx, cmd,
				channel)
			if callback != nil {
				// Unregister on exit.
				defer callback()
			}
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
			var errPanic error
			reply, errPanic = internalError(cmd, err)
			if errPanic != nil {
				// If we don't reply the client will hang.
				panic(fmt.Sprintf("err: %v; errPanic %v",
					err, errPanic))
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
	defer func() {
		log.Tracef("handleChannel exit")
	}()

	if t := newChannel.ChannelType(); t != types.PCChannel {
		_ = newChannel.Reject(ssh.UnknownChannelType,
			fmt.Sprintf("unknown channel type: %s", t))
		log.Errorf("handleChannel: unknown channel %s", t)
		return
	}

	channel, requests, err := newChannel.Accept()
	if err != nil {
		log.Errorf("handleChannel: could not accept channel (%s)", err)
		return
	}
	defer channel.Close()

	go p.oobHandler(ctx, channel, requests)

	for {
		// We do not use the read sink in the collector. Just log the
		// line if something comes in.
		r := bufio.NewReader(channel)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				log.Tracef("handleChannel read error: %v", err)
				return
			}
			log.Debugf(line)
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

func (p *PerfCollector) sshServe(ctx context.Context, listen string, signer ssh.Signer) error {
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

	p := &PerfCollector{
		cfg:         loadedCfg,
		allowedKeys: make(map[string]struct{}),
		sinkC:       make(chan interface{}),
	}
	for _, v := range p.cfg.AllowedKeys {
		p.allowedKeys[v] = struct{}{}
	}

	log.Infof("Version      : %v", version())
	log.Infof("Home dir     : %v", p.cfg.HomeDir)

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
	ctx, cancel := context.WithCancel(context.Background())
	go p.sink(ctx)

	// Listen for incoming SSH connections.
	listenC := make(chan error)
	for _, listener := range loadedCfg.Listeners {
		listen := listener
		go func() {
			listenC <- p.sshServe(ctx, listen, signer)
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
	cancel()
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
