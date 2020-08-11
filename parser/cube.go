package parser

import (
	"fmt"
	"math"

	"github.com/businessperformancetuning/perfcollector/database"
)

func getAllBusy(t *CPUStat) (float64, float64) {
	busy := t.User + t.System + t.Nice + t.Iowait + t.IRQ + t.SoftIRQ +
		t.Steal
	return busy + t.Idle, busy
}

func calculateIO(t1, t2 *CPUStat) float64 {
	t1All, t1Busy := getAllBusy(t1)
	t2All, t2Busy := getAllBusy(t2)

	if t2Busy <= t1Busy {
		return 0
	}
	if t2All <= t1All {
		return 100
	}
	return math.Min(100, math.Max(0, (t2.Iowait-t1.Iowait)/(t2All-t1All)*100))
}

func calculateUser(t1, t2 *CPUStat) float64 {
	t1All, t1Busy := getAllBusy(t1)
	t2All, t2Busy := getAllBusy(t2)

	if t2Busy <= t1Busy {
		return 0
	}
	if t2All <= t1All {
		return 100
	}
	return math.Min(100, math.Max(0, (t2.User-t1.User)/(t2All-t1All)*100))
}

func calculateNice(t1, t2 *CPUStat) float64 {
	t1All, t1Busy := getAllBusy(t1)
	t2All, t2Busy := getAllBusy(t2)

	if t2Busy <= t1Busy {
		return 0
	}
	if t2All <= t1All {
		return 100
	}
	return math.Min(100, math.Max(0, (t2.Nice-t1.Nice)/(t2All-t1All)*100))
}

func calculateSystem(t1, t2 *CPUStat) float64 {
	t1All, t1Busy := getAllBusy(t1)
	t2All, t2Busy := getAllBusy(t2)

	if t2Busy <= t1Busy {
		return 0
	}
	if t2All <= t1All {
		return 100
	}
	return math.Min(100, math.Max(0, (t2.System-t1.System)/(t2All-t1All)*100))
}

func calculateSteal(t1, t2 *CPUStat) float64 {
	t1All, t1Busy := getAllBusy(t1)
	t2All, t2Busy := getAllBusy(t2)

	if t2Busy <= t1Busy {
		return 0
	}
	if t2All <= t1All {
		return 100
	}
	return math.Min(100, math.Max(0, (t2.Steal-t1.Steal)/(t2All-t1All)*100))
}

func calculateBusy(t1, t2 *CPUStat) float64 {
	t1All, t1Busy := getAllBusy(t1)
	t2All, t2Busy := getAllBusy(t2)

	if t2Busy <= t1Busy {
		return 0
	}
	if t2All <= t1All {
		return 100
	}
	return math.Min(100, math.Max(0, (t2Busy-t1Busy)/(t2All-t1All)*100))
}

func CubeStat(runID uint64, timestamp, start, duration int64, t1, t2 *Stat) ([]database.Stat, error) {
	if len(t1.CPU) != len(t2.CPU) {
		return nil, fmt.Errorf("invalid CPU length %v %v", len(t1.CPU),
			len(t2.CPU))
	}
	s := make([]database.Stat, len(t1.CPU)+1)

	// Individual CPUs
	for k := range s {
		var cpu1, cpu2 *CPUStat
		idx := k - 1
		if k == 0 {
			cpu1 = &t1.CPUTotal
			cpu2 = &t2.CPUTotal
		} else {
			cpu1 = &t1.CPU[idx]
			cpu2 = &t2.CPU[idx]
		}
		s[k] = database.Stat{
			RunID:     runID,
			Timestamp: timestamp,
			Start:     start,
			Duration:  duration,
			CPU:       idx,
			UserT:     calculateUser(cpu1, cpu2),
			Nice:      calculateNice(cpu1, cpu2),
			System:    calculateSystem(cpu1, cpu2),
			IOWait:    calculateIO(cpu1, cpu2),
			Steal:     calculateSteal(cpu1, cpu2),
			Idle:      100 - calculateBusy(cpu1, cpu2),
		}
	}

	return s, nil
}

func CubeMeminfo(runID uint64, timestamp, start, duration int64, mi *Meminfo) (*database.Meminfo, error) {
	// kbmemfree   kbavail kbmemused  %memused kbbuffers  kbcached  kbcommit   %commit  kbactive   kbinact   kbdirty

	// nousedmem = smc->frmkb + smc->bufkb + smc->camkb + smc->slabkb;
	// if (nousedmem > smc->tlmkb) {
	//	nousedmem = smc->tlmkb;
	// }

	var usedMem uint64
	usedMem = mi.MemFree + mi.Buffers + mi.Cached + mi.Slab
	if usedMem > mi.MemTotal {
		usedMem = mi.MemTotal
	}
	return &database.Meminfo{
		MemFree:      mi.MemFree,
		MemAvailable: mi.MemAvailable,
		MemUsed:      mi.MemTotal - usedMem,
		PercentUsed: float64(mi.MemTotal-usedMem) /
			float64(mi.MemTotal) * 100,
		Buffers: mi.Buffers,
		Cached:  mi.Cached,
		Commit:  mi.CommittedAS,
		PercentCommit: float64(mi.CommittedAS) /
			float64(mi.MemTotal+mi.SwapTotal) * 100,
		Active:   mi.Active,
		Inactive: mi.Inactive,
		Dirty:    mi.Dirty,
	}, nil
}

func svalue(t1, t2, tvi uint64) float64 {
	return (float64(t2) - float64(t1)) / float64(tvi) * 100
}

func CubeNetDev(runID uint64, timestamp, start, duration int64, t1, t2 NetDev, tvi uint64) ([]database.NetDev, error) {
	if len(t1) != len(t2) {
		return nil, fmt.Errorf("inval;id length %v %v",
			len(t1), len(t2))
	}

	dnd := make([]database.NetDev, 0, len(t1))
	for k := range t1 {
		cur, ok := t2[k]
		if !ok {
			return nil, fmt.Errorf("current interface not found: %v",
				k)
		}

		rxBytes := svalue(t1[k].RxBytes, cur.RxBytes, tvi)
		txBytes := svalue(t1[k].TxBytes, cur.TxBytes, tvi)
		dnd = append(dnd, database.NetDev{
			Name:         k,
			RxPackets:    svalue(t1[k].RxPackets, cur.RxPackets, tvi),
			TxPackets:    svalue(t1[k].TxPackets, cur.TxPackets, tvi),
			RxKBytes:     rxBytes / 1024,
			TxKBytes:     txBytes / 1024,
			RxCompressed: svalue(t1[k].RxCompressed, cur.RxCompressed, tvi),
			TxCompressed: svalue(t1[k].TxCompressed, cur.TxCompressed, tvi),
			RxMulticast:  svalue(t1[k].RxMulticast, cur.RxMulticast, tvi),
			IfUtil:       0, // XXX need /sys fields for this
		})
	}

	return dnd, nil
}
