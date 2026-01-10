package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/businessperformancetuning/perfcollector/database"
	"github.com/jmoiron/sqlx"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

var (
	dbName    = getEnv("PERFCOLLECTOR_TEST_DBNAME", "testdb12345")
	dbUser    = getEnv("PERFCOLLECTOR_TEST_USER", "postgres")
	dbHost    = getEnv("PERFCOLLECTOR_TEST_HOST", "localhost")
	dbSSL     = getEnv("PERFCOLLECTOR_TEST_SSLMODE", "disable")
	createURI = fmt.Sprintf("user=%s dbname=postgres host=%s sslmode=%s", dbUser, dbHost, dbSSL)
	openURI   = fmt.Sprintf("user=%s dbname=%s host=%s sslmode=%s", dbUser, dbName, dbHost, dbSSL)
)

var skipTests = false

func init() {
	// Try to connect to PostgreSQL, skip tests if unavailable
	db, err := sqlx.Open("postgres", createURI)
	if err != nil {
		skipTests = true
		return
	}
	if err := db.Ping(); err != nil {
		skipTests = true
		db.Close()
		return
	}

	// Drop test database if it exists
	_, _ = db.Exec("DROP DATABASE IF EXISTS " + dbName + ";")
	db.Close()
}

func skipIfNoPostgres(t *testing.T) {
	t.Helper()
	if skipTests {
		t.Skip("PostgreSQL not available, skipping test")
	}
}

func TestPostgres(t *testing.T) {
	skipIfNoPostgres(t)

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
	defer db.Close()

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

	// Test SELECT methods
	t.Run("StatSelect", func(t *testing.T) {
		stats, err := db.StatSelect(ctx, runId)
		if err != nil {
			t.Fatal(err)
		}
		if len(stats) != 5 {
			t.Fatalf("expected 5 stats, got %d", len(stats))
		}
		for i, stat := range stats {
			if stat.RunID != runId {
				t.Errorf("stat[%d].RunID = %d, want %d", i, stat.RunID, runId)
			}
			if stat.CPU != i {
				t.Errorf("stat[%d].CPU = %d, want %d", i, stat.CPU, i)
			}
		}
	})

	t.Run("MeminfoSelect", func(t *testing.T) {
		meminfos, err := db.MeminfoSelect(ctx, runId)
		if err != nil {
			t.Fatal(err)
		}
		if len(meminfos) != 1 {
			t.Fatalf("expected 1 meminfo, got %d", len(meminfos))
		}
		if meminfos[0].RunID != runId {
			t.Errorf("meminfo.RunID = %d, want %d", meminfos[0].RunID, runId)
		}
		if meminfos[0].MemFree != 54321 {
			t.Errorf("meminfo.MemFree = %d, want 54321", meminfos[0].MemFree)
		}
	})

	t.Run("NetDevSelect", func(t *testing.T) {
		netdevs, err := db.NetDevSelect(ctx, runId)
		if err != nil {
			t.Fatal(err)
		}
		if len(netdevs) != 5 {
			t.Fatalf("expected 5 netdevs, got %d", len(netdevs))
		}
		for i, nd := range netdevs {
			if nd.RunID != runId {
				t.Errorf("netdev[%d].RunID = %d, want %d", i, nd.RunID, runId)
			}
		}
	})

	t.Run("DiskstatSelect", func(t *testing.T) {
		diskstats, err := db.DiskstatSelect(ctx, runId)
		if err != nil {
			t.Fatal(err)
		}
		if len(diskstats) != 5 {
			t.Fatalf("expected 5 diskstats, got %d", len(diskstats))
		}
		for i, ds := range diskstats {
			if ds.RunID != runId {
				t.Errorf("diskstat[%d].RunID = %d, want %d", i, ds.RunID, runId)
			}
		}
	})

	t.Run("MeasurementsSelect", func(t *testing.T) {
		measurements, err := db.MeasurementsSelect(ctx, runId)
		if err != nil {
			t.Fatal(err)
		}
		if measurements.RunID != runId {
			t.Errorf("measurements.RunID = %d, want %d", measurements.RunID, runId)
		}
		if measurements.SiteID != 1 {
			t.Errorf("measurements.SiteID = %d, want 1", measurements.SiteID)
		}
		if measurements.HostID != 2 {
			t.Errorf("measurements.HostID = %d, want 2", measurements.HostID)
		}
	})

	t.Run("ListRuns", func(t *testing.T) {
		runs, err := db.ListRuns(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(runs) != 2 {
			t.Fatalf("expected 2 runs, got %d", len(runs))
		}
		// Runs should be ordered by runid
		if runs[0].RunID != 1 {
			t.Errorf("runs[0].RunID = %d, want 1", runs[0].RunID)
		}
		if runs[1].RunID != 2 {
			t.Errorf("runs[1].RunID = %d, want 2", runs[1].RunID)
		}
	})
}
