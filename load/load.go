package load

import (
	"crypto/rand"
	"fmt"
	"math/big"
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

	sn := new(big.Int).Set(n)
	sn.Sqrt(n)
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
