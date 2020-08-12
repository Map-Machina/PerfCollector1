package database

// tps      rtps      wtps      dtps   bread/s   bwrtn/s   bdscd/s
type Diskstat struct {
	RunID uint64 // ID for this measurement

	Timestamp int64 // UNIX timestamp of overall measurement
	Start     int64 // UNIX timestamp of this measurement
	Duration  int64 // Duration of measurement in nano seconds

	Name  string
	Tps   float64
	Rtps  float64
	Wtps  float64
	Dtps  float64
	Bread float64
	Bwrtn float64
	Bdscd float64
}

// InsertDiskstat inserts a Diskstat record into the database.
var (
	InsertDiskstat = `
INSERT INTO diskstat (
	runid,
	timestamp,
	start,
	duration,

	name,
	tps,
	rtps,
	wtps,
	dtps,
	bread,
	bwrtn,
	bdscd
)
VALUES(
	:runid,
	:timestamp,
	:start,
	:duration,

	:name,
	:tps,
	:rtps,
	:wtps,
	:dtps,
	:bread,
	:bwrtn,
	:bdscd
);
`
)
