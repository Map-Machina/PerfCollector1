package database

import "github.com/businessperformancetuning/perfcollector/parser"

// NetDev2 is a structure that prefixes the parser.NetDec with the database
// identifiers and collection data. We use anonymous structures in order to
// minimize code churn.
type NetDev2 struct {
	MeminfoIdentifiers
	Collection
	parser.NetDev // This map needs to be deconstructed into separate inserts.
}

// MeminfoIdentifiers link this meminfo measurement.
type NetDevIdentifiers struct {
	RunID uint64
}

// InsertNetDev inserts a netdev record into the database.
var (
	InsertNetDev = `
INSERT INTO netdev (
	runid,
	timestamp,
	name,

	duration,

	rxbytes,
	rxpackets,
	rxerrors,
	rxdropped,
	rxfifo,
	rxframe,
	rxcompressed,
	rxmulticast,
	txbytes,
	txpackets,
	txerrors,
	txdropped,
	txfifo,
	txcollisions,
	txcarrier,
	txcompressed
)
VALUES(
	:runid,
	:timestamp,
	:name,

	:duration,

	:rxbytes,
	:rxpackets,
	:rxerrors,
	:rxdropped,
	:rxfifo,
	:rxframe,
	:rxcompressed,
	:rxmulticast,
	:txbytes,
	:txpackets,
	:txerrors,
	:txdropped,
	:txfifo,
	:txcollisions,
	:txcarrier,
	:txcompressed
);
`
)
