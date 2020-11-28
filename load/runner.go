package load

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/businessperformancetuning/perfcollector/database"
	"github.com/businessperformancetuning/perfcollector/parser"
)

type LoadError struct {
	err error
}

func (l LoadError) Error() string {
	return l.err.Error()
}

type WorkLoad struct {
	measuredLoad float64
}

type Worker struct {
	actualCores       uint
	virtualCores      uint
	threads           uint
	unitsPerSecond    float64
	measuredFrequency int
	hyperThreading    bool

	wg        sync.WaitGroup
	loadC     chan WorkLoad
	completeC chan int // Return units
}

func (w *Worker) worker(ctx context.Context, id uint) {
	defer w.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return

		case load := <-w.loadC:
			if load.measuredLoad <= 0 {
				w.completeC <- 0 // Do mark complete
				continue
			}
			cpuLoad := load.measuredLoad
			if load.measuredLoad > 100.0 {
				cpuLoad = 100.0
			}
			cpuLoad /= 100

			// Execute load
			replayLoad := math.RoundToEven(cpuLoad *
				w.unitsPerSecond *
				float64(w.measuredFrequency))
			//fmt.Printf("replayLoad %v cl %v us %v fr %v\n",
			//	replayLoad, cpuLoad, w.unitsPerSecond, w.measuredFrequency)
			_ = UserWork(int(replayLoad))

			w.completeC <- int(replayLoad)
		}
	}
}

// workCPUHT is the load runner when the CPU is in hyperthreading mode.
func (w *Worker) workCPUHT(s []database.Stat) int {
	var (
		totalLoad float64
		cpuCount  int
	)
	for k := range s {
		if s[k].CPU == -1 {
			// Skip global CPU
			continue
		}

		// Pretend that all load is created equal.
		total := 100.0 - s[k].Idle
		if total < 0 {
			total = 0
		}
		totalLoad += total
		cpuCount++
	}

	// Distribute work load over all CPU and let hyper threading figure it
	// out.
	workLoad := WorkLoad{measuredLoad: totalLoad / float64(cpuCount)}
	for i := 0; i < cpuCount; i++ {
		w.loadC <- workLoad
	}

	return cpuCount
}

func (w *Worker) workCPUNoHT(s []database.Stat) int {
	var cpuCount int
	for k := range s {
		if s[k].CPU == -1 {
			// Skip global CPU
			continue
		}

		load := 100.0 - s[k].Idle
		if load < 0 {
			load = 0
		}
		w.loadC <- WorkLoad{measuredLoad: load}

		cpuCount++
	}

	return cpuCount
}

func (w *Worker) WorkCPU(s []database.Stat) (int, error) {
	var waits int
	if w.hyperThreading {
		waits = w.workCPUHT(s)
	} else {
		waits = w.workCPUNoHT(s)
	}

	// Mark start
	start := time.Now()

	// Wait for work to complete
	var units int
	for i := 0; i < waits; i++ {
		units += <-w.completeC
	}

	// Mark done
	idle := time.Duration(w.measuredFrequency)*time.Second - time.Now().Sub(start)
	if idle < 0 {
		return units, fmt.Errorf("can't keep up %v", idle)
	} else {
		UserIdle(idle)
	}

	return units, nil
}

func (w *Worker) WaitForExit() {
	w.wg.Wait()
}

type trainError struct {
	err error
}

func (te trainError) Error() string {
	return te.err.Error()
}

func (w *Worker) train(cpuLoad float64) (int, error) {
	if cpuLoad <= 0 || cpuLoad > 100 {
		return 0, fmt.Errorf("invalid load %v", cpuLoad)
	}

	// Short circuit some values.
	if cpuLoad == 0 {
		return 0, nil
	} else if cpuLoad == 100 {
		return int(w.unitsPerSecond), nil
	}

	// Rough CPU usage per core
	replayLoad := math.RoundToEven(cpuLoad / 100 *
		w.unitsPerSecond * float64(w.measuredFrequency))
	//fmt.Printf("replayLoad %v\n", replayLoad)
	// Execute work
	start := time.Now()
	var wg sync.WaitGroup
	for i := uint(0); i < w.virtualCores; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			UserWork(int(replayLoad))
		}()
	}
	wg.Wait()

	idleDuration := time.Duration(w.measuredFrequency)*time.Second - time.Now().Sub(start)
	if idleDuration < 0 {
		return 0, trainError{
			err: fmt.Errorf("took too long %v", idleDuration),
		}
	} else {
		UserIdle(idleDuration)
	}
	return int(replayLoad), nil
}

