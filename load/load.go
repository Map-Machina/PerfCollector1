package load

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/big"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/businessperformancetuning/perfcollector/parser"
	"github.com/businessperformancetuning/perfcollector/util"
	"github.com/inhies/go-bytesize"
)

// MeasureUnitsPerSecond runs a single core at maximum speed for 1 second. It
// then runs the same load for the provided duration and ensures that the rough
// and fine measurement are within 5% of one another.
func MeasureUnitsPerSecond(ctx context.Context, fine uint) (float64, error) {
	// Get rough measurement
	durationRough, unitsRough, err := ExecuteParallel(ctx, time.Second,
		10000, 1, RMW)
	if err != nil {
		// Expect error
		if err != context.DeadlineExceeded {
			return 0, err
		}
	}
	//fmt.Printf("rough measurement: %v %v\n", unitsRough, durationRough)

	// Verify measurement
	durationFine, unitsFine, err := ExecuteParallel(ctx,
		2*time.Duration(fine)*time.Second, uint64(fine)*unitsRough,
		1, RMW)
	if err != nil {
		return 0, err
	}
	//fmt.Printf("fine measurement: %v %v\n", unitsFine, durationFine)

	// If units or time are not within 5% error out to let caller know that
	// the test needs to probably be restarted.
	ur := float64(unitsRough)
	uf := float64(unitsFine) / float64(fine)
	variance := (ur - uf) / uf
	//fmt.Printf("variance measurement: ur %v uf %v var %v\n", ur, uf, variance)
	if math.Abs(variance) > 0.05 {
		return 0, fmt.Errorf("invalid units measurement: %.2f%%",
			variance*100)
	}

	// If time is not withing 5% also let caller know.
	df := float64(durationFine) / float64(fine)
	durationVariance := (float64(durationRough) - df) / df
	if math.Abs(durationVariance) > 0.05 {
		return 0, fmt.Errorf("invalid time measurement: %.2f%%",
			durationVariance*100)
	}
	//fmt.Printf("variance measurement: var %v\n", durationVariance)

	return uf, nil
}

// NumCores returns the number of CPU cores and the number of threads.
func NumCores() (uint, uint, error) {
	// Assume linux for now.
	blob, err := ioutil.ReadFile("/proc/cpuinfo")
	if err != nil {
		return 0, 0, err
	}
	ci, err := parser.ProcessCPUInfo(blob)
	if err != nil {
		return 0, 0, err
	}

	return ci[0].CPUCores, uint(len(ci)), nil
}

// UserIdle idles the caller for the provided duration.
func UserIdle(duration time.Duration) {
	to := time.After(duration)
	<-to
}

func user(percent float64, duration time.Duration) int {
	if percent <= 0 {
		return 0
	}
	if percent > 1 {
		percent = 1
	}

	busy := float64(duration) * percent
	idle := float64(time.Second) - busy
	userLoops := UserLoad(time.Duration(busy))
	UserIdle(time.Duration(idle))
	return userLoops
}

func systemLoad(duration time.Duration) (loops int) {
	start := time.Now()
	b := make([]byte, 16384)
	for {
		rand.Read(b)
		elapsed := time.Now().Sub(start)
		loops++
		if elapsed > duration {
			return
		}
	}
}

func system(percent float64, duration time.Duration) int {
	if percent <= 0 {
		return 0
	}
	if percent > 1 {
		percent = 1
	}

	busy := float64(duration) * percent
	idle := float64(time.Second) - busy
	systemLoops := systemLoad(time.Duration(busy))
	UserIdle(time.Duration(idle))
	return systemLoops
}

