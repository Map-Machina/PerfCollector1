// +build linux

package load

import (
	"context"
	"testing"

	"github.com/businessperformancetuning/perfcollector/database"
	"github.com/businessperformancetuning/perfcollector/parser"
)

func TestTrain(t *testing.T) {
	ctx, _ := context.WithCancel(context.Background())
	w, err := NewWorkerPool(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}

	loads, err := w.Train()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("loads: %v", loads)

	//for i := 1; i < 10; i++ {
	//	//w.Train(90.0)
	//	//if err != nil {
	//	//	t.Fatal(err)
	//	//}

	//	w.Train(10.0 * float64(i))
	//	if err != nil {
	//		t.Fatal(err)
	//	}
	//}
}

func TestRunner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w, err := NewWorkerPool(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}

	type test struct {
		name string
		idle float64
	}
	tests := []test{
		{
			name: "10%",
			idle: 10.0,
		},
		{
			name: "20%",
			idle: 20.0,
		},
		{
			name: "30%",
			idle: 30.0,
		},
		{
			name: "40%",
			idle: 40.0,
		},
		{
			name: "50%",
			idle: 50.0,
		},
		{
			name: "60%",
			idle: 60.0,
		},
		{
			name: "70%",
			idle: 70.0,
		},
		{
			name: "80%",
			idle: 80.0,
		},
		{
			name: "90%",
			idle: 90.0,
		},
		{
			name: "100%",
			idle: 100.0,
		},
	}

	for k := range tests {
		t.Logf("Executing test: %v", tests[k].name)

		work := make([]database.Stat, 0, w.virtualCores)
		for i := uint(0); i < w.virtualCores; i++ {
			work = append(work, database.Stat{Idle: tests[k].idle})
		}

	restart:
		cs, err := CPUStat()
		if err != nil {
			t.Fatal(err)
		}

		units, err := w.WorkCPU(work)
		if err != nil {
			t.Fatal(err)
		}
		_ = units

		ce, err := CPUStat()
		if err != nil {
			t.Fatal(err)
		}

		//currentUnits := float64(units)

		// Cube data
		stat, err := parser.CubeStat(0, 0, 0, 0, &cs, &ce)
		if err != nil {
			t.Fatal(err)
		}

		//t.Logf("Executed units: %v idle %v", units, stat[0].Idle)

		// Calculate difference
		margin := 0.05 // percent margin
		idle := tests[k].idle
		maxDiff := margin * idle
		//t.Logf("%v < %v < %v", idle-maxDiff, stat[0].Idle, idle+maxDiff)
		if stat[0].Idle > idle-maxDiff && stat[0].Idle < idle+maxDiff {
			t.Logf("Within %v%% margin", margin*100)
		} else {
			//t.Logf("Outside %v%% margin %v", margin*100,
			//	stat.StdDev(stat[0].Idle, []float64{prevUnits, currentUnits}))
			if stat[0].Idle < idle {
				for kk := range work {
					work[kk].Idle += work[kk].Idle * 0.05
					//t.Logf("+New idle: %v", work[kk].Idle)
				}
			} else {
				for kk := range work {
					work[kk].Idle -= work[kk].Idle * 0.05
					//t.Logf("-New idle: %v", work[kk].Idle)
				}
			}
			goto restart
		}
	}

	cancel()
	w.WaitForExit()
}