func (w *Worker) Train() (map[int]int, error) {
	loadPercent := make(map[int]int) // [percent]load
	for i := 1; i < 10; i++ {
		load := 10.0 * float64(i)
		percentage := load
		margin := 0.05 // percent margin
		idle := 100.0 - load
		maxDiff := margin * idle

		loadFound := false
		//fmt.Printf("=== looking for %v\n", load)
		for retry := 0; retry < 5; retry++ {
			// Start work
			cs, err := CPUStat()
			if err != nil {
				return nil, err
			}

			x, err := w.train(load)
			if err != nil {
				var te trainError
				if errors.As(err, &te) {
					load--
					//fmt.Printf("---%v\n", te)
					continue
				} else {
					return nil, err
				}
			}

			// End work
			ce, err := CPUStat()
			if err != nil {
				return nil, err
			}

			s, err := parser.CubeStat(0, 0, 0, 0, &cs, &ce)
			if err != nil {
				return nil, err
			}

			//fmt.Printf("%v < %v < %v\n", idle-maxDiff, s[0].Idle, idle+maxDiff)
			if s[0].Idle > idle-maxDiff && s[0].Idle < idle+maxDiff {
				//fmt.Printf("Within %v%% margin\n", margin*100)
				loadFound = true
				loadPercent[int(percentage)] = x
				break
			} else {
				// expected idle, got s[0].Idle
				if s[0].Idle < idle-maxDiff {
					load--
				} else {
					load++
				}
			}
		}
		if !loadFound {
			return nil, fmt.Errorf("no load found within 5%%")
		}
	}

	return loadPercent, nil
}

func (w *Worker) Train_(cpuLoad float64) {
	if cpuLoad <= 0 || cpuLoad > 100 {
		return
	}

	// Rough CPU usage per core
	replayLoad := math.RoundToEven(cpuLoad / 100 *
		w.unitsPerSecond * float64(w.measuredFrequency))
	//replayLoad = 41
restart:
	//fmt.Printf("%v rl %v us %v mf %v vc %v\n", cpuLoad, replayLoad, w.unitsPerSecond,
	//float64(w.measuredFrequency), float64(w.virtualCores))

	// Start work
	cs, err := CPUStat()
	if err != nil {
		panic(err)
	}

	// Execute work
	start := time.Now()
	var wg sync.WaitGroup
	for i := uint(0); i < w.virtualCores; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			UserWork(int(replayLoad))
		}()
	}
	wg.Wait()
	idleDuration := time.Duration(w.measuredFrequency)*time.Second - time.Now().Sub(start)
	if idleDuration < 0 {
		x := 0.05 * replayLoad
		replayLoad = replayLoad - x
		goto restart
		fmt.Printf(fmt.Sprintf("idleDuration %v", idleDuration))
	} else {
		UserIdle(idleDuration)
	}

	// End work
	ce, err := CPUStat()
	if err != nil {
		panic(err)
	}

	s, err := parser.CubeStat(0, 0, 0, 0, &cs, &ce)
	if err != nil {
		panic(err)
	}

	margin := 0.05 // percent margin
	idle := 100.0 - cpuLoad
	maxDiff := margin * idle
	fmt.Printf("%v < %v < %v\n", idle-maxDiff, s[0].Idle, idle+maxDiff)
	if s[0].Idle > idle-maxDiff && s[0].Idle < idle+maxDiff {
		fmt.Printf("Within %v%% margin\n", margin*100)
	} else {
		// expected idle, got s[0].Idle
		// XXX ad missing case
		if s[0].Idle < idle-maxDiff {
			x := (idle - s[0].Idle) / 100 * replayLoad
			replayLoad = replayLoad - x
			fmt.Printf("new replayLoad: %v %v\n", replayLoad, x)
		} else {
			x := (idle - s[0].Idle) / 100 * replayLoad
			replayLoad = replayLoad + x
			fmt.Printf("new replayLoad: %v %v\n", replayLoad, x)
		}
		goto restart
	}
}

func NewWorkerPool(ctx context.Context, frequency int) (*Worker, error) {
	var (
		w   Worker
		err error
	)

	// Store measured frequency.
	w.measuredFrequency = frequency

	// Figure out how many cores we have
	w.actualCores, w.virtualCores, err = NumCores()
	if err != nil {
		return nil, err
	}
	if w.virtualCores != uint(runtime.GOMAXPROCS(-1)) {
		return nil, LoadError{
			err: fmt.Errorf("virtual cores != MAXPROCS , %v != %v",
				w.virtualCores, runtime.GOMAXPROCS(-1)),
		}
	}

	// Determine if this machine is hyper threading
	if w.virtualCores != 0 && w.actualCores != w.virtualCores {
		w.hyperThreading = true
	}

	w.threads = w.virtualCores / w.actualCores
	if w.virtualCores%w.actualCores != 0 {
		return nil, LoadError{
			err: fmt.Errorf("invalid number of threads %v",
				w.virtualCores%w.actualCores),
		}
	}

	// Calculate units per second based on hyperthreading.
	if w.hyperThreading {
		// Determine units/second while using hyperthreading
		w.unitsPerSecond, err = MeasureUnitsPerSecond(ctx,
			w.virtualCores, 0)
		if err != nil {
			return nil, err
		}
	} else {
		// Determine units/second while using full speed
		w.unitsPerSecond, err = MeasureUnitsPerSecond(ctx,
			w.actualCores, 0)
		if err != nil {
			return nil, err
		}
	}

	// Launch workers that perform load
	// XXX ascertain virtualCores == actualCores when hyperthreading
	w.completeC = make(chan int)
	w.loadC = make(chan WorkLoad)
	for i := uint(0); i < w.virtualCores; i++ {
		w.wg.Add(1)
		go w.worker(ctx, i)
	}

	return &w, nil
}
