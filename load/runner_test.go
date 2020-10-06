package load

import (
	"context"
	"testing"

	"github.com/businessperformancetuning/perfcollector/database"
	"github.com/businessperformancetuning/perfcollector/parser"
)

func TestRunner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w, err := NewWorkerPool(ctx, 5) // 5 second measurements
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

		cs, err := CPUStat()
		if err != nil {
			t.Fatal(err)
		}

		units, err := w.WorkCPU(work)
		if err != nil {
			t.Fatal(err)
		}

		ce, err := CPUStat()
		if err != nil {
			t.Fatal(err)
		}

		// Cube data
		stat, err := parser.CubeStat(0, 0, 0, 0, &cs, &ce)
		if err != nil {
			t.Fatal(err)
		}

		t.Logf("Executed units: %v idle %v", units, stat[0].Idle)

		// Calculate difference
		idle := tests[k].idle
		maxDiff := 0.1 * idle
		t.Logf("%v < %v < %v", idle-maxDiff, stat[0].Idle, idle+maxDiff)
		if stat[0].Idle > idle-maxDiff && stat[0].Idle < idle+maxDiff {
			t.Logf("Within 10%% margin")
		} else {
			t.Logf("outside 10%% margin")
		}
	}

	cancel()
	w.WaitForExit()
}
