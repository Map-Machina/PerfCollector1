package database

import "github.com/businessperformancetuning/perfcollector/parser"

// MemInfo2 is a structure that prefixes the parser.MemInfo with the database
// identifiers and collection data. We use anonymous structures in order to
// minimize code churn.
type Meminfo2 struct {
	MeminfoIdentifiers
	Collection
	parser.Meminfo
}

// MeminfoIdentifiers link this meminfo measurement.
type MeminfoIdentifiers struct {
	RunID uint64
}

// InsertMeminfo inserts a memory info record into the database.
var (
	InsertMeminfo2 = `
INSERT INTO meminfo (
	runid,
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
	:runid,
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
