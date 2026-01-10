// Package validation provides replay accuracy validation metrics and reporting.
//
// This package implements statistical validation of replay accuracy by comparing
// target metrics (from journal) against actual metrics (measured during replay).
//
// Key metrics:
//   - RMSE (Root Mean Square Error): Measures average deviation from target
//   - Correlation: Measures how well replay tracks target patterns (target: >0.95)
//   - Tolerance: Percentage of samples within acceptable bounds (±5%, ±10%)
//   - Peak Accuracy: How well replay matches peak values
package validation

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"
)

// MetricType identifies the type of metric being validated.
type MetricType string

const (
	MetricCPU    MetricType = "cpu"
	MetricMemory MetricType = "memory"
	MetricDiskR  MetricType = "disk_read"
	MetricDiskW  MetricType = "disk_write"
	MetricNet    MetricType = "network"
)

// Sample represents a single data point for validation.
type Sample struct {
	Timestamp time.Time
	Target    float64 // Expected value from journal
	Actual    float64 // Measured value during replay
}

// MetricSeries holds a series of samples for a specific metric.
type MetricSeries struct {
	Type    MetricType
	Unit    string // e.g., "percent", "bytes", "iops"
	Samples []Sample
	mu      sync.Mutex
}

// ValidationResult contains computed validation metrics for a series.
type ValidationResult struct {
	MetricType       MetricType
	SampleCount      int
	RMSE             float64 // Root Mean Square Error
	MAE              float64 // Mean Absolute Error
	Correlation      float64 // Pearson correlation coefficient
	Within5Percent   float64 // Percentage of samples within ±5%
	Within10Percent  float64 // Percentage of samples within ±10%
	PeakTarget       float64 // Maximum target value
	PeakActual       float64 // Maximum actual value
	PeakError        float64 // Percentage error at peak
	MeanTarget       float64
	MeanActual       float64
	StdDevTarget     float64
	StdDevActual     float64
}

// AcceptanceCriteria defines the thresholds for acceptable replay accuracy.
type AcceptanceCriteria struct {
	MaxRMSE            float64 // Maximum acceptable RMSE (default: 5%)
	MinCorrelation     float64 // Minimum acceptable correlation (default: 0.95)
	MinWithin5Percent  float64 // Minimum % of samples within ±5% (default: 80%)
	MinWithin10Percent float64 // Minimum % of samples within ±10% (default: 95%)
	MaxPeakError       float64 // Maximum acceptable peak error (default: 10%)
}

// DefaultAcceptanceCriteria returns the default acceptance criteria.
func DefaultAcceptanceCriteria() AcceptanceCriteria {
	return AcceptanceCriteria{
		MaxRMSE:            5.0,  // 5%
		MinCorrelation:     0.95, // 95% correlation
		MinWithin5Percent:  80.0, // 80% within ±5%
		MinWithin10Percent: 95.0, // 95% within ±10%
		MaxPeakError:       10.0, // 10% peak error
	}
}

// Collector aggregates validation samples across all metric types.
type Collector struct {
	series   map[MetricType]*MetricSeries
	mu       sync.RWMutex
	started  time.Time
	criteria AcceptanceCriteria
}

// NewCollector creates a new validation collector.
func NewCollector(criteria AcceptanceCriteria) *Collector {
	return &Collector{
		series:   make(map[MetricType]*MetricSeries),
		started:  time.Now(),
		criteria: criteria,
	}
}

// Record adds a sample to the appropriate metric series.
func (c *Collector) Record(metricType MetricType, target, actual float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	series, ok := c.series[metricType]
	if !ok {
		series = &MetricSeries{
			Type:    metricType,
			Samples: make([]Sample, 0, 1000),
		}
		c.series[metricType] = series
	}

	series.mu.Lock()
	series.Samples = append(series.Samples, Sample{
		Timestamp: time.Now(),
		Target:    target,
		Actual:    actual,
	})
	series.mu.Unlock()
}

// RecordCPU is a convenience method for recording CPU metrics.
func (c *Collector) RecordCPU(targetBusy, actualBusy float64) {
	c.Record(MetricCPU, targetBusy, actualBusy)
}

// RecordMemory is a convenience method for recording memory metrics.
func (c *Collector) RecordMemory(targetUsed, actualUsed float64) {
	c.Record(MetricMemory, targetUsed, actualUsed)
}

