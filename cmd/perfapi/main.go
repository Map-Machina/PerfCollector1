package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/businessperformancetuning/perfcollector/database"
	"github.com/businessperformancetuning/perfcollector/database/postgres"
)

// Metrics holds application metrics
type Metrics struct {
	RequestsTotal   atomic.Uint64
	RequestsSuccess atomic.Uint64
	RequestsError   atomic.Uint64
	RequestDuration atomic.Int64 // nanoseconds, for calculating average
	StartTime       time.Time
}

type APIServer struct {
	db      database.Database
	server  *http.Server
	metrics *Metrics
	logger  *slog.Logger
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type RunsResponse struct {
	Runs []database.Measurements `json:"runs"`
}

type RunDataResponse struct {
	Measurements *database.Measurements `json:"measurements"`
	Stats        []database.Stat        `json:"stats"`
	Meminfo      []database.Meminfo     `json:"meminfo"`
	NetDev       []database.NetDev      `json:"netdev"`
	Diskstat     []database.Diskstat    `json:"diskstat"`
}

type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp int64  `json:"timestamp"`
	DBVersion int    `json:"db_version"`
}

type MetricsResponse struct {
	Uptime          string  `json:"uptime"`
	RequestsTotal   uint64  `json:"requests_total"`
	RequestsSuccess uint64  `json:"requests_success"`
	RequestsError   uint64  `json:"requests_error"`
	AvgDurationMs   float64 `json:"avg_duration_ms"`
	GoRoutines      int     `json:"goroutines"`
	HeapAllocMB     float64 `json:"heap_alloc_mb"`
	HeapSysMB       float64 `json:"heap_sys_mb"`
}

// loggingMiddleware logs requests and tracks metrics
func (api *APIServer) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		// Update metrics
		api.metrics.RequestsTotal.Add(1)
		api.metrics.RequestDuration.Add(duration.Nanoseconds())
		if wrapped.statusCode >= 400 {
			api.metrics.RequestsError.Add(1)
		} else {
			api.metrics.RequestsSuccess.Add(1)
		}

		// Log request
		api.logger.Info("request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", wrapped.statusCode),
			slog.Duration("duration", duration),
			slog.String("remote_addr", r.RemoteAddr),
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{Error: message})
}

func (api *APIServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now().Unix(),
		DBVersion: database.Version,
	})
}

func (api *APIServer) metricsHandler(w http.ResponseWriter, r *http.Request) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	total := api.metrics.RequestsTotal.Load()
	var avgDuration float64
	if total > 0 {
		avgDuration = float64(api.metrics.RequestDuration.Load()) / float64(total) / 1e6 // convert to ms
	}

	writeJSON(w, http.StatusOK, MetricsResponse{
		Uptime:          time.Since(api.metrics.StartTime).Round(time.Second).String(),
		RequestsTotal:   total,
		RequestsSuccess: api.metrics.RequestsSuccess.Load(),
		RequestsError:   api.metrics.RequestsError.Load(),
		AvgDurationMs:   avgDuration,
		GoRoutines:      runtime.NumGoroutine(),
		HeapAllocMB:     float64(memStats.HeapAlloc) / 1024 / 1024,
		HeapSysMB:       float64(memStats.HeapSys) / 1024 / 1024,
	})
}

// prometheusHandler returns metrics in Prometheus format
func (api *APIServer) prometheusHandler(w http.ResponseWriter, r *http.Request) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	total := api.metrics.RequestsTotal.Load()
	var avgDuration float64
	if total > 0 {
		avgDuration = float64(api.metrics.RequestDuration.Load()) / float64(total) / 1e9 // convert to seconds
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP perfapi_requests_total Total number of HTTP requests\n")
	fmt.Fprintf(w, "# TYPE perfapi_requests_total counter\n")
	fmt.Fprintf(w, "perfapi_requests_total %d\n", total)

	fmt.Fprintf(w, "# HELP perfapi_requests_success_total Total number of successful HTTP requests\n")
	fmt.Fprintf(w, "# TYPE perfapi_requests_success_total counter\n")
	fmt.Fprintf(w, "perfapi_requests_success_total %d\n", api.metrics.RequestsSuccess.Load())

	fmt.Fprintf(w, "# HELP perfapi_requests_error_total Total number of failed HTTP requests\n")
	fmt.Fprintf(w, "# TYPE perfapi_requests_error_total counter\n")
	fmt.Fprintf(w, "perfapi_requests_error_total %d\n", api.metrics.RequestsError.Load())

	fmt.Fprintf(w, "# HELP perfapi_request_duration_seconds Average request duration in seconds\n")
	fmt.Fprintf(w, "# TYPE perfapi_request_duration_seconds gauge\n")
	fmt.Fprintf(w, "perfapi_request_duration_seconds %f\n", avgDuration)

	fmt.Fprintf(w, "# HELP perfapi_uptime_seconds Server uptime in seconds\n")
	fmt.Fprintf(w, "# TYPE perfapi_uptime_seconds gauge\n")
	fmt.Fprintf(w, "perfapi_uptime_seconds %f\n", time.Since(api.metrics.StartTime).Seconds())

	fmt.Fprintf(w, "# HELP perfapi_goroutines Number of goroutines\n")
	fmt.Fprintf(w, "# TYPE perfapi_goroutines gauge\n")
	fmt.Fprintf(w, "perfapi_goroutines %d\n", runtime.NumGoroutine())

	fmt.Fprintf(w, "# HELP perfapi_heap_alloc_bytes Heap memory allocated in bytes\n")
	fmt.Fprintf(w, "# TYPE perfapi_heap_alloc_bytes gauge\n")
	fmt.Fprintf(w, "perfapi_heap_alloc_bytes %d\n", memStats.HeapAlloc)

	fmt.Fprintf(w, "# HELP perfapi_heap_sys_bytes Heap memory obtained from system in bytes\n")
	fmt.Fprintf(w, "# TYPE perfapi_heap_sys_bytes gauge\n")
	fmt.Fprintf(w, "perfapi_heap_sys_bytes %d\n", memStats.HeapSys)
}

