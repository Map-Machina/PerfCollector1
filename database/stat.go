package database

import "github.com/businessperformancetuning/perfcollector/parser"

// MemInfo2 is a structure that prefixes the parser.MemInfo with the database
// identifiers and collection data. We use anonymous structures in order to
// minimize code churn.
type Stat2 struct {
	StatIdentifiers
	Collection
	parser.Stat // We need to remove CPU identifiers internally
}

// MeminfoIdentifiers link this meminfo measurement.
type StatIdentifiers struct {
	RunID uint64
}

type CPUStat2 struct {
	CPUStatIdentifiers
	Collection
	parser.CPUStat
}

type CPUStatIdentifiers struct {
	RunID uint64
	CPUID int
}

// Stat3 represents kernel/system statistics without CPU metrics. Used by the
// database.
type Stat3 struct {
	BootTime uint64
	// CPUTotal CPUStat
	// CPU []CPUStat
	IRQTotal         uint64
	IRQ              []uint64
	ContextSwitches  uint64
	ProcessCreated   uint64
	ProcessesRunning uint64
	ProcessesBlocked uint64
	SoftIRQTotal     uint64
	parser.SoftIRQStat
}

// InsertStat3 inserts a stat record into the database.
var (
	InsertStat3 = `
INSERT INTO stat (
	runid,
	timestamp,
	duration,

	boottime,
	irqtotal,
	irq,
	contextswitches,
	processcreated,
	processesrunning,
	processesblocked,
	softirqtotal,

	/* SoftIRQStat */
	hi,
	timer,
	nettx,
	netrx,
	block,
	blockiopoll,
	tasklet,
	sched,
	hrtimer,
	rcu,
)
VALUES(
	:runid,
	:timestamp,
	:duration,

	:boottime,
	:irqtotal,
	:irq,
	:contextswitches,
	:processcreated,
	:processesrunning,
	:processesblocked,
	:softirqtotal,

	/* SoftIRQStat */
	:hi,
	:timer,
	:nettx,
	:netrx,
	:block,
	:blockiopoll,
	:tasklet,
	:sched,
	:hrtimer,
	:rcu,
);
`

	InsertCPUStat2 = `
INSERT INTO cpustat (
	runid,
	cpuid,
	timestamp,
	duration,

	"user",
	nice,
	system,
	idle,
	iowait,
	irq,
	softirq,
	steal,
	guest,
	guestnice,
VALUES(
	:runid,
	:cpuid,
	:timestamp,
	:duration,

	:"user",
	:nice,
	:system,
	:idle,
	:iowait,
	:irq,
	:softirq,
	:steal,
	:guest,
	:guestnice,
);
`
)
