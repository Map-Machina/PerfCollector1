package load

import (
	"sync"
	"testing"
	"time"
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

func TestCombinedWork(t *testing.T) {
	start := time.Now()
	userLoops := userLoad(5 * time.Second)
	elapsed := time.Now().Sub(start)
	t.Logf("elapsed: %v %v", elapsed, userLoops)

	start2 := time.Now()
	workElapsed := UserWork(userLoops)
	elapsed2 := time.Now().Sub(start2)
	t.Logf("delta: %v", float64((elapsed-elapsed2))/float64(elapsed))
	_ = workElapsed
}
