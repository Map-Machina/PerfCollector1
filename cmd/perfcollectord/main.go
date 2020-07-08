package main

import (
	"bufio"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/businessperformancetuning/sizer/types"
	"github.com/businessperformancetuning/sizer/util"
	"golang.org/x/crypto/ssh"
)

type PerfCollector struct {
	cfg *config

	allowedKeys map[string]struct{}
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

func (p *PerfCollector) handleChannels(conn *ssh.ServerConn, chans <-chan ssh.NewChannel) {
	log.Tracef("handleChannels")
	defer log.Tracef("handleChannels exit")

	for newChannel := range chans {
		log.Tracef("handleChannels: %v", newChannel.ChannelType())
		go p.handleChannel(conn, newChannel)
	}
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
		filename := filepath.Join("/proc", v)
		log.Tracef("handleOnce: %v", filename)
		payload.Values[k], err = ioutil.ReadFile(filename)
		if err != nil {
			return nil, err
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

func (p *PerfCollector) startCollection(sc types.PCStartCollection, channel ssh.Channel, eod chan struct{}) {
	log.Tracef("startCollection %v", sc.Frequency)
	defer log.Tracef("startCollection %v exit", sc.Frequency)

	filenames := make([]string, len(sc.Systems))
	for k, v := range sc.Systems {
		filenames[k] = filepath.Join("/proc", v)
	}

	measurements := make(chan *types.PCCollection, sc.QueueDepth)
	// Flusher function
	go func() {
		log.Tracef("flusher %v", sc.QueueDepth)
		defer log.Tracef("flusher %v exit", sc.QueueDepth)

		// Create network encoder
		enc := gob.NewEncoder(channel)
		for {
			select {
			case m := <-measurements:
				log.Tracef("flusher %v", len(measurements))
				err := enc.Encode(*m)
				if err != nil {
					log.Errorf("flusher encode error: %v",
						err)
				}
			case <-eod:
				return
			}
		}
	}()

	t := time.Tick(sc.Frequency) // Replace this with an elapsed time counter
	for {
		select {
		case <-t:
		case <-eod:
			return
		}
		log.Tracef("startCollection: tick")

		var err error
		for k, v := range sc.Systems {
			m := types.PCCollection{
				System:    v,
				Timestamp: time.Now(),
			}

			m.Measurement, err = ioutil.ReadFile(filenames[k])
			if err != nil {
				log.Errorf("startCollection: %v", err)
				// Abort measurement.
				continue
			}

			m.Duration = time.Now().Sub(m.Timestamp)

			// Spill last measurement if queue depth is reached
			select {
			case measurements <- &m:
			default:
				log.Tracef("startCollection: spill %v",
					len(measurements))
			}
		}

	}
}

func (p *PerfCollector) handleStartCollection(cmd types.PCCommand, channel ssh.Channel, eod chan struct{}) ([]byte, error) {
	log.Tracef("handleStartCollection %v", cmd.Cmd)
	defer log.Tracef("handleStartCollection %v exit", cmd.Cmd)

	sc, ok := cmd.Payload.(types.PCStartCollection)
	if !ok {
		// Should not happen
		return nil, fmt.Errorf("handleStartCollection: type "+
			"assertion error %T", sc)
	}

	// Verify frequency.
	if sc.Frequency < time.Second {
		// XXX return PCError instead
		return nil, fmt.Errorf("bad frequency")
	}

	// Verify that all systems exist.
	for _, v := range sc.Systems {
		filename := filepath.Join("/proc", v)
		if util.FileExists(filename) {
			continue
		}
		// XXX return PCError instead
		return nil, fmt.Errorf("bad system %v", filename)
	}

	// XXX handle already running collection

	go p.startCollection(sc, channel, eod)

	// Ack remote.
	reply := types.PCCommand{
		Version: types.PCVersion,
		Tag:     cmd.Tag,
		Cmd:     types.PCAck,
	}
	return types.Encode(reply)
}

func (p *PerfCollector) oobHandler(channel ssh.Channel, requests <-chan *ssh.Request) {
	log.Tracef("oobHandler")

	// Close channel
	eod := make(chan struct{})

	defer func() {
		close(eod)
		log.Tracef("oobHandler exit")
	}()

	for req := range requests {
		log.Tracef("oobHandler req.Type: %v", req.Type)

		// Always reply or else the other end may hang.
		req.Reply(true, nil)

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

		var (
			cmdId string
			reply []byte
		)
		switch cmd.Cmd {
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

		case types.PCCollectOnceCmd:
			reply, err = p.handleOnce(cmd)
			if err != nil {
				log.Errorf("oobHandler handleOnce: %v", err)
				continue
			}
			cmdId = types.PCCmd

		case types.PCStartCollectionCmd:
			reply, err = p.handleStartCollection(cmd, channel, eod)
			if err != nil {
				log.Errorf("oobHandler handleStartCollection"+
					": %v", err)
				continue
			}
			cmdId = types.PCCmd

		default:
			log.Errorf("oobHandler unknown request: %v", cmd.Cmd)
			cmdId = types.PCCmd
			reply, err = types.Encode(types.PCCommand{
				Tag: cmd.Tag,
				Cmd: types.PCErrorCmd,
				Payload: types.PCError{
					Error: "unknown OOB request: " + cmd.Cmd,
				},
			})
		}

		// Send payload to server.
		if reply == nil {
			// Nothing to do
			continue
		}
		_, err = channel.SendRequest(cmdId, false, reply)
		if err != nil {
			log.Errorf("oobHandler SendRequest: %v", err)
		}
	}
}

func (p *PerfCollector) handleChannel(conn *ssh.ServerConn, newChannel ssh.NewChannel) {
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

	go p.oobHandler(channel, requests)

	//_, err = channel.Write([]byte("Hello world from server\n"))
	//if err != nil {
	//	log.Infof("write: %v", err)
	//	return
	//}

	for {
		log.Infof("loop")
		defer channel.Close()
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
		go p.handleChannels(sshConn, chans)
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
		cfg:         loadedCfg,
		allowedKeys: make(map[string]struct{}),
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
