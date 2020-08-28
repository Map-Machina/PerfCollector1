package load

import (
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/inhies/go-bytesize"
)

func TestLoad(t *testing.T) {
	var wg sync.WaitGroup
	for {
		start := time.Now()
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(cpu int) {
				defer wg.Done()
				loops := system(0.15, time.Second)
				t.Logf("cpu: %v loops: %v", cpu, loops)
			}(i)
		}
		wg.Wait()
		elapsed := time.Now().Sub(start)
		t.Logf("elapsed: %v", elapsed)
	}
}

func TestCombinedLoad(t *testing.T) {
	var wg sync.WaitGroup
	for {
		start := time.Now()
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(cpu int) {
				defer wg.Done()
				ul, sl, err := MeasureCombined(0.15, 0.25,
					5*time.Second)
				if err != nil {
					t.Fatalf("cpu %v %v", cpu, err)
				}
				t.Logf("cpu: %v user: %v system: %v",
					cpu, ul, sl)
			}(i)
		}
		wg.Wait()
		elapsed := time.Now().Sub(start)
		t.Logf("elapsed: %v", elapsed)
	}
}

func TestCombinedWorkAll(t *testing.T) {
	var userLoops [8]int
	var elapsedCPU [8]time.Duration
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(cpu int) {
			defer wg.Done()
			userLoops[cpu] = UserLoad(10 * time.Second)
		}(i)
	}
	wg.Wait()
	elapsed := time.Now().Sub(start)
	t.Logf("elapsed: %v", elapsed)

	start2 := time.Now()
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(cpu int) {
			defer wg.Done()
			elapsedCPU[cpu] = UserWork(userLoops[cpu])
		}(i)
	}
	wg.Wait()
	elapsed2 := time.Now().Sub(start2)
	t.Logf("elapsed2: %v", elapsed2)

	for i := 0; i < 8; i++ {
		t.Logf("cpu %v: %v %v", i, userLoops[i], elapsedCPU[i])
	}
}

func TestCombinedWork(t *testing.T) {
	start := time.Now()
	userLoops := UserLoad(5 * time.Second)
	elapsed := time.Now().Sub(start)
	t.Logf("elapsed: %v %v", elapsed, userLoops)

	start2 := time.Now()
	workElapsed := UserWork(userLoops)
	elapsed2 := time.Now().Sub(start2)
	t.Logf("elapsed2: %v", elapsed2)
	d := (float64(elapsed) - float64(elapsed2)) / float64(elapsed)
	t.Logf("delta: %v%%", d*100)
	_ = workElapsed

	t.Logf("%v", bytesize.New(float64(userLoops*4*1024*1024)))
}

func TestPrime(t *testing.T) {
	n, _ := new(big.Int).SetString("65610001", 10)
	if isPrime(n) {
		t.Logf("prime")
	} else {
		t.Logf("not prime")
	}
}

func TestPrimeRunner(t *testing.T) {
	var primes uint64
	for i := uint64(0); i < 1000000; i++ {
		if isPrime(new(big.Int).SetUint64(i)) {
			primes++
		}
	}
	t.Logf("primes found: %v", primes)
}