func (api *APIServer) listRunsHandler(w http.ResponseWriter, r *http.Request) {
	runs, err := api.db.ListRuns(r.Context())
	if err != nil {
		api.logger.Error("failed to list runs", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list runs: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, RunsResponse{Runs: runs})
}

func (api *APIServer) getRunHandler(w http.ResponseWriter, r *http.Request) {
	runIDStr := r.PathValue("runID")
	runID, err := strconv.ParseUint(runIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run ID")
		return
	}

	ctx := r.Context()

	measurements, err := api.db.MeasurementsSelect(ctx, runID)
	if err != nil {
		api.logger.Error("run not found", slog.Uint64("runID", runID), slog.String("error", err.Error()))
		writeError(w, http.StatusNotFound, fmt.Sprintf("run not found: %v", err))
		return
	}

	stats, err := api.db.StatSelect(ctx, runID)
	if err != nil {
		api.logger.Error("failed to get stats", slog.Uint64("runID", runID), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get stats: %v", err))
		return
	}

	meminfo, err := api.db.MeminfoSelect(ctx, runID)
	if err != nil {
		api.logger.Error("failed to get meminfo", slog.Uint64("runID", runID), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get meminfo: %v", err))
		return
	}

	netdev, err := api.db.NetDevSelect(ctx, runID)
	if err != nil {
		api.logger.Error("failed to get netdev", slog.Uint64("runID", runID), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get netdev: %v", err))
		return
	}

	diskstat, err := api.db.DiskstatSelect(ctx, runID)
	if err != nil {
		api.logger.Error("failed to get diskstat", slog.Uint64("runID", runID), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get diskstat: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, RunDataResponse{
		Measurements: measurements,
		Stats:        stats,
		Meminfo:      meminfo,
		NetDev:       netdev,
		Diskstat:     diskstat,
	})
}

func (api *APIServer) getStatsHandler(w http.ResponseWriter, r *http.Request) {
	runIDStr := r.PathValue("runID")
	runID, err := strconv.ParseUint(runIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run ID")
		return
	}

	stats, err := api.db.StatSelect(r.Context(), runID)
	if err != nil {
		api.logger.Error("failed to get stats", slog.Uint64("runID", runID), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get stats: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

func (api *APIServer) getMeminfoHandler(w http.ResponseWriter, r *http.Request) {
	runIDStr := r.PathValue("runID")
	runID, err := strconv.ParseUint(runIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run ID")
		return
	}

	meminfo, err := api.db.MeminfoSelect(r.Context(), runID)
	if err != nil {
		api.logger.Error("failed to get meminfo", slog.Uint64("runID", runID), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get meminfo: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, meminfo)
}

func (api *APIServer) getNetDevHandler(w http.ResponseWriter, r *http.Request) {
	runIDStr := r.PathValue("runID")
	runID, err := strconv.ParseUint(runIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run ID")
		return
	}

	netdev, err := api.db.NetDevSelect(r.Context(), runID)
	if err != nil {
		api.logger.Error("failed to get netdev", slog.Uint64("runID", runID), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get netdev: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, netdev)
}

func (api *APIServer) getDiskstatHandler(w http.ResponseWriter, r *http.Request) {
	runIDStr := r.PathValue("runID")
	runID, err := strconv.ParseUint(runIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run ID")
		return
	}

	diskstat, err := api.db.DiskstatSelect(r.Context(), runID)
	if err != nil {
		api.logger.Error("failed to get diskstat", slog.Uint64("runID", runID), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get diskstat: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, diskstat)
}

func (api *APIServer) exportStatsCSV(w http.ResponseWriter, r *http.Request) {
	runIDStr := r.PathValue("runID")
	runID, err := strconv.ParseUint(runIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run ID")
		return
	}

	stats, err := api.db.StatSelect(r.Context(), runID)
	if err != nil {
		api.logger.Error("failed to export stats", slog.Uint64("runID", runID), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get stats: %v", err))
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=stats_run_%d.csv", runID))

	csvWriter := csv.NewWriter(w)
	csvWriter.Write([]string{"runid", "timestamp", "start", "duration", "cpu", "user", "nice", "system", "iowait", "steal", "idle"})

	for _, s := range stats {
		csvWriter.Write([]string{
			strconv.FormatUint(s.RunID, 10),
			strconv.FormatInt(s.Timestamp, 10),
			strconv.FormatInt(s.Start, 10),
			strconv.FormatInt(s.Duration, 10),
			strconv.Itoa(s.CPU),
			strconv.FormatFloat(s.UserT, 'f', 2, 64),
			strconv.FormatFloat(s.Nice, 'f', 2, 64),
			strconv.FormatFloat(s.System, 'f', 2, 64),
			strconv.FormatFloat(s.IOWait, 'f', 2, 64),
			strconv.FormatFloat(s.Steal, 'f', 2, 64),
			strconv.FormatFloat(s.Idle, 'f', 2, 64),
		})
	}
	csvWriter.Flush()
}

func (api *APIServer) exportMeminfoCSV(w http.ResponseWriter, r *http.Request) {
	runIDStr := r.PathValue("runID")
	runID, err := strconv.ParseUint(runIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run ID")
		return
	}

	meminfo, err := api.db.MeminfoSelect(r.Context(), runID)
	if err != nil {
		api.logger.Error("failed to export meminfo", slog.Uint64("runID", runID), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get meminfo: %v", err))
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=meminfo_run_%d.csv", runID))

	csvWriter := csv.NewWriter(w)
	csvWriter.Write([]string{"runid", "timestamp", "start", "duration", "memfree", "memavailable", "memused", "percentused", "buffers", "cached", "commit", "percentcommit", "active", "inactive", "dirty"})

	for _, m := range meminfo {
		csvWriter.Write([]string{
			strconv.FormatUint(m.RunID, 10),
			strconv.FormatInt(m.Timestamp, 10),
			strconv.FormatInt(m.Start, 10),
			strconv.FormatInt(m.Duration, 10),
			strconv.FormatUint(m.MemFree, 10),
			strconv.FormatUint(m.MemAvailable, 10),
			strconv.FormatUint(m.MemUsed, 10),
			strconv.FormatFloat(m.PercentUsed, 'f', 2, 64),
			strconv.FormatUint(m.Buffers, 10),
			strconv.FormatUint(m.Cached, 10),
			strconv.FormatUint(m.Commit, 10),
			strconv.FormatFloat(m.PercentCommit, 'f', 2, 64),
			strconv.FormatUint(m.Active, 10),
			strconv.FormatUint(m.Inactive, 10),
			strconv.FormatUint(m.Dirty, 10),
		})
	}
	csvWriter.Flush()
}

func (api *APIServer) exportNetDevCSV(w http.ResponseWriter, r *http.Request) {
	runIDStr := r.PathValue("runID")
	runID, err := strconv.ParseUint(runIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run ID")
		return
	}

	netdev, err := api.db.NetDevSelect(r.Context(), runID)
	if err != nil {
		api.logger.Error("failed to export netdev", slog.Uint64("runID", runID), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get netdev: %v", err))
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=netdev_run_%d.csv", runID))

	csvWriter := csv.NewWriter(w)
	csvWriter.Write([]string{"runid", "timestamp", "start", "duration", "name", "rxpackets", "txpackets", "rxkbytes", "txkbytes", "rxcompressed", "txcompressed", "rxmulticast", "ifutil"})

	for _, n := range netdev {
		csvWriter.Write([]string{
			strconv.FormatUint(n.RunID, 10),
			strconv.FormatInt(n.Timestamp, 10),
			strconv.FormatInt(n.Start, 10),
			strconv.FormatInt(n.Duration, 10),
			n.Name,
			strconv.FormatFloat(n.RxPackets, 'f', 2, 64),
			strconv.FormatFloat(n.TxPackets, 'f', 2, 64),
			strconv.FormatFloat(n.RxKBytes, 'f', 2, 64),
			strconv.FormatFloat(n.TxKBytes, 'f', 2, 64),
			strconv.FormatFloat(n.RxCompressed, 'f', 2, 64),
			strconv.FormatFloat(n.TxCompressed, 'f', 2, 64),
			strconv.FormatFloat(n.RxMulticast, 'f', 2, 64),
			strconv.FormatFloat(n.IfUtil, 'f', 2, 64),
		})
	}
	csvWriter.Flush()
}

func (api *APIServer) exportDiskstatCSV(w http.ResponseWriter, r *http.Request) {
	runIDStr := r.PathValue("runID")
	runID, err := strconv.ParseUint(runIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run ID")
		return
	}

	diskstat, err := api.db.DiskstatSelect(r.Context(), runID)
	if err != nil {
		api.logger.Error("failed to export diskstat", slog.Uint64("runID", runID), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get diskstat: %v", err))
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=diskstat_run_%d.csv", runID))

	csvWriter := csv.NewWriter(w)
	csvWriter.Write([]string{"runid", "timestamp", "start", "duration", "name", "tps", "rtps", "wtps", "dtps", "bread", "bwrtn", "bdscd"})

	for _, d := range diskstat {
		csvWriter.Write([]string{
			strconv.FormatUint(d.RunID, 10),
			strconv.FormatInt(d.Timestamp, 10),
			strconv.FormatInt(d.Start, 10),
			strconv.FormatInt(d.Duration, 10),
			d.Name,
			strconv.FormatFloat(d.Tps, 'f', 2, 64),
			strconv.FormatFloat(d.Rtps, 'f', 2, 64),
			strconv.FormatFloat(d.Wtps, 'f', 2, 64),
			strconv.FormatFloat(d.Dtps, 'f', 2, 64),
			strconv.FormatFloat(d.Bread, 'f', 2, 64),
			strconv.FormatFloat(d.Bwrtn, 'f', 2, 64),
			strconv.FormatFloat(d.Bdscd, 'f', 2, 64),
		})
	}
	csvWriter.Flush()
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func main() {
	listenAddr := getEnv("PERFAPI_LISTEN", ":8080")
	dbURI := getEnv("PERFAPI_DB_URI", "user=postgres dbname=performancedata host=localhost sslmode=disable")
	logFormat := getEnv("PERFAPI_LOG_FORMAT", "json") // json or text

	// Setup structured logger
	var logHandler slog.Handler
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if logFormat == "text" {
		logHandler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		logHandler = slog.NewJSONHandler(os.Stdout, opts)
	}
	logger := slog.New(logHandler)

	db, err := postgres.New(database.Name, dbURI)
	if err != nil {
		logger.Error("failed to create database connection", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if err := db.Open(); err != nil {
		logger.Error("failed to open database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer db.Close()

	api := &APIServer{
		db:     db,
		logger: logger,
		metrics: &Metrics{
			StartTime: time.Now(),
		},
	}

	mux := http.NewServeMux()

	// Health and metrics endpoints (no logging middleware)
	mux.HandleFunc("GET /health", api.healthHandler)
	mux.HandleFunc("GET /metrics", api.metricsHandler)
	mux.HandleFunc("GET /metrics/prometheus", api.prometheusHandler)

	// Runs endpoints
	mux.HandleFunc("GET /api/v1/runs", api.listRunsHandler)
	mux.HandleFunc("GET /api/v1/runs/{runID}", api.getRunHandler)

	// Data endpoints
	mux.HandleFunc("GET /api/v1/runs/{runID}/stats", api.getStatsHandler)
	mux.HandleFunc("GET /api/v1/runs/{runID}/meminfo", api.getMeminfoHandler)
	mux.HandleFunc("GET /api/v1/runs/{runID}/netdev", api.getNetDevHandler)
	mux.HandleFunc("GET /api/v1/runs/{runID}/diskstat", api.getDiskstatHandler)

	// Export endpoints (CSV)
	mux.HandleFunc("GET /api/v1/runs/{runID}/stats/export", api.exportStatsCSV)
	mux.HandleFunc("GET /api/v1/runs/{runID}/meminfo/export", api.exportMeminfoCSV)
	mux.HandleFunc("GET /api/v1/runs/{runID}/netdev/export", api.exportNetDevCSV)
	mux.HandleFunc("GET /api/v1/runs/{runID}/diskstat/export", api.exportDiskstatCSV)

	// Wrap with logging middleware
	httpHandler := api.loggingMiddleware(mux)

	api.server = &http.Server{
		Addr:         listenAddr,
		Handler:      httpHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs
		logger.Info("shutting down server")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		api.server.Shutdown(ctx)
	}()

	logger.Info("server starting",
		slog.String("address", listenAddr),
		slog.Int("db_version", database.Version),
	)

	if err := api.server.ListenAndServe(); err != http.ErrServerClosed {
		logger.Error("server error", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("server stopped")
}
