// +build linux

package load

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/businessperformancetuning/perfcollector/parser"
	"github.com/businessperformancetuning/perfcollector/util"
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

type workLoad struct {
	measuredLoad      float64
	measuredFrequency int
	unitsPerSecond    float64
}

func worker(ctx context.Context, worker uint, wg *sync.WaitGroup, c chan workLoad) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("exit %v\n", worker)
			return

		case load := <-c:
			if load.measuredLoad <= 0 {
				continue
			}
			// Include everything in time measurement
			replayStart := time.Now()

			// Execute load
			replayLoad := math.RoundToEven(load.measuredLoad *
				load.unitsPerSecond *
				float64(load.measuredFrequency))
			//fmt.Printf("replayload %v: %v\n", worker, replayLoad)
			_ = UserWork(int(replayLoad))
			replayDuration := time.Now().Sub(replayStart)

			// Idle CPU
			idle := time.Duration(load.measuredFrequency)*time.Second -
				replayDuration
			if idle < 0 {
				s := fmt.Sprintf("can't keep up %v", idle)
				fmt.Printf("--- %v\n", s)
			} else {
				UserIdle(idle)
			}
		}
	}
}

func TestEnd2End(t *testing.T) {
	t.Logf("MAXPROCS = %v", runtime.GOMAXPROCS(-1))
	ctx, cancel := context.WithCancel(context.Background())

	// Figure out how many cores we have
	actualCores, virtualCores, err := NumCores()
	if err != nil {
		t.Fatal(err)
	}
	var ht bool // HyperThreading
	if virtualCores != 0 && actualCores != virtualCores {
		ht = true
	}
	threads := virtualCores / actualCores
	if virtualCores%actualCores != 0 {
		t.Fatalf("invalid threads %v", virtualCores%actualCores)
	}
	t.Logf("cores %v virtual %v ht %v threads %v",
		actualCores, virtualCores, ht, threads)

	// Determine units/second while using full speed
	usFull, err := MeasureUnitsPerSecond(ctx, actualCores, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("units/second full speed = %v", usFull)

	// Determine units/second while using hyperthreading
	usHT, err := MeasureUnitsPerSecond(ctx, virtualCores, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("units/second hyperthreading = %v", usHT)

	// Launch workers that perform load
	var wg sync.WaitGroup
	c := make(chan workLoad)
	for i := uint(0); i < virtualCores; i++ {
		wg.Add(1)
		go worker(ctx, i, &wg, c)
	}

	// Measure load => cpu_percentage/duration
	//measuredLoad := []float64{1}
	//measuredLoad2 := []float64{1}
	measuredLoad := []float64{0.25, 0.33, 0.10, 0.80, 1.00, 0.55}
	measuredLoad2 := []float64{0.50, 0.11, 0.20, 0.10, 1.00, 0.45}
	measuredFrequency := 1 // Every 5 seconds

	// Replay load
	loopStart := time.Now()
	for k := range measuredLoad {
		// Take CPU measurement
		cpuStart, err := util.Measure("/proc/stat")
		if err != nil {
			t.Fatal(err)
		}
		cs, err := parser.ProcessStat(cpuStart)
		if err != nil {
			t.Fatal(err)
		}

		// Send of work load
		if ht {
			// Since we are hyperthreading we have to half the
			// workload for any work > 50% load.
			for i := uint(0); i < virtualCores; i += threads {
				// Combine workload
				// XXX make for loop instead of assuming 1 thread!
				load := measuredLoad[k] + measuredLoad2[k]
				l := load / 2
				t.Logf("%v small load %v %v -> %v",
					i, measuredLoad[k], measuredLoad2[k], l)
				c <- workLoad{
					measuredLoad:      l, //measuredLoad[k],
					measuredFrequency: measuredFrequency,
					unitsPerSecond:    usHT,
				}
				c <- workLoad{
					measuredLoad:      l, //measuredLoad2[k],
					measuredFrequency: measuredFrequency,
					unitsPerSecond:    usHT,
				}
			}
		} else {
			// virtualCores == actualCores
			for i := uint(0); i < actualCores; i++ {
				c <- workLoad{
					measuredLoad:      measuredLoad[k],
					measuredFrequency: measuredFrequency,
					unitsPerSecond:    usFull,
				}
			}
		}

		// XXX don't sleep, wait for signal
		time.Sleep(time.Duration(measuredFrequency) * time.Second)

		// Take CPU measurement
		cpuEnd, err := util.Measure("/proc/stat")
		if err != nil {
			t.Fatal(err)
		}
		ce, err := parser.ProcessStat(cpuEnd)
		if err != nil {
			t.Fatal(err)
		}

		// Cube data
		stat, err := parser.CubeStat(0, 0, 0, 0, &cs, &ce)
		if err != nil {
			t.Fatal(err)
		}

		//t.Logf("load %v replay units executed %v in %v idle %v",
		//	measuredLoad[k], int(replayLoad), replayDuration, idle)
		for k := range stat {
			t.Logf("cpu %v user %.2f nice %.2f system %.2f io %.2f steal %.2f idle %.2f",
				stat[k].CPU,
				stat[k].UserT,
				stat[k].Nice,
				stat[k].System,
				stat[k].IOWait,
				stat[k].Steal,
				stat[k].Idle)
		}
		t.Logf("==========================================================")
	}
	loopDuration := time.Now().Sub(loopStart)
	t.Logf("ran %v in %v", len(measuredLoad), loopDuration)

	cancel()
	wg.Wait()
}
