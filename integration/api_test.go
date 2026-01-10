// Package integration contains integration tests for the PerfCollector system.
// These tests require a PostgreSQL database to be available.
//
// Run with: go test -tags=integration ./integration/...
//
//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/businessperformancetuning/perfcollector/database"
	"github.com/businessperformancetuning/perfcollector/database/postgres"
	"github.com/jmoiron/sqlx"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

var (
	testDBName  = getEnv("INTEGRATION_TEST_DBNAME", "perfcollector_integration_test")
	testDBUser  = getEnv("INTEGRATION_TEST_USER", "postgres")
	testDBHost  = getEnv("INTEGRATION_TEST_HOST", "localhost")
	testDBSSL   = getEnv("INTEGRATION_TEST_SSLMODE", "disable")
	adminURI    = fmt.Sprintf("user=%s dbname=postgres host=%s sslmode=%s", testDBUser, testDBHost, testDBSSL)
	testDBURI   = fmt.Sprintf("user=%s dbname=%s host=%s sslmode=%s", testDBUser, testDBName, testDBHost, testDBSSL)
	skipTests   = false
	testDB      database.Database
)

func TestMain(m *testing.M) {
	// Setup: Create test database
	adminDB, err := sqlx.Open("postgres", adminURI)
	if err != nil {
		fmt.Printf("Failed to connect to PostgreSQL: %v\n", err)
		skipTests = true
		os.Exit(0)
	}

	if err := adminDB.Ping(); err != nil {
		fmt.Printf("PostgreSQL not available: %v\n", err)
		skipTests = true
		adminDB.Close()
		os.Exit(0)
	}

	// Drop and recreate test database
	adminDB.Exec("DROP DATABASE IF EXISTS " + testDBName)
	_, err = adminDB.Exec("CREATE DATABASE " + testDBName)
	if err != nil {
		fmt.Printf("Failed to create test database: %v\n", err)
		adminDB.Close()
		os.Exit(1)
	}
	adminDB.Close()

	// Open connection to test database
	testDB, err = postgres.New(testDBName, testDBURI)
	if err != nil {
		fmt.Printf("Failed to create database connection: %v\n", err)
		os.Exit(1)
	}

	if err := testDB.Open(); err != nil {
		fmt.Printf("Failed to open database: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Teardown
	testDB.Close()

	// Drop test database
	adminDB, _ = sqlx.Open("postgres", adminURI)
	adminDB.Exec("DROP DATABASE IF EXISTS " + testDBName)
	adminDB.Close()

	os.Exit(code)
}

func skipIfNoPostgres(t *testing.T) {
	t.Helper()
	if skipTests {
		t.Skip("PostgreSQL not available, skipping integration test")
	}
}

func TestDatabaseIntegration(t *testing.T) {
	skipIfNoPostgres(t)
	ctx := context.Background()

	// Test inserting measurements
	m := database.Measurements{
		SiteID: 1,
		HostID: 1,
	}
	runID, err := testDB.MeasurementsInsert(ctx, &m)
	if err != nil {
		t.Fatalf("MeasurementsInsert failed: %v", err)
	}
	if runID == 0 {
		t.Fatal("Expected non-zero runID")
	}

	// Test inserting stats
	ts := time.Now().Unix()
	stats := []database.Stat{
		{
			RunID:     runID,
			Timestamp: ts,
			Start:     ts,
			Duration:  1000,
			CPU:       0,
			UserT:     10.5,
			Nice:      0.5,
			System:    5.0,
			IOWait:    2.0,
			Steal:     0.0,
			Idle:      82.0,
		},
		{
			RunID:     runID,
			Timestamp: ts,
			Start:     ts,
			Duration:  1000,
			CPU:       1,
			UserT:     15.0,
			Nice:      0.0,
			System:    3.0,
			IOWait:    1.0,
			Steal:     0.0,
			Idle:      81.0,
		},
	}
	if err := testDB.StatInsert(ctx, stats); err != nil {
		t.Fatalf("StatInsert failed: %v", err)
	}

	// Test inserting meminfo
	meminfo := &database.Meminfo{
		RunID:         runID,
		Timestamp:     ts,
		Start:         ts,
		Duration:      1000,
		MemFree:       1000000,
		MemAvailable:  2000000,
		MemUsed:       3000000,
		PercentUsed:   60.0,
		Buffers:       100000,
		Cached:        500000,
		Commit:        4000000,
		PercentCommit: 80.0,
		Active:        2500000,
		Inactive:      500000,
		Dirty:         1000,
	}
	if err := testDB.MeminfoInsert(ctx, meminfo); err != nil {
		t.Fatalf("MeminfoInsert failed: %v", err)
	}

	// Test inserting netdev
	netdevs := []database.NetDev{
		{
			RunID:        runID,
			Timestamp:    ts,
			Start:        ts,
			Duration:     1000,
			Name:         "eth0",
			RxPackets:    1000.0,
			TxPackets:    500.0,
			RxKBytes:     1024.0,
			TxKBytes:     512.0,
			RxCompressed: 0.0,
			TxCompressed: 0.0,
			RxMulticast:  10.0,
			IfUtil:       0.5,
		},
	}
	if err := testDB.NetDevInsert(ctx, netdevs); err != nil {
		t.Fatalf("NetDevInsert failed: %v", err)
	}

	// Test inserting diskstat
	diskstats := []database.Diskstat{
		{
			RunID:     runID,
			Timestamp: ts,
			Start:     ts,
			Duration:  1000,
			Name:      "sda",
			Tps:       100.0,
			Rtps:      50.0,
			Wtps:      50.0,
			Dtps:      0.0,
			Bread:     1024.0,
			Bwrtn:     512.0,
			Bdscd:     0.0,
		},
	}
	if err := testDB.DiskstatInsert(ctx, diskstats); err != nil {
		t.Fatalf("DiskstatInsert failed: %v", err)
	}

	// Test querying data back
	t.Run("StatSelect", func(t *testing.T) {
		result, err := testDB.StatSelect(ctx, runID)
		if err != nil {
			t.Fatalf("StatSelect failed: %v", err)
		}
		if len(result) != 2 {
			t.Errorf("Expected 2 stats, got %d", len(result))
		}
	})

	t.Run("MeminfoSelect", func(t *testing.T) {
		result, err := testDB.MeminfoSelect(ctx, runID)
		if err != nil {
			t.Fatalf("MeminfoSelect failed: %v", err)
		}
		if len(result) != 1 {
			t.Errorf("Expected 1 meminfo, got %d", len(result))
		}
	})

	t.Run("NetDevSelect", func(t *testing.T) {
		result, err := testDB.NetDevSelect(ctx, runID)
		if err != nil {
			t.Fatalf("NetDevSelect failed: %v", err)
		}
		if len(result) != 1 {
			t.Errorf("Expected 1 netdev, got %d", len(result))
		}
	})

	t.Run("DiskstatSelect", func(t *testing.T) {
		result, err := testDB.DiskstatSelect(ctx, runID)
		if err != nil {
			t.Fatalf("DiskstatSelect failed: %v", err)
		}
		if len(result) != 1 {
			t.Errorf("Expected 1 diskstat, got %d", len(result))
		}
	})

	t.Run("MeasurementsSelect", func(t *testing.T) {
		result, err := testDB.MeasurementsSelect(ctx, runID)
		if err != nil {
			t.Fatalf("MeasurementsSelect failed: %v", err)
		}
		if result.RunID != runID {
			t.Errorf("Expected runID %d, got %d", runID, result.RunID)
		}
	})

	t.Run("ListRuns", func(t *testing.T) {
		result, err := testDB.ListRuns(ctx)
		if err != nil {
			t.Fatalf("ListRuns failed: %v", err)
		}
		if len(result) < 1 {
			t.Error("Expected at least 1 run")
		}
	})
}

// APIServer is a simplified version for testing
type APIServer struct {
	db database.Database
}

func (api *APIServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "healthy",
		"timestamp":  time.Now().Unix(),
		"db_version": database.Version,
	})
}

func (api *APIServer) listRunsHandler(w http.ResponseWriter, r *http.Request) {
	runs, err := api.db.ListRuns(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"runs": runs})
}

func TestAPIIntegration(t *testing.T) {
	skipIfNoPostgres(t)

	api := &APIServer{db: testDB}

	t.Run("HealthEndpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()

		api.healthHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if response["status"] != "healthy" {
			t.Error("Expected status to be 'healthy'")
		}
	})

	t.Run("ListRunsEndpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/runs", nil)
		w := httptest.NewRecorder()

		api.listRunsHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		runs, ok := response["runs"].([]interface{})
		if !ok {
			t.Error("Expected runs to be an array")
		}
		if len(runs) < 1 {
			t.Error("Expected at least 1 run from previous test")
		}
	})
}