// RecordDiskRead is a convenience method for recording disk read metrics.
func (c *Collector) RecordDiskRead(targetIOPS, actualIOPS float64) {
	c.Record(MetricDiskR, targetIOPS, actualIOPS)
}

// RecordDiskWrite is a convenience method for recording disk write metrics.
func (c *Collector) RecordDiskWrite(targetIOPS, actualIOPS float64) {
	c.Record(MetricDiskW, targetIOPS, actualIOPS)
}

// Compute calculates validation metrics for a specific metric type.
func (c *Collector) Compute(metricType MetricType) (*ValidationResult, error) {
	c.mu.RLock()
	series, ok := c.series[metricType]
	c.mu.RUnlock()

	if !ok || len(series.Samples) == 0 {
		return nil, fmt.Errorf("no samples for metric type: %s", metricType)
	}

	series.mu.Lock()
	samples := make([]Sample, len(series.Samples))
	copy(samples, series.Samples)
	series.mu.Unlock()

	n := len(samples)
	result := &ValidationResult{
		MetricType:  metricType,
		SampleCount: n,
	}

	// Calculate means
	var sumTarget, sumActual float64
	for _, s := range samples {
		sumTarget += s.Target
		sumActual += s.Actual
	}
	result.MeanTarget = sumTarget / float64(n)
	result.MeanActual = sumActual / float64(n)

	// Calculate RMSE, MAE, standard deviations, and tolerance counts
	var sumSquaredError, sumAbsError float64
	var sumSquaredDevTarget, sumSquaredDevActual float64
	var within5, within10 int
	var peakTarget, peakActual float64

	for _, s := range samples {
		// Error calculations
		error := s.Actual - s.Target
		sumSquaredError += error * error
		sumAbsError += math.Abs(error)

		// Standard deviation calculations
		devTarget := s.Target - result.MeanTarget
		devActual := s.Actual - result.MeanActual
		sumSquaredDevTarget += devTarget * devTarget
		sumSquaredDevActual += devActual * devActual

		// Tolerance calculations (avoid division by zero)
		if s.Target != 0 {
			percentError := math.Abs(error/s.Target) * 100
			if percentError <= 5.0 {
				within5++
			}
			if percentError <= 10.0 {
				within10++
			}
		} else if s.Actual == 0 {
			// Both zero = perfect match
			within5++
			within10++
		}

		// Peak tracking
		if s.Target > peakTarget {
			peakTarget = s.Target
		}
		if s.Actual > peakActual {
			peakActual = s.Actual
		}
	}

	result.RMSE = math.Sqrt(sumSquaredError / float64(n))
	result.MAE = sumAbsError / float64(n)
	result.StdDevTarget = math.Sqrt(sumSquaredDevTarget / float64(n))
	result.StdDevActual = math.Sqrt(sumSquaredDevActual / float64(n))
	result.Within5Percent = float64(within5) / float64(n) * 100
	result.Within10Percent = float64(within10) / float64(n) * 100
	result.PeakTarget = peakTarget
	result.PeakActual = peakActual

	// Peak error calculation
	if peakTarget != 0 {
		result.PeakError = math.Abs(peakActual-peakTarget) / peakTarget * 100
	}

	// Calculate Pearson correlation coefficient
	result.Correlation = c.calculateCorrelation(samples, result.MeanTarget, result.MeanActual)

	return result, nil
}

// calculateCorrelation computes the Pearson correlation coefficient.
func (c *Collector) calculateCorrelation(samples []Sample, meanTarget, meanActual float64) float64 {
	var sumProduct, sumSquaredTarget, sumSquaredActual float64

	for _, s := range samples {
		devTarget := s.Target - meanTarget
		devActual := s.Actual - meanActual
		sumProduct += devTarget * devActual
		sumSquaredTarget += devTarget * devTarget
		sumSquaredActual += devActual * devActual
	}

	denominator := math.Sqrt(sumSquaredTarget * sumSquaredActual)
	if denominator == 0 {
		return 0
	}

	return sumProduct / denominator
}

// ComputeAll calculates validation metrics for all recorded metric types.
func (c *Collector) ComputeAll() map[MetricType]*ValidationResult {
	c.mu.RLock()
	types := make([]MetricType, 0, len(c.series))
	for t := range c.series {
		types = append(types, t)
	}
	c.mu.RUnlock()

	results := make(map[MetricType]*ValidationResult)
	for _, t := range types {
		if result, err := c.Compute(t); err == nil {
			results[t] = result
		}
	}
	return results
}

