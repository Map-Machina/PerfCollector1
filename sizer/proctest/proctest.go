package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	userHZ = 100.0
)

// CPUStat shows how much time the cpu spend in various stages.
type CPUStat struct {
	User      float64
	Nice      float64
	System    float64
	Idle      float64
	Iowait    float64
	IRQ       float64
	SoftIRQ   float64
	Steal     float64
	Guest     float64
	GuestNice float64
}

// SoftIRQStat represent the softirq statistics as exported in the procfs stat file.
// A nice introduction can be found at https://0xax.gitbooks.io/linux-insides/content/interrupts/interrupts-9.html
// It is possible to get per-cpu stats by reading /proc/softirqs
type SoftIRQStat struct {
	Hi          uint64
	Timer       uint64
	NetTx       uint64
	NetRx       uint64
	Block       uint64
	BlockIoPoll uint64
	Tasklet     uint64
	Sched       uint64
	Hrtimer     uint64
	Rcu         uint64
}

// Stat represents kernel/system statistics.
type Stat struct {
	// Boot time in seconds since the Epoch.
	BootTime uint64
	// Summed up cpu statistics.
	CPUTotal CPUStat
	// Per-CPU statistics.
	CPU []CPUStat
	// Number of times interrupts were handled, which contains numbered and unnumbered IRQs.
	IRQTotal uint64
	// Number of times a numbered IRQ was triggered.
	IRQ []uint64
	// Number of times a context switch happened.
	ContextSwitches uint64
	// Number of times a process was created.
	ProcessCreated uint64
	// Number of processes currently running.
	ProcessesRunning uint64
	// Number of processes currently blocked (waiting for IO).
	ProcessesBlocked uint64
	// Number of times a softirq was scheduled.
	SoftIRQTotal uint64
	// Detailed softirq statistics.
	SoftIRQ SoftIRQStat
}

// Parse a cpu statistics line and returns the CPUStat struct plus the cpu id (or -1 for the overall sum).
func parseCPUStat(line string) (CPUStat, int64, error) {
	cpuStat := CPUStat{}
	var cpu string

	count, err := fmt.Sscanf(line, "%s %f %f %f %f %f %f %f %f %f %f",
		&cpu,
		&cpuStat.User, &cpuStat.Nice, &cpuStat.System, &cpuStat.Idle,
		&cpuStat.Iowait, &cpuStat.IRQ, &cpuStat.SoftIRQ, &cpuStat.Steal,
		&cpuStat.Guest, &cpuStat.GuestNice)

	if err != nil && err != io.EOF {
		return CPUStat{}, -1, fmt.Errorf("couldn't parse %s (cpu): %s", line, err)
	}
	if count == 0 {
		return CPUStat{}, -1, fmt.Errorf("couldn't parse %s (cpu): 0 elements parsed", line)
	}

	cpuStat.User /= userHZ
	cpuStat.Nice /= userHZ
	cpuStat.System /= userHZ
	cpuStat.Idle /= userHZ
	cpuStat.Iowait /= userHZ
	cpuStat.IRQ /= userHZ
	cpuStat.SoftIRQ /= userHZ
	cpuStat.Steal /= userHZ
	cpuStat.Guest /= userHZ
	cpuStat.GuestNice /= userHZ

	if cpu == "cpu" {
		return cpuStat, -1, nil
	}

	cpuID, err := strconv.ParseInt(cpu[3:], 10, 64)
	if err != nil {
		return CPUStat{}, -1, fmt.Errorf("couldn't parse %s (cpu/cpuid): %s", line, err)
	}

	return cpuStat, cpuID, nil
}

// Parse a softirq line.
func parseSoftIRQStat(line string) (SoftIRQStat, uint64, error) {
	softIRQStat := SoftIRQStat{}
	var total uint64
	var prefix string

	_, err := fmt.Sscanf(line, "%s %d %d %d %d %d %d %d %d %d %d %d",
		&prefix, &total,
		&softIRQStat.Hi, &softIRQStat.Timer, &softIRQStat.NetTx, &softIRQStat.NetRx,
		&softIRQStat.Block, &softIRQStat.BlockIoPoll,
		&softIRQStat.Tasklet, &softIRQStat.Sched,
		&softIRQStat.Hrtimer, &softIRQStat.Rcu)

	if err != nil {
		return SoftIRQStat{}, 0, fmt.Errorf("couldn't parse %s (softirq): %s", line, err)
	}

	return softIRQStat, total, nil
}