func MeasureCombined(percentUser, percentSystem float64, duration time.Duration) (int, int, error) {
	if percentUser < 0 || percentUser > 1 {
		return 0, 0, fmt.Errorf("invalid user percentage: %v",
			percentUser)
	}
	if percentSystem < 0 || percentSystem > 1 {
		return 0, 0, fmt.Errorf("invalid system percentage: %v",
			percentSystem)
	}
	if percentUser+percentSystem > 1 {
		return 0, 0, fmt.Errorf("invalid total percentage: %v",
			percentUser+percentSystem)
	}

	seconds := duration / time.Second
	fraction := float64(duration % time.Second)
	second := float64(time.Second)
	userDuration := time.Duration(percentUser * second)
	systemDuration := time.Duration(percentSystem * second)
	idleDuration := time.Second - userDuration - systemDuration
	if idleDuration < 0 {
		panic(fmt.Sprintf("impossible idleDuration: %v", idleDuration))
	}
	var userLoops, systemLoops int
	for i := time.Duration(0); i < seconds; i++ {
		userLoops += UserLoad(userDuration)
		systemLoops += systemLoad(systemDuration)
		UserIdle(idleDuration)
	}

	// Deal with fraction. This is rather expensive and should be avoided.
	if fraction != 0 {
		userFractionDuration := time.Duration(percentUser * fraction)
		systemFractionDuration := time.Duration(percentSystem * fraction)
		idleFractionDuration := time.Duration(fraction) -
			userFractionDuration - systemFractionDuration
		if idleFractionDuration < 0 {
			panic(fmt.Sprintf("impossible idleFractionDuration: %v",
				idleFractionDuration))
		}
		userLoops += UserLoad(userFractionDuration)
		systemLoops += systemLoad(systemFractionDuration)
		UserIdle(idleFractionDuration)
	}

	return userLoops, systemLoops, nil
}

// Declare some variables for unit of work tests. This probably needs to become
// a receiver so that we can vary the load based on the hardware.
var (
	unitStart = uint32(0xaa55aa55)
	unitCount = uint32(17)
	unitXor   = uint32(0x55aa55aa)
	mem       [1024 * 1024]uint32
)

func unit() {
	for k := range mem {
		// Spin CPU for a bit so that we don't tight loop memory which
		// interferes with parallelzing the load.
		var x uint32
		for i := unitStart; i < unitStart+unitCount; i++ {
			x = x + i
			x |= unitXor
		}
		// Do a Read-Modify-Write with result and index.
		mem[k] = (mem[k] + uint32(k)) ^ uint32(x)
	}
}

func UserLoad(duration time.Duration) (loops int) {
	start := time.Now()
	for {
		unit()
		loops++
		if time.Now().Sub(start) > duration {
			return
		}
	}
}

func UserWork(workUnits int) (elapsed time.Duration) {
	start := time.Now()
	for {
		unit()
		workUnits--
		elapsed = time.Now().Sub(start)
		if workUnits == 0 {
			return
		}
	}
}

var (
	zero = big.NewInt(0)
	one  = big.NewInt(1)
	two  = big.NewInt(2)
)

// Worst possible prime number calculator.
func isPrime(n *big.Int) bool {
	if n.Cmp(two) == -1 {
		return false
	}

	sn := new(big.Int).Sqrt(n)
	i := new(big.Int).Set(two)
	z := new(big.Int)
	for {
		if i.Cmp(sn) == 1 {
			break
		}
		if z.Mod(n, i).Cmp(zero) == 0 {
			return false
		}

		i.Add(i, one)
	}

	return true
}

// Add context?
func FindPrimes(n uint) time.Duration {
	start := time.Now()
	found := uint(0)
	i := new(big.Int).Set(zero)
	for {
		if found >= n {
			break
		}
		if isPrime(i) {
			found++
		}
		i.Add(i, one)
	}
	return time.Now().Sub(start)
}

// RMW Read-Modify-Write test. Always returns false for the continue condition.
func RMW(p *big.Int) bool {
	UserWork(1)
	return false
}

// Prime test returns true for the continue condition if the provided number is
// not a prime.
func Prime(p *big.Int) bool {
	return !isPrime(p)
}

