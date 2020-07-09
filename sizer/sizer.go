package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"time"

	"github.com/businessperformancetuning/perfcollector/types"
	"github.com/davecgh/go-spew/spew"
	"github.com/prometheus/procfs"
	"golang.org/x/crypto/ssh"
)

/*
PrevIdle = previdle + previowait
Idle = idle + iowait

PrevNonIdle = prevuser + prevnice + prevsystem + previrq + prevsoftirq + prevsteal
NonIdle = user + nice + system + irq + softirq + steal

PrevTotal = PrevIdle + PrevNonIdle
Total = Idle + NonIdle

# differentiate: actual value minus the previous one
totald = Total - PrevTotal
idled = Idle - PrevIdle

CPU_Percentage = (totald - idled)/totald

https://stackoverflow.com/questions/23367857/accurate-calculation-of-cpu-usage-given-in-percentage-in-linux
*/

func cookStat(stats procfs.Stat) error {
	c := stats.CPUTotal
	spew.Dump(c)
	return nil
	usertime := c.User - c.Guest
	nicetime := c.Nice - c.GuestNice

	idlealltime := c.Idle + c.Iowait
	systemalltime := c.System + c.IRQ + c.SoftIRQ
	virtalltime := c.Guest + c.GuestNice
	totaltime := usertime + nicetime + systemalltime + idlealltime + c.Steal + virtalltime

	fmt.Printf("total %v v %v s %v i %v n %v u %v\n",
		totaltime,
		virtalltime,
		systemalltime,
		idlealltime,
		nicetime,
		usertime)
	fmt.Printf("user %.2f system %.2f nice %.2f idle %.2f %.2f\n",
		usertime/totaltime*100,
		systemalltime/totaltime*100,
		nicetime/totaltime*100,
		idlealltime/totaltime*100,
		(totaltime-idlealltime)/totaltime*100)

	return nil
}

func _main() error {
	ticker := time.NewTicker(1000 * time.Millisecond)
	fs, err := procfs.NewDefaultFS()
	if err != nil {
		return err
	}

	for {
		select {
		//case <-done:
		//	return nil
		case t := <-ticker.C:
			x := time.Now()
			stats, err := fs.Stat()
			if err != nil {
				return err
			}
			mem, err := fs.Meminfo()
			delta := time.Now().Sub(x)
			fmt.Println(delta)
			_ = stats
			_ = mem
			//err = cookStat(stats)
			//if err != nil {
			//	return nil
			//}
			//fmt.Printf("CPU: user %v system %v idle %v\n",
			//	stats.CPUTotal.User,
			//	stats.CPUTotal.System,
			//	stats.CPUTotal.Idle)
			_ = t
		}
	}

	return nil
}

func sshMain() error {
	//sessionHandler := gssh.Handler(func(s gssh.Session) {
	//	log.Printf("user %v %v\n", s.User(), s.RemoteAddr())

	//	io.WriteString(s, fmt.Sprintf("Hello world %v\n"))

	//	for {
	//		buf := make([]byte, 128, 4096)
	//		n, err := s.Read(buf)
	//		if err != nil {
	//			log.Printf("connection closed: %v", err)
	//			return
	//		}
	//		log.Printf("got: %v %v", n, string(buf))
	//		time.Sleep(time.Second)
	//	}
	//})

	//sessionRequest := gssh.SessionRequestCallback(func(s gssh.Session, requestType string) bool {
	//	log.Printf("session request: %v", requestType)
	//	return true
	//})

	//publicKeyHandler := func(ctx gssh.Context, key gssh.PublicKey) bool {
	//	log.Printf("Allow key %x\n", key)
	//	return true // allow all keys, or use gssh.KeysEqual() to compare against known keys
	//}

	//s := &gssh.Server{
	//	Addr:                   ":2222",
	//	Handler:                sessionHandler,
	//	PublicKeyHandler:       publicKeyHandler,
	//	SessionRequestCallback: sessionRequest,
	//}
	//s.SetOption(gssh.HostKeyFile("~/.gssh/id_ed25519"))
	//log.Fatal(s.ListenAndServe())

	listener, err := net.Listen("tcp", "0.0.0.0:2222")
	if err != nil {
		log.Fatalf("Failed to listen on 0.0.0.0:2222 (%s)", err)
	}
	log.Printf("Listening on 0.0.0.0:2222")

	sshConfig := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			log.Printf("PublicKeyHandler: %v", ssh.FingerprintSHA256(key))
			return &ssh.Permissions{}, nil
		},
	}
	// endregion

	// region Host key
	hostKeyData, err := ioutil.ReadFile("/home/marco/.ssh/id_ed25519")
	if err != nil {
		log.Fatalf("failed to load host key (%s)", err)
	}
	signer, err := ssh.ParsePrivateKey(hostKeyData)
	if err != nil {
		log.Fatalf("failed to parse host key (%s)", err)
	}
	sshConfig.AddHostKey(signer)
	// endregion

	for {
		tcpConn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept incoming connection (%s)", err)
			// Continue with the next loop
			continue
		}

		sshConn, chans, reqs, err := ssh.NewServerConn(tcpConn, sshConfig)
		if err != nil {
			log.Printf("handshake failed (%s)", err)
			continue
		}

		go ssh.DiscardRequests(reqs)
		go handleChannels(sshConn, chans)
	}

	return nil
}

