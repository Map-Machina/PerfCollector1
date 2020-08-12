package database

type NetDev struct {
	RunID uint64 // ID for this measurement

	Timestamp int64 // UNIX timestamp of overall measurement
	Start     int64 // UNIX timestamp of this measurement
	Duration  int64 // Duration of measurement in nano seconds

	Name         string  // IFACE
	RxPackets    float64 // rxpck/s
	TxPackets    float64 // txpck/s
	RxKBytes     float64 // rxkB/s
	TxKBytes     float64 // txkB/s
	RxCompressed float64 // rxcmp/s
	TxCompressed float64 // txcmp/s
	RxMulticast  float64 // rxmcst/s
	IfUtil       float64 // %ifutil
}

// InsertNetDev inserts a netdev record into the database.
var (
	InsertNetDev = `
INSERT INTO netdev (
	runid,
	timestamp,
	start,
	duration,

	name,
	rxpackets,
	txpackets,
	rxkbytes,
	txkbytes,
	rxcompressed,
	txcompressed,
	rxmulticast,
	ifutil
)
VALUES(
	:runid,
	:timestamp,
	:start,
	:duration,

	:name,
	:rxpackets,
	:txpackets,
	:rxkbytes,
	:txkbytes,
	:rxcompressed,
	:txcompressed,
	:rxmulticast,
	:ifutil
);
`
)
