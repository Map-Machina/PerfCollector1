package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/businessperformancetuning/perfcollector/database"
	"github.com/jmoiron/sqlx"
)

const (
	dbName    = "testdb12345"
	createURI = "user=marco dbname=postgres host=/tmp/"
	openURI   = "user=marco dbname=" + dbName + " host=/tmp/"
)

func init() {
	// Drop test database.
	db, err := sqlx.Open("postgres", createURI)
	if err != nil {
		panic(err)
	}
	_, err = db.Query("DROP DATABASE IF EXISTS " + dbName + ";")
	if err != nil {
		panic(err)
	}
	db.Close()
}

func TestPostgress(t *testing.T) {
	ctx := context.TODO()
	db, err := New(dbName, createURI)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Create()
	if err != nil {
		t.Fatal(err)
	}
	err = db.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Open database
	db, err = New(dbName, openURI)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(openURI)
	err = db.Open()
	if err != nil {
		t.Fatal(err)
	}

	// Insert Measurements
	m := database.Measurements{
		SiteID: 1,
		HostID: 2,
	}
	runId, err := db.MeasurementsInsert(ctx, &m)
	if err != nil {
		t.Fatal(err)
	}
	if runId != 1 {
		t.Fatalf("got %v", runId)
	}
	runId, err = db.MeasurementsInsert(ctx, &m)
	if err != nil {
		t.Fatal(err)
	}
	if runId != 2 {
		t.Fatalf("got %v", runId)
	}

	// Insert 5 records into stat
	ts := time.Now()
	s := make([]database.Stat, 0, 5)
	for i := 0; i < 5; i++ {
		s = append(s, database.Stat{
			RunID:     runId,
			Timestamp: ts.Unix(),
			Start:     ts.Add(time.Duration(i+1) * time.Microsecond).Unix(),
			Duration:  1234,

			CPU:    i,
			UserT:  float64(i),
			Nice:   float64(i),
			System: float64(i),
			IOWait: float64(i),
			Steal:  float64(i),
			Idle:   float64(i),
		})
	}
	err = db.StatInsert(ctx, s)
	if err != nil {
		t.Fatal(err)
	}

	// Insert meminfo
	mi := database.Meminfo{
		RunID:     runId,
		Timestamp: ts.Unix(),
		Start:     ts.Add(time.Duration(time.Microsecond)).Unix(),
		Duration:  1234,

		MemFree:       54321,
		MemAvailable:  54321,
		MemUsed:       54321,
		PercentUsed:   0.12,
		Buffers:       54321,
		Cached:        54321,
		Commit:        54321,
		PercentCommit: 0.98,
		Active:        54321,
		Inactive:      54321,
		Dirty:         54321,
	}
	err = db.MeminfoInsert(ctx, &mi)
	if err != nil {
		t.Fatal(err)
	}

	// Insert netdev
	nd := make([]database.NetDev, 0, 5)
	for i := 0; i < 5; i++ {
		nd = append(nd, database.NetDev{
			RunID:     runId,
			Timestamp: ts.Unix(),
			Start:     ts.Add(time.Duration(time.Microsecond)).Unix(),
			Duration:  1234,

			Name:         fmt.Sprintf("eno%v", i),
			RxPackets:    12.34,
			TxPackets:    35.34,
			RxKBytes:     36.34,
			TxKBytes:     37.34,
			RxCompressed: 38.34,
			TxCompressed: 39.34,
			RxMulticast:  40.34,
			IfUtil:       0.99,
		})
	}
	err = db.NetDevInsert(ctx, nd)
	if err != nil {
		t.Fatal(err)
	}

	// Insert Diskstat
	ds := make([]database.Diskstat, 0, 5)
	for i := 0; i < 5; i++ {
		ds = append(ds, database.Diskstat{
			RunID:     runId,
			Timestamp: ts.Unix(),
			Start:     ts.Add(time.Duration(time.Microsecond)).Unix(),
			Duration:  1234,

			Name:  fmt.Sprintf("sda%v", i),
			Tps:   12.34,
			Rtps:  35.34,
			Wtps:  36.34,
			Dtps:  37.34,
			Bread: 38.34,
			Bwrtn: 39.34,
			Bdscd: 40.34,
		})
	}
	err = db.DiskstatInsert(ctx, ds)
	if err != nil {
		t.Fatal(err)
	}
}