// Stat returns information about current cpu/process statistics.
// See https://www.kernel.org/doc/Documentation/filesystems/proc.txt
func GetStat() (Stat, error) {
	fileName := "/proc/stat"
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		return Stat{}, err
	}

	stat := Stat{}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(scanner.Text())
		// require at least <key> <value>
		if len(parts) < 2 {
			continue
		}
		switch {
		case parts[0] == "btime":
			if stat.BootTime, err = strconv.ParseUint(parts[1], 10, 64); err != nil {
				return Stat{}, fmt.Errorf("couldn't parse %s (btime): %s", parts[1], err)
			}
		case parts[0] == "intr":
			if stat.IRQTotal, err = strconv.ParseUint(parts[1], 10, 64); err != nil {
				return Stat{}, fmt.Errorf("couldn't parse %s (intr): %s", parts[1], err)
			}
			numberedIRQs := parts[2:]
			stat.IRQ = make([]uint64, len(numberedIRQs))
			for i, count := range numberedIRQs {
				if stat.IRQ[i], err = strconv.ParseUint(count, 10, 64); err != nil {
					return Stat{}, fmt.Errorf("couldn't parse %s (intr%d): %s", count, i, err)
				}
			}
		case parts[0] == "ctxt":
			if stat.ContextSwitches, err = strconv.ParseUint(parts[1], 10, 64); err != nil {
				return Stat{}, fmt.Errorf("couldn't parse %s (ctxt): %s", parts[1], err)
			}
		case parts[0] == "processes":
			if stat.ProcessCreated, err = strconv.ParseUint(parts[1], 10, 64); err != nil {
				return Stat{}, fmt.Errorf("couldn't parse %s (processes): %s", parts[1], err)
			}
		case parts[0] == "procs_running":
			if stat.ProcessesRunning, err = strconv.ParseUint(parts[1], 10, 64); err != nil {
				return Stat{}, fmt.Errorf("couldn't parse %s (procs_running): %s", parts[1], err)
			}
		case parts[0] == "procs_blocked":
			if stat.ProcessesBlocked, err = strconv.ParseUint(parts[1], 10, 64); err != nil {
				return Stat{}, fmt.Errorf("couldn't parse %s (procs_blocked): %s", parts[1], err)
			}
		case parts[0] == "softirq":
			softIRQStats, total, err := parseSoftIRQStat(line)
			if err != nil {
				return Stat{}, err
			}
			stat.SoftIRQTotal = total
			stat.SoftIRQ = softIRQStats
		case strings.HasPrefix(parts[0], "cpu"):
			cpuStat, cpuID, err := parseCPUStat(line)
			if err != nil {
				return Stat{}, err
			}
			if cpuID == -1 {
				stat.CPUTotal = cpuStat
			} else {
				for int64(len(stat.CPU)) <= cpuID {
					stat.CPU = append(stat.CPU, CPUStat{})
				}
				stat.CPU[cpuID] = cpuStat
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return Stat{}, fmt.Errorf("couldn't parse %s: %s", fileName, err)
	}

	return stat, nil
}

func getAllBusy(t CPUStat) (float64, float64) {
	busy := t.User + t.System + t.Nice + t.Iowait + t.IRQ +
		t.SoftIRQ + t.Steal
	return busy + t.Idle, busy
}