// ExecuteParallel excutes n functions f with the supplied workers threads.
func ExecuteParallel(parent context.Context, maxDuration time.Duration, n, workers uint64, f func(*big.Int) bool) (time.Duration, uint64, error) {
	// Launch workers
	var (
		wg    sync.WaitGroup
		found uint64 // atomic
	)
	ctx, cancel := context.WithTimeout(parent, maxDuration)
	pipe := make(chan *big.Int, int(workers))
	for x := uint64(0); x < workers; x++ {
		wg.Add(1)
		go func(me uint64) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case p := <-pipe:
					if f(p) {
						continue
					}

					newFound := atomic.AddUint64(&found, 1)
					if newFound >= n {
						cancel()
						return
					}
				}
			}
		}(x)
	}

	// Work
	var returnError error
	start := time.Now()
	i := new(big.Int).Set(zero)
	for {
		if atomic.LoadUint64(&found) >= n {
			break
		}

		select {
		case <-ctx.Done():
			if context.Canceled != ctx.Err() {
				returnError = ctx.Err()
			}
			goto done
		case pipe <- new(big.Int).Set(i):
		}
		i.Add(i, one)
	}
done:
	wg.Wait()

	return time.Now().Sub(start), found, returnError
}

func DiskWrite(parent context.Context, maxDuration time.Duration, filename string, units, size uint64) (time.Duration, uint64, error) {
	ctx, _ := context.WithTimeout(parent, maxDuration)

	block, err := util.Random(int(size))
	if err != nil {
		return 0, 0, err
	}

	// Work
	start := time.Now()
	f, err := os.OpenFile(filename,
		os.O_CREATE|os.O_TRUNC|os.O_SYNC|os.O_WRONLY|syscall.O_DIRECT,
		0644)
	if err != nil {
		return 0, 0, err
	}
	// Close file if not closed already.
	defer func() {
		if f != nil {
			f.Close()
		}
	}()
	for i := uint64(0); i < units; i++ {
		_, err = f.Write(block)
		if err != nil {
			return time.Now().Sub(start), uint64(i), err
		}
		select {
		case <-ctx.Done():
			return time.Now().Sub(start), uint64(i), ctx.Err()
		default:
		}
	}
	err = f.Sync()
	if err != nil {
		return time.Now().Sub(start), units, err
	}
	err = f.Close()
	if err != nil {
		return time.Now().Sub(start), units, err
	}
	f = nil

	return time.Now().Sub(start), units, err
}

func DiskRead(parent context.Context, maxDuration time.Duration, filename string, units, size uint64) (time.Duration, uint64, error) {
	ctx, _ := context.WithTimeout(parent, maxDuration)

	block := make([]byte, size)

	// Work
	start := time.Now()
	f, err := os.OpenFile(filename,
		os.O_SYNC|os.O_RDONLY|syscall.O_DIRECT, 0644)
	if err != nil {
		return 0, 0, err
	}
	// Close file if not closed already.
	defer func() {
		if f != nil {
			f.Close()
		}
	}()
	for i := uint64(0); i < units; i++ {
		_, err = f.Read(block)
		if err != nil {
			return time.Now().Sub(start), uint64(i), err
		}
		select {
		case <-ctx.Done():
			return time.Now().Sub(start), uint64(i), ctx.Err()
		default:
		}
	}
	err = f.Sync()
	if err != nil {
		return time.Now().Sub(start), units, err
	}
	err = f.Close()
	if err != nil {
		return time.Now().Sub(start), units, err
	}
	f = nil

	return time.Now().Sub(start), units, err
}

// NetCommand instructs the server what to do. The server listens and executes
// the workload the client requests.
type NetCommand struct {
	Version uint
	Command string
	Units   uint64
	Size    uint64
}

type NetServerResult struct {
	Duration   time.Duration
	Bytes      uint64
	Verb       string
	RemoteAddr string
}

