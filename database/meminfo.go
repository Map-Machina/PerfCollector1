package database

type Meminfo struct {
	RunID uint64 // ID for this measurement

	Timestamp int64 // UNIX timestamp of overall measurement
	Start     int64 // UNIX timestamp of this measurement
	Duration  int64 // Duration of measurement in nano seconds

	MemFree       uint64  // kbmemfree
	MemAvailable  uint64  // kbavail
	MemUsed       uint64  // kbmemused
	PercentUsed   float64 // %memused
	Buffers       uint64  // kbbuffers
	Cached        uint64  // kbcached
	Commit        uint64  // kbcommit
	PercentCommit float64 // %commit
	Active        uint64  // kbactive
	Inactive      uint64  // kbinactive
	Dirty         uint64  // kbdirty
}

// SQL queries for meminfo table.
var (
	InsertMeminfo = `
INSERT INTO meminfo (
	runid,
	timestamp,
	start,
	duration,

	memfree,
	memavailable,
	memused,
	percentused,
	buffers,
	cached,
	commit,
	percentcommit,
	active,
	inactive,
	dirty
)
VALUES(
	:runid,
	:timestamp,
	:start,
	:duration,

	:memfree,
	:memavailable,
	:memused,
	:percentused,
	:buffers,
	:cached,
	:commit,
	:percentcommit,
	:active,
	:inactive,
	:dirty
);
`
	SelectMeminfoByRunID = `
SELECT runid, timestamp, start, duration, memfree, memavailable, memused, percentused,
       buffers, cached, commit, percentcommit, active, inactive, dirty
FROM meminfo
WHERE runid = $1
ORDER BY timestamp;
`
)