func calculateBusy(t1, t2 CPUStat) float64 {
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

func calculateSystem(t1, t2 CPUStat) float64 {
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

func calculateIO(t1, t2 CPUStat) float64 {
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

func cookStat(current Stat, previous *Stat) error {
	if previous == nil {
		return nil
	}

	// tot_jiffies_c = scc->cpu_user + scc->cpu_nice +
	// 		scc->cpu_sys + scc->cpu_idle +
	// 		scc->cpu_iowait + scc->cpu_hardirq +
	// 		scc->cpu_steal + scc->cpu_softirq;
	// tot_jiffies_p = scp->cpu_user + scp->cpu_nice +
	// 		scp->cpu_sys + scp->cpu_idle +
	// 		scp->cpu_iowait + scp->cpu_hardirq +
	// 		scp->cpu_steal + scp->cpu_softirq;

	//return ((scc->cpu_user    + scc->cpu_nice   +
	//	 scc->cpu_sys     + scc->cpu_iowait +
	//	 scc->cpu_idle    + scc->cpu_steal  +
	//	 scc->cpu_hardirq + scc->cpu_softirq) -
	//	(scp->cpu_user    + scp->cpu_nice   +
	//	 scp->cpu_sys     + scp->cpu_iowait +
	//	 scp->cpu_idle    + scp->cpu_steal  +
	//	 scp->cpu_hardirq + scp->cpu_softirq) +
	//	 ishift);

	fmt.Printf("%.1f%%\n", calculateBusy(previous.CPUTotal, current.CPUTotal))
	fmt.Printf("system %.1f%% iowait %.1f%%\n",
		calculateSystem(previous.CPUTotal, current.CPUTotal),
		calculateIO(previous.CPUTotal, current.CPUTotal))
	return nil

	c := current.CPUTotal
	cJiffies := c.User + c.Nice + c.System + c.Iowait + c.IRQ +
		c.Steal + c.SoftIRQ
	cAll := cJiffies + c.Idle

	p := previous.CPUTotal
	pJiffies := p.User + p.Nice + p.System + p.Iowait + p.IRQ +
		p.Steal + p.SoftIRQ
	pAll := pJiffies + c.Idle

	percent := (cJiffies - pJiffies) / (cAll - pAll) * 100
	_ = percent
	fmt.Printf("%v %v\n", percent, (c.System - p.System))
	//c := stats.CPUTotal
	//spew.Dump(c)
	return nil
	usertime := c.User - c.Guest
	nicetime := c.Nice - c.GuestNice

	idlealltime := c.Idle + c.Iowait
	systemalltime := c.System + c.IRQ + c.SoftIRQ
	virtalltime := c.Guest + c.GuestNice
	totaltime := usertime + nicetime + systemalltime + idlealltime + c.Steal + virtalltime

	fmt.Printf("total %v v %v s %v i %v n %v u %v\n",
		totaltime,
		virtalltime,
		systemalltime,
		idlealltime,
		nicetime,
		usertime)
	fmt.Printf("user %.2f system %.2f nice %.2f idle %.2f %.2f\n",
		usertime/totaltime*100,
		systemalltime/totaltime*100,
		nicetime/totaltime*100,
		idlealltime/totaltime*100,
		(totaltime-idlealltime)/totaltime*100)

	return nil
}

func _main() error {
	ticker := time.NewTicker(time.Second)
	//fs, err := procfs.NewDefaultFS()
	//if err != nil {
	//	return err
	//}

	var previous *Stat
	for {
		select {
		//case <-done:
		//	return nil
		case t := <-ticker.C:
			//x := time.Now()
			x, err := GetStat()
			if err != nil {
				return err
			}
			//fmt.Printf("%v %v\n", x.CPUTotal.System, spew.Sdump(x.CPUTotal)) //.System)
			_ = t
			//mem, err := fs.Meminfo()
			//delta := time.Now().Sub(x)
			//fmt.Println(delta)
			//_ = stats
			//_ = mem
			err = cookStat(x, previous)
			if err != nil {
				return nil
			}
			//fmt.Printf("CPU: user %v system %v idle %v\n",
			//	stats.CPUTotal.User,
			//	stats.CPUTotal.System,
			//	stats.CPUTotal.Idle)
			//_ = t
			//if previous != nil {
			//	cS := x.CPUTotal.System
			//	pS := previous.CPUTotal.System
			//	fmt.Printf("%v\n", cS-pS)
			//	//fmt.Printf("%v %v\n", previous.CPU[0].System, stat.CPU[0].System)
			//}
			previous = &x
		}
	}

	return nil
}

func main() {
	//var client = flag.Bool("client", false, "help message for flagname")
	//flag.Parse()

	if err := _main(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