// NetServer waits for a client connection and reads/writes units*size bytes
// based on the client command.
// Note that NetServer returns bytes processed and not units processes.
func NetServer(parent context.Context, maxDuration time.Duration, listen string) (NetServerResult, error) {
	ctx, _ := context.WithTimeout(parent, maxDuration)

	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", listen)
	if err != nil {
		return NetServerResult{}, err
	}

	tl := ln.(*net.TCPListener)
	for {
		nsr := NetServerResult{} // Always return nsr
		// See if context has expired
		select {
		case <-ctx.Done():
			return nsr, ctx.Err()
		default:
		}

		// Set deadline to 1 second and restart
		tl.SetDeadline(time.Now().Add(time.Second))
		conn, err := tl.Accept()
		if err != nil {
			if e, ok := err.(*net.OpError); ok {
				if e.Timeout() {
					continue
				}
			}
			return nsr, err
		}
		nsr.RemoteAddr = conn.RemoteAddr().String()

		// We only allow one connection at a time

		// Wait for client command
		r := bufio.NewReader(conn)
		jsonBlob, err := r.ReadString('\n')
		if err != nil {
			return nsr, err
		}

		var nc NetCommand
		err = json.Unmarshal([]byte(jsonBlob), &nc)
		if err != nil {
			return nsr, err
		}

		var start time.Time
		switch nc.Command {
		case "write":
			// Client write is server read
			nsr.Verb = nc.Command
			block := make([]byte, nc.Size)
			conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			start = time.Now()
			for {
				n, err := r.Read(block)
				nsr.Bytes += uint64(n)
				if err != nil {
					nsr.Duration = time.Now().Sub(start)
					return nsr, err
				}

				if nsr.Bytes >= nc.Units*nc.Size {
					goto done
				}

				select {
				case <-ctx.Done():
					nsr.Duration = time.Now().Sub(start)
					return nsr, ctx.Err()
				default:
				}

			}

		case "read":
			// Client read is server write
			nsr.Verb = nc.Command
			block, err := util.Random(int(nc.Size))
			if err != nil {
				return nsr, err
			}
			start = time.Now()
			for i := uint64(0); i < nc.Units; i++ {
				n, err := conn.Write(block)
				nsr.Bytes += uint64(n)
				if err != nil {
					nsr.Duration = time.Now().Sub(start)
					return nsr, err
				}
			}

		default:
			return nsr, fmt.Errorf("invalid net command: %v",
				nc.Command)
		}
	done:
		nsr.Duration = time.Now().Sub(start)
		conn.Close()

		return nsr, err
	}
}

// NetClient connects and reads/writes units*size bytes.
// Note that NetClient returns bytes processed and not units processes.
func NetClient(parent context.Context, maxDuration time.Duration, command, connect string, units uint, size bytesize.ByteSize) (time.Duration, uint64, error) {
	ctx, _ := context.WithTimeout(parent, maxDuration)

	conn, err := net.Dial("tcp", connect)
	if err != nil {
		return 0, 0, err
	}

	// Write command. The command is written in JSON with a terminating \n.
	// This is needed becasue readers are greedy and the other end may have
	// leftovers causing a short read.
	//
	// By writing a JSON \n terminated blob the reader can read up to \n
	// without affecting the underlying raw connection.
	jsonBlob, err := json.Marshal(NetCommand{
		Version: 1,
		Command: command,
		Units:   uint64(units),
		Size:    uint64(size),
	})
	if err != nil {
		return 0, 0, err
	}
	_, err = fmt.Fprintf(conn, "%s\n", jsonBlob)
	if err != nil {
		return 0, 0, err
	}

	var (
		x     uint64
		start time.Time
	)
	switch command {
	case "write":
		block := make([]byte, size)
		start = time.Now()
		for {
			n, err := conn.Write(block)
			if err != nil {
				return time.Now().Sub(start), x, err
			}
			x += uint64(n)
			if x >= uint64(units)*uint64(size) {
				break
			}

			select {
			case <-ctx.Done():
				return time.Now().Sub(start), x, ctx.Err()
			default:
			}
		}

	case "read":
		block := make([]byte, size)
		start = time.Now()
		for {
			n, err := conn.Read(block)
			if err != nil {
				return time.Now().Sub(start), x, err
			}
			x += uint64(n)
			if x >= uint64(units)*uint64(size) {
				break
			}

			select {
			case <-ctx.Done():
				return time.Now().Sub(start), x, ctx.Err()
			default:
			}
		}

	default:
	}

	d := time.Now().Sub(start)

	// Wait for EOF
	b := []byte{0xff}
	_, err = conn.Read(b)
	if err != nil {
		if err == io.EOF {
			return d, x, nil
		}
		return d, x, err
	}

	return d, x, nil
}
