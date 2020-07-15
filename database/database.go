package database

import (
	"time"
)

type Database interface {
	Create() error // Create schema. Database is NOT Opened!
	Open() error   // Open database connection and create+upgrade schema
	Close() error  // Close database

	// Insert measurement and return fresh run id
	MeasurementsInsert(*Measurements) (uint64, error)

	MeminfoInsert(*Meminfo2) error // Insert Meminfo into the database
}

const (
	Name    = "performancedata"
	Version = 1
)

var (
	CreateFormat  = "CREATE DATABASE %v;"
	SelectVersion = "SELECT * FROM version LIMIT 1;"
)

// Measurements is a lookup table that joins site, host and run identifiers so
// that the measurements can be reconstituted.
type Measurements struct {
	RunID  uint64 // Run identifier
	SiteID uint64 // Site identifier
	HostID uint64 // Host Identifier
}

var (
	InsertMeasurements = `
INSERT INTO measurements (
	siteid,
	hostid
)
VALUES(
	:siteid,
	:hostid
)
RETURNING runid;
`
)

// Collection is prefixed after identifiers on every measurement that is being
// stored.
type Collection struct {
	Timestamp int64         // Time of collection
	Duration  time.Duration // Time collection took
}

var (
	SchemaV1 = []string{`
CREATE TABLE version (Version int);
`, `
INSERT INTO version (Version) VALUES (1);
`, `
CREATE TABLE measurements (
	runid			BIGSERIAL UNIQUE NOT NULL,
	siteid			BIGINT NOT NULL,
	hostid			BIGINT NOT NULL,

	PRIMARY KEY		(runid, siteid, hostid),
	UNIQUE			(runid, siteid, hostid)
);
`, `
CREATE TABLE stat (
	runid			BIGSERIAL NOT NULL,
	timestamp		BIGINT NOT NULL,

	duration		BIGINT,

	boottime		BIGSERIAL,

	/* CpuTotal CPUStat */

	/* CPU []CPUStat */

	irqtotal		BIGSERIAL,
	irq			BIGINT[], /* should have been uint64 */
	contextswitches		BIGSERIAL,
	processcreated		BIGSERIAL,
	processesrunning	BIGSERIAL,
	processesblocked	BIGSERIAL,
	softirqtotal		BIGSERIAL,

	/* SoftIRQStat */
	hi			BIGSERIAL,
	timer			BIGSERIAL,
	nettx			BIGSERIAL,
	netrx			BIGSERIAL,
	block			BIGSERIAL,
	blockIopoll		BIGSERIAL,
	tasklet			BIGSERIAL,
	sched			BIGSERIAL,
	hrtimer			BIGSERIAL,
	rcu			BIGSERIAL,

	PRIMARY KEY		(runid, timestamp),
	UNIQUE			(runid, timestamp)
);
`, `
CREATE TABLE cpustat (
	runid			BIGSERIAL NOT NULL,
	cpuid			SMALLINT NOT NULL,
	timestamp		BIGINT NOT NULL,

	duration		BIGINT,

	usert			NUMERIC,
	nice			NUMERIC,
	system			NUMERIC,
	idle			NUMERIC,
	iowait			NUMERIC,
	irq			NUMERIC,
	softirq			NUMERIC,
	steal			NUMERIC,
	guest			NUMERIC,
	guestnice		NUMERIC,

	PRIMARY KEY		(runid, cpuid, timestamp),
	FOREIGN KEY		(runid, timestamp) REFERENCES stat (runid, timestamp),
	UNIQUE			(runid, cpuid, timestamp)
);
`, `
CREATE TABLE meminfo (
	runid			BIGSERIAL NOT NULL,
	timestamp		BIGINT NOT NULL,

	duration		BIGINT,

	memtotal		BIGINT,
	memfree			BIGINT,
	memavailable		BIGINT,
	buffers			BIGINT,
	cached			BIGINT,
	swapcached		BIGINT,
	active			BIGINT,
	inactive		BIGINT,
	activeanon		BIGINT,
	inactiveanon		BIGINT,
	activefile		BIGINT,
	inactivefile		BIGINT,
	unevictable		BIGINT,
	mlocked			BIGINT,
	swaptotal		BIGINT,
	swapfree		BIGINT,
	dirty			BIGINT,
	writeback		BIGINT,
	anonpages		BIGINT,
	mapped			BIGINT,
	shmem			BIGINT,
	slab			BIGINT,
	sreclaimable		BIGINT,
	sunreclaim		BIGINT,
	kernelstack		BIGINT,
	pagetables		BIGINT,
	nfsunstable		BIGINT,
	bounce			BIGINT,
	writebacktmp		BIGINT,
	commitlimit		BIGINT,
	committedas		BIGINT,
	vmalloctotal		BIGINT,
	vmallocused		BIGINT,
	vmallocchunk		BIGINT,
	hardwarecorrupted	BIGINT,
	anonhugepages		BIGINT,
	shmemhugepages		BIGINT,
	shmempmdmapped		BIGINT,
	cmatotal		BIGINT,
	cmafree			BIGINT,
	hugepagestotal		BIGINT,
	hugepagesfree		BIGINT,
	hugepagesrsvd		BIGINT,
	hugepagessurp		BIGINT,
	hugepagesize		BIGINT,
	directmap4k		BIGINT,
	directmap2m		BIGINT,
	directmap1g		BIGINT,

	PRIMARY KEY		(runid, timestamp),
	FOREIGN KEY		(runid) REFERENCES measurements (runid)
);
`, `
CREATE TABLE netdev (
	runid			BIGSERIAL NOT NULL,
	timestamp		BIGINT NOT NULL,
	name			TEXT,

	duration		BIGINT,

	rxbytes			BIGSERIAL,
	rxpackets		BIGSERIAL,
	rxerrors		BIGSERIAL,
	rxdropped		BIGSERIAL,
	rxfifo			BIGSERIAL,
	rxframe			BIGSERIAL,
	rxcompressed		BIGSERIAL,
	rxmulticast		BIGSERIAL,
	txbytes			BIGSERIAL,
	txpackets		BIGSERIAL,
	txerrors		BIGSERIAL,
	txdropped		BIGSERIAL,
	txfifo			BIGSERIAL,
	txcollisions		BIGSERIAL,
	txcarrier		BIGSERIAL,
	txcompressed		BIGSERIAL,

	PRIMARY KEY		(runid, timestamp, name),
	FOREIGN KEY		(runid) REFERENCES measurements (runid)
);
`}
)