func handleChannels(conn *ssh.ServerConn, chans <-chan ssh.NewChannel) {
	for newChannel := range chans {
		log.Printf("handleChannels: %v", newChannel.ChannelType())
		go handleChannel(conn, newChannel)
	}
}

func handleChannel(conn *ssh.ServerConn, newChannel ssh.NewChannel) {
	if t := newChannel.ChannelType(); t != "collector" {
		_ = newChannel.Reject(ssh.UnknownChannelType,
			fmt.Sprintf("unknown channel type: %s", t))
		return
	}

	channel, requests, err := newChannel.Accept()
	if err != nil {
		log.Printf("could not accept channel (%s)", err)
		return
	}

	go func(in <-chan *ssh.Request) {
		log.Printf("start req handler")
		for req := range in {
			log.Printf("req.Type: %v %s", req.Type, req.Payload)
			req.Reply(req.Type == "moo", []byte("payload reply"))
		}
		log.Printf("exit req handler")
	}(requests)

	_, err = channel.Write([]byte("Hello world from server\n"))
	if err != nil {
		log.Printf("write: %v", err)
		return
	}

	go func() {
		log.Printf("loop")
		defer channel.Close()
		r := bufio.NewReader(channel)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				log.Printf("readstring: %v", err)
				return
			}
			log.Printf(line)
		}
	}()

	//defer conn.Close()
	//_ = channel

	//go handleRequests(requests)

	//log.Printf("ready to handle collector")

	////
	//_, err = channel.Write([]byte("Hello world from server"))
	//if err != nil {
	//	log.Printf("channel write: %v", err)
	//	return
	//}

	//data := make([]byte, 0, 100)
	//_, err = channel.Read(data)
	//if err != nil {
	//	log.Printf("channel read: %v", err)
	//	return
	//}
	//spew.Dump(data)
}

func handleRequests(r <-chan *ssh.Request) {
	//for {
	select {
	case req := <-r:
		log.Printf("req: %v", req)
	}
	//}
	log.Printf("exiting handleRequests")
}

func PublicKeyFile(file string) ssh.AuthMethod {
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		return nil
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil
	}
	return ssh.PublicKeys(key)
}

func sshClient() error {
	//var hostKey ssh.PublicKey

	//key, err := ioutil.ReadFile("/home/marco/.ssh/id_ed25519")
	//if err != nil {
	//	return err
	//}

	//// Create the Signer for this private key.
	//signer, err := ssh.ParsePrivateKey(key)
	//if err != nil {
	//	return err
	//}

	// Create client config
	config := &ssh.ClientConfig{
		User: "marco",
		Auth: []ssh.AuthMethod{
			PublicKeyFile("/home/marco/.ssh/id_ed25519"),
		},
		//HostKeyCallback: ssh.FixedHostKey(hostKey),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// Connect to ssh server
	conn, err := ssh.Dial("tcp", "10.170.0.5:2222", config)
	if err != nil {
		return err
	}

	defer conn.Close()

	ch, requests, err := conn.OpenChannel("collector", nil)
	if err != nil {
		return err
	}
	defer ch.Close()

	go func(in <-chan *ssh.Request) {
		log.Printf("start req handler")
		for req := range in {
			log.Printf("req.Type: %v %s\n", req.Type, req.Payload)
			switch req.Type {
			case types.PCCollectOnceReplyCmd:
				// Decode
				or, err := types.Decode(req.Type, req.Payload)
				if err != nil {
					log.Printf("decode error: %v", err)
					continue
				}
				o, ok := or.(*types.PCCollectOnceReply)
				if !ok {
					// Should not happen
					log.Printf("type assertion error %T", or)
					continue
				}
				spew.Dump(o)

			default:
				log.Printf("invalid req: %v", req.Type)
			}
			req.Reply(true, nil)
		}
		log.Printf("exit req handler")
	}(requests)

	// Ask for version
	go func() {
		o, err := types.Encode(types.PCCollectOnce{
			Systems: []string{"version", "uptime", "stat"},
		})
		if err != nil {
			//return err
			return
		}
		ok, err := ch.SendRequest(types.PCCollectOnceCmd, true, o)
		if err != nil {
			//return err
			return
		}
		fmt.Printf("ok: %v\n", ok)
	}()

	_, err = ch.Write([]byte("Hello world from client\n"))
	if err != nil {
		return err
	}

	r := bufio.NewReader(ch)
	for {
		fmt.Printf("loop\n")
		line, err := r.ReadString('\n')
		if err != nil {
			return err
		}
		fmt.Printf("line: %v", line)
	}

	return nil
}

func main() {
	var client = flag.Bool("client", false, "help message for flagname")
	flag.Parse()

	if *client {
		if err := sshClient(); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := sshMain(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
