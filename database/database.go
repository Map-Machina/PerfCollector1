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

	StatInsert(*Stat) error // Insert stat record.
}

const (
	Name    = "performancedata"
	Version = 1
)

var (
	CreateFormat  = "CREATE DATABASE %v;"
	SelectVersion = "SELECT * FROM version LIMIT 1;"
	// 12:50:56        CPU     %user     %nice   %system   %iowait    %steal     %idle
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
	Start			BIGINT NOT NULL,
	Duration		BIGINT NOT NULL,

	cpu			SMALLINT NOT NULL,
	usert			NUMERIC,
	nice			NUMERIC,
	system			NUMERIC,
	iowait			NUMERIC,
	steal			NUMERIC,
	idle			NUMERIC,

	PRIMARY KEY		(runid, timestamp, cpu),
	UNIQUE			(runid, timestamp, cpu)
);
`}
)
