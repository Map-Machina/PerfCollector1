package postgres

import (
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
	runId, err := db.MeasurementsInsert(&m)
	if err != nil {
		t.Fatal(err)
	}
	if runId != 1 {
		t.Fatalf("got %v", runId)
	}
	runId, err = db.MeasurementsInsert(&m)
	if err != nil {
		t.Fatal(err)
	}
	if runId != 2 {
		t.Fatalf("got %v", runId)
	}

	// Insert 5 records into stat
	for i := 0; i < 5; i++ {
		ts := time.Now()
		s := database.Stat{
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
		}
		err = db.StatInsert(&s)
		if err != nil {
			t.Fatal(err)
		}
	}
}
