package load

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
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

func getCPUCores() (uint64, error) {
	// XXX get value from: cat /proc/cpuinfo | grep "cpu cores"
	return uint64(runtime.NumCPU()), nil
}

func FindPrimesParallel(n, cores uint64) (time.Duration, uint64, error) {
	// Launch workers
	var (
		wg    sync.WaitGroup
		found uint64 // atomic
	)
	ctx, cancel := context.WithCancel(context.Background())
	pipe := make(chan *big.Int, int(cores))
	for x := uint64(0); x < cores; x++ {
		wg.Add(1)
		go func(me uint64) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case p, ok := <-pipe:
					if !ok {
						return
					}
					_ = p
					UserWork(1)
					newFound := atomic.AddUint64(&found, 1)
					if newFound >= n {
						cancel()
						return
					}
					continue

					//if !isPrime(p) {
					//	continue
					//}
					//// We have a prime, add to total and exit all
					//// go routines if done.
					//newFound := atomic.AddUint64(&found, 1)
					//if newFound >= n {
					//	cancel()
					//	return
					//}
				}
			}
		}(x)
	}

	// Work
	start := time.Now()
	i := new(big.Int).Set(zero)
	for {
		if atomic.LoadUint64(&found) >= n {
			break
		}

		select {
		case <-ctx.Done():
			break
		case pipe <- new(big.Int).Set(i):
		}
		i.Add(i, one)
	}
	wg.Wait()

	return time.Now().Sub(start), found, nil
}
