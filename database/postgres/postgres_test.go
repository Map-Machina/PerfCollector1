package postgres

import (
	"testing"
	"time"

	"github.com/businessperformancetuning/perfcollector/database"
	"github.com/businessperformancetuning/perfcollector/parser"
	"github.com/jmoiron/sqlx"
)

const (
	dbName    = "testdb12345"
	createURI = "user=marco dbname=postgres host=/tmp/"
	openURI   = "user=marco dbname=" + dbName + " host=/tmp/"

	statTest = `
cpu  15384209 0 3709980 4963746738 10552729 794717 729854 0 0 0
cpu0 1932873 0 466047 620084082 1703744 88658 68088 0 0 0
cpu1 1965976 0 493362 620152425 1461306 116509 161395 0 0 0
cpu2 1920308 0 461115 620641490 1209438 89627 44534 0 0 0
cpu3 1890579 0 438615 620930368 979692 89969 44815 0 0 0
cpu4 1893567 0 448503 621013777 897480 85265 38936 0 0 0
cpu5 1939262 0 518277 620580407 967532 134375 229952 0 0 0
cpu6 1928065 0 458330 620986977 870987 88645 41145 0 0 0
cpu7 1913575 0 425726 619357209 2462545 101666 100987 0 0 0
intr 1943097907 7 0 0 0 0 0 0 0 1 4 0 0 0 0 0 0 100000 0 0 0 0 0 0 66 0 0 0 7586489 0 0 0 0 0 0 0 0 0 613 20 146201659 757 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0
ctxt 2833057666
btime 1588264109
processes 14587951
procs_running 1
procs_blocked 0
softirq 1962241580 8 990893179 1900849 147623298 7570974 0 3323706 464829592 381349 345718625
`
)

func init() {
	// Drop test database.
	db, err := sqlx.Open("postgres", createURI)
	if err != nil {
		panic(err)
	}
	_, err = db.Query("DROP DATABASE " + dbName + ";")
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

	// Insert 5 meminfo
	for i := 0; i < 5; i++ {
		mi := database.Meminfo2{
			database.MeminfoIdentifiers{
				RunID: 1,
			},
			database.Collection{
				Timestamp: time.Now().UnixNano(),
				Duration:  time.Second,
			},
			parser.Meminfo{
				MemTotal: 1024 * 1024,
			},
		}
		err = db.MeminfoInsert(&mi)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Test stat
	s, err := parser.ProcessStat([]byte(statTest))
	if err != nil {
		t.Fatal(err)
	}
	s2 := database.Stat2{
		database.StatIdentifiers{
			RunID: 1,
		},
		database.Collection{
			Timestamp: time.Now().UnixNano(),
			Duration:  time.Second,
		},
		s,
	}
	err = db.StatInsert(&s2)
	if err != nil {
		t.Fatal(err)
	}
}