// CheckAcceptance verifies if a result meets the acceptance criteria.
func (c *Collector) CheckAcceptance(result *ValidationResult) (passed bool, failures []string) {
	failures = make([]string, 0)

	if result.RMSE > c.criteria.MaxRMSE {
		failures = append(failures, fmt.Sprintf("RMSE %.2f%% exceeds max %.2f%%",
			result.RMSE, c.criteria.MaxRMSE))
	}

	if result.Correlation < c.criteria.MinCorrelation {
		failures = append(failures, fmt.Sprintf("Correlation %.4f below min %.4f",
			result.Correlation, c.criteria.MinCorrelation))
	}

	if result.Within5Percent < c.criteria.MinWithin5Percent {
		failures = append(failures, fmt.Sprintf("Within5%% %.1f%% below min %.1f%%",
			result.Within5Percent, c.criteria.MinWithin5Percent))
	}

	if result.Within10Percent < c.criteria.MinWithin10Percent {
		failures = append(failures, fmt.Sprintf("Within10%% %.1f%% below min %.1f%%",
			result.Within10Percent, c.criteria.MinWithin10Percent))
	}

	if result.PeakError > c.criteria.MaxPeakError {
		failures = append(failures, fmt.Sprintf("PeakError %.2f%% exceeds max %.2f%%",
			result.PeakError, c.criteria.MaxPeakError))
	}

	return len(failures) == 0, failures
}

// WriteCSV exports all samples to a CSV file for analysis.
func (c *Collector) WriteCSV(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"timestamp", "metric_type", "target", "actual", "error", "percent_error"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	for metricType, series := range c.series {
		series.mu.Lock()
		for _, s := range series.Samples {
			error := s.Actual - s.Target
			var percentError float64
			if s.Target != 0 {
				percentError = (error / s.Target) * 100
			}

			record := []string{
				s.Timestamp.Format(time.RFC3339Nano),
				string(metricType),
				fmt.Sprintf("%.6f", s.Target),
				fmt.Sprintf("%.6f", s.Actual),
				fmt.Sprintf("%.6f", error),
				fmt.Sprintf("%.2f", percentError),
			}
			if err := writer.Write(record); err != nil {
				series.mu.Unlock()
				return fmt.Errorf("failed to write CSV record: %w", err)
			}
		}
		series.mu.Unlock()
	}

	return nil
}

// Summary returns a formatted summary string of all validation results.
func (c *Collector) Summary() string {
	results := c.ComputeAll()
	if len(results) == 0 {
		return "No validation data collected"
	}

	var sb strings.Builder
	sb.WriteString("\n========== REPLAY VALIDATION SUMMARY ==========\n")
	sb.WriteString(fmt.Sprintf("Duration: %s\n", time.Since(c.started).Round(time.Second)))
	sb.WriteString(fmt.Sprintf("Criteria: RMSE<%.1f%%, Correlation>%.2f, Within5%%>%.0f%%, Within10%%>%.0f%%\n\n",
		c.criteria.MaxRMSE, c.criteria.MinCorrelation,
		c.criteria.MinWithin5Percent, c.criteria.MinWithin10Percent))

	allPassed := true
	for metricType, result := range results {
		passed, failures := c.CheckAcceptance(result)
		status := "✓ PASS"
		if !passed {
			status = "✗ FAIL"
			allPassed = false
		}

		sb.WriteString(fmt.Sprintf("--- %s [%s] ---\n", metricType, status))
		sb.WriteString(fmt.Sprintf("  Samples:     %d\n", result.SampleCount))
		sb.WriteString(fmt.Sprintf("  RMSE:        %.2f%%\n", result.RMSE))
		sb.WriteString(fmt.Sprintf("  Correlation: %.4f\n", result.Correlation))
		sb.WriteString(fmt.Sprintf("  Within ±5%%:  %.1f%%\n", result.Within5Percent))
		sb.WriteString(fmt.Sprintf("  Within ±10%%: %.1f%%\n", result.Within10Percent))
		sb.WriteString(fmt.Sprintf("  Peak Error:  %.2f%%\n", result.PeakError))

		if !passed {
			sb.WriteString("  Failures:\n")
			for _, f := range failures {
				sb.WriteString(fmt.Sprintf("    - %s\n", f))
			}
		}
		sb.WriteString("\n")
	}

	if allPassed {
		sb.WriteString("========== OVERALL: PASSED ==========\n")
	} else {
		sb.WriteString("========== OVERALL: FAILED ==========\n")
	}

	return sb.String()
}
