package database

// Stat is the result of cubin data that is being stored in the database. The
// unique key is RunID, Timestamp and CPU.
type Stat struct {
	RunID uint64 // ID for this measurement

	Timestamp int64 // UNIX timestamp of overall measurement
	Start     int64 // UNIX timestamp of this measurement
	Duration  int64 // Duration of measurement in nano seconds

	CPU    int
	UserT  float64
	Nice   float64
	System float64
	IOWait float64
	Steal  float64
	Idle   float64
}

// SQL queries for stat table.
var (
	InsertStat = `
INSERT INTO stat (
	runid,
	timestamp,
	start,
	duration,

	cpu,
	usert,
	nice,
	system,
	iowait,
	steal,
	idle
)
VALUES(
	:runid,
	:timestamp,
	:start,
	:duration,

	:cpu,
	:usert,
	:nice,
	:system,
	:iowait,
	:steal,
	:idle
);
`
	SelectStatByRunID = `
SELECT runid, timestamp, start, duration, cpu, usert, nice, system, iowait, steal, idle
FROM stat
WHERE runid = $1
ORDER BY timestamp, cpu;
`
)
