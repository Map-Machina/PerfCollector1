package load

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/businessperformancetuning/perfcollector/util"
)

func userIdle(duration time.Duration) {
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
	userIdle(time.Duration(idle))
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
	userIdle(time.Duration(idle))
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
		userIdle(idleDuration)
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
		userIdle(idleFractionDuration)
	}

	return userLoops, systemLoops, nil
}

func unit() {
	//var mem [1024 * 1024]int32
	var mem [1024 * 1024]int32
	for k := range mem {
		mem[k] = mem[k] + int32(k)
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
