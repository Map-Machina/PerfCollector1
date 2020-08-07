package database

// 12:50:56        CPU     %user     %nice   %system   %iowait    %steal     %idle
// XXX this isn't 3rd normal form
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

// InsertStat inserts a stat record into the database.
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
)
