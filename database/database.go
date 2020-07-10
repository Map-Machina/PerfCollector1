package database

import (
	"time"

	"github.com/businessperformancetuning/perfcollector/parser"
)

type Database interface {
	Create() error // Create schema. Database is NOT Opened!
	Open() error   // Open database connection and create+upgrade schema
	Close() error  // Close database

	MeminfoInsert(*Meminfo2) error // Insert Meminfo into the database
}

const (
	Name    = "performancedata"
	Version = 1
)

var (
	Create        = "CREATE DATABASE " + Name + ";"
	SelectVersion = "SELECT * FROM version LIMIT 1;"
)

// MemInfo2 is a structure that prefixes the parser.MemInfo with the database
// identifiers and collection data. We use anonymous structures in order to
// minimize code churn.
type Meminfo2 struct {
	MemInfoIdetifiers
	Collection
	parser.Meminfo
}

type MemInfoIdetifiers struct {
	MemId uint
}

type Collection struct {
	Timestamp time.Time     // Time of collection
	Duration  time.Duration // Time collection took
}

// InsertMeminfo inserts a memory info record into the database.
var (
	InsertMeminfo2 = `
INSERT INTO meminfo (
	memid,
	timestamp,
	duration,

	memtotal,
	memfree,
	memavailable,
	buffers,
	cached,
	swapcached,
	active,
	inactive,
	activeanon,
	inactiveanon,
	activefile,
	inactivefile,
	unevictable,
	mlocked,
	swaptotal,
	swapfree,
	dirty,
	writeback,
	anonpages,
	mapped,
	shmem,
	slab,
	sreclaimable,
	sunreclaim,
	kernelstack,
	pagetables,
	nfsunstable,
	bounce,
	writebacktmp,
	commitlimit,
	committedas,
	vmalloctotal,
	vmallocused,
	vmallocchunk,
	hardwarecorrupted,
	anonhugepages,
	shmemhugepages,
	shmempmdmapped,
	cmatotal,
	cmafree,
	hugepagestotal,
	hugepagesfree,
	hugepagesrsvd,
	hugepagessurp,
	hugepagesize,
	directmap4k,
	directmap2m,
	directmap1g)
VALUES(
	:memid,
	:timestamp,
	:duration,

	:memtotal,
	:memfree,
	:memavailable,
	:buffers,
	:cached,
	:swapcached,
	:active,
	:inactive,
	:activeanon,
	:inactiveanon,
	:activefile,
	:inactivefile,
	:unevictable,
	:mlocked,
	:swaptotal,
	:swapfree,
	:dirty,
	:writeback,
	:anonpages,
	:mapped,
	:shmem,
	:slab,
	:sreclaimable,
	:sunreclaim,
	:kernelstack,
	:pagetables,
	:nfsunstable,
	:bounce,
	:writebacktmp,
	:commitlimit,
	:committedas,
	:vmalloctotal,
	:vmallocused,
	:vmallocchunk,
	:hardwarecorrupted,
	:anonhugepages,
	:shmemhugepages,
	:shmempmdmapped,
	:cmatotal,
	:cmafree,
	:hugepagestotal,
	:hugepagesfree,
	:hugepagesrsvd,
	:hugepagessurp,
	:hugepagesize,
	:directmap4k,
	:directmap2m,
	:directmap1g);
`
)

var (
	SchemaV1 = []string{`
CREATE TABLE version (Version int);
`, `
INSERT INTO version (Version) VALUES (1);
`, `
CREATE TABLE meminfo (
	memid			BIGINT,
	timestamp		TIMESTAMP,
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
	directmap1g		BIGINT);
`}
)
