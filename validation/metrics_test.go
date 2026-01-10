package validation

import (
	"math"
	"os"
	"testing"
)

func TestNewCollector(t *testing.T) {
	criteria := DefaultAcceptanceCriteria()
	collector := NewCollector(criteria)

	if collector == nil {
		t.Fatal("NewCollector returned nil")
	}
	if collector.series == nil {
		t.Error("series map not initialized")
	}
	if collector.criteria.MaxRMSE != 5.0 {
		t.Errorf("expected MaxRMSE=5.0, got %f", collector.criteria.MaxRMSE)
	}
}

func TestDefaultAcceptanceCriteria(t *testing.T) {
	criteria := DefaultAcceptanceCriteria()

	if criteria.MaxRMSE != 5.0 {
		t.Errorf("expected MaxRMSE=5.0, got %f", criteria.MaxRMSE)
	}
	if criteria.MinCorrelation != 0.95 {
		t.Errorf("expected MinCorrelation=0.95, got %f", criteria.MinCorrelation)
	}
	if criteria.MinWithin5Percent != 80.0 {
		t.Errorf("expected MinWithin5Percent=80.0, got %f", criteria.MinWithin5Percent)
	}
	if criteria.MinWithin10Percent != 95.0 {
		t.Errorf("expected MinWithin10Percent=95.0, got %f", criteria.MinWithin10Percent)
	}
	if criteria.MaxPeakError != 10.0 {
		t.Errorf("expected MaxPeakError=10.0, got %f", criteria.MaxPeakError)
	}
}

func TestRecordAndCompute(t *testing.T) {
	collector := NewCollector(DefaultAcceptanceCriteria())

	// Record some CPU samples with perfect accuracy
	for i := 0; i < 10; i++ {
		collector.RecordCPU(50.0, 50.0) // Target = Actual
	}

	result, err := collector.Compute(MetricCPU)
	if err != nil {
		t.Fatalf("Compute failed: %v", err)
	}

	if result.SampleCount != 10 {
		t.Errorf("expected SampleCount=10, got %d", result.SampleCount)
	}
	if result.RMSE != 0.0 {
		t.Errorf("expected RMSE=0.0 for perfect match, got %f", result.RMSE)
	}
	if result.MAE != 0.0 {
		t.Errorf("expected MAE=0.0 for perfect match, got %f", result.MAE)
	}
	if result.Correlation != 1.0 && !math.IsNaN(result.Correlation) {
		// Correlation is NaN when all values are identical (no variance)
		t.Logf("Correlation=%f (expected 1.0 or NaN for constant values)", result.Correlation)
	}
	if result.Within5Percent != 100.0 {
		t.Errorf("expected Within5Percent=100.0, got %f", result.Within5Percent)
	}
	if result.Within10Percent != 100.0 {
		t.Errorf("expected Within10Percent=100.0, got %f", result.Within10Percent)
	}
}

func TestComputeWithVariation(t *testing.T) {
	collector := NewCollector(DefaultAcceptanceCriteria())

	// Record samples with known variation
	// Target: 100, Actual: varies
	testData := []struct {
		target float64
		actual float64
	}{
		{100.0, 100.0}, // 0% error
		{100.0, 102.0}, // 2% error
		{100.0, 95.0},  // 5% error
		{100.0, 108.0}, // 8% error
		{100.0, 90.0},  // 10% error
	}

	for _, d := range testData {
		collector.RecordCPU(d.target, d.actual)
	}

	result, err := collector.Compute(MetricCPU)
	if err != nil {
		t.Fatalf("Compute failed: %v", err)
	}

	if result.SampleCount != 5 {
		t.Errorf("expected SampleCount=5, got %d", result.SampleCount)
	}

	// Mean target should be 100
	if result.MeanTarget != 100.0 {
		t.Errorf("expected MeanTarget=100.0, got %f", result.MeanTarget)
	}

	// Mean actual: (100+102+95+108+90)/5 = 99.0
	expectedMeanActual := 99.0
	if math.Abs(result.MeanActual-expectedMeanActual) > 0.001 {
		t.Errorf("expected MeanActual=%f, got %f", expectedMeanActual, result.MeanActual)
	}

	// Check tolerance buckets
	// Within 5%: 100, 102, 95 = 3 samples = 60%
	if math.Abs(result.Within5Percent-60.0) > 0.001 {
		t.Errorf("expected Within5Percent=60.0, got %f", result.Within5Percent)
	}

	// Within 10%: all 5 samples = 100%
	if result.Within10Percent != 100.0 {
		t.Errorf("expected Within10Percent=100.0, got %f", result.Within10Percent)
	}
}

func TestComputeRMSE(t *testing.T) {
	collector := NewCollector(DefaultAcceptanceCriteria())

	// Simple case: target=10, actual=12 (error=2)
	// RMSE = sqrt(4/1) = 2
	collector.RecordCPU(10.0, 12.0)

	result, err := collector.Compute(MetricCPU)
	if err != nil {
		t.Fatalf("Compute failed: %v", err)
	}

	if math.Abs(result.RMSE-2.0) > 0.001 {
		t.Errorf("expected RMSE=2.0, got %f", result.RMSE)
	}
	if math.Abs(result.MAE-2.0) > 0.001 {
		t.Errorf("expected MAE=2.0, got %f", result.MAE)
	}
}

func TestComputeCorrelation(t *testing.T) {
	collector := NewCollector(DefaultAcceptanceCriteria())

	// Perfect positive correlation: y = x
	for i := 1; i <= 10; i++ {
		collector.RecordCPU(float64(i)*10, float64(i)*10)
	}

	result, err := collector.Compute(MetricCPU)
	if err != nil {
		t.Fatalf("Compute failed: %v", err)
	}

	// For identical values with variance, correlation should be 1.0
	if math.Abs(result.Correlation-1.0) > 0.001 {
		t.Errorf("expected Correlation=1.0, got %f", result.Correlation)
	}
}

func TestComputeCorrelationNegative(t *testing.T) {
	collector := NewCollector(DefaultAcceptanceCriteria())

	// Negative correlation: as target increases, actual decreases
	for i := 1; i <= 10; i++ {
		collector.RecordCPU(float64(i)*10, float64(11-i)*10)
	}

	result, err := collector.Compute(MetricCPU)
	if err != nil {
		t.Fatalf("Compute failed: %v", err)
	}

	// Correlation should be -1.0
	if math.Abs(result.Correlation-(-1.0)) > 0.001 {
		t.Errorf("expected Correlation=-1.0, got %f", result.Correlation)
	}
}

func TestComputePeakMetrics(t *testing.T) {
	collector := NewCollector(DefaultAcceptanceCriteria())

	collector.RecordCPU(50.0, 48.0)
	collector.RecordCPU(100.0, 95.0) // Peak
	collector.RecordCPU(30.0, 32.0)

	result, err := collector.Compute(MetricCPU)
	if err != nil {
		t.Fatalf("Compute failed: %v", err)
	}

	if result.PeakTarget != 100.0 {
		t.Errorf("expected PeakTarget=100.0, got %f", result.PeakTarget)
	}
	if result.PeakActual != 95.0 {
		t.Errorf("expected PeakActual=95.0, got %f", result.PeakActual)
	}
	// Peak error: |95-100|/100 * 100 = 5%
	if math.Abs(result.PeakError-5.0) > 0.001 {
		t.Errorf("expected PeakError=5.0, got %f", result.PeakError)
	}
}

func TestComputeNoSamples(t *testing.T) {
	collector := NewCollector(DefaultAcceptanceCriteria())

	_, err := collector.Compute(MetricCPU)
	if err == nil {
		t.Error("expected error for no samples")
	}
}

func TestMultipleMetricTypes(t *testing.T) {
	collector := NewCollector(DefaultAcceptanceCriteria())

	collector.RecordCPU(50.0, 50.0)
	collector.RecordMemory(1000.0, 1000.0)
	collector.RecordDiskRead(100.0, 100.0)
	collector.RecordDiskWrite(50.0, 50.0)

	results := collector.ComputeAll()

	if len(results) != 4 {
		t.Errorf("expected 4 metric types, got %d", len(results))
	}

	for metricType, result := range results {
		if result.SampleCount != 1 {
			t.Errorf("%s: expected SampleCount=1, got %d", metricType, result.SampleCount)
		}
	}
}

func TestCheckAcceptancePassing(t *testing.T) {
	collector := NewCollector(DefaultAcceptanceCriteria())

	// Record data that should pass all criteria
	// Use varying target values to ensure good correlation
	for i := 0; i < 100; i++ {
		target := float64(10 + i) // Varying target: 10, 11, 12, ..., 109
		// Actual tracks target with small variation (<5%)
		actual := target + float64(i%3)*0.3 // Small noise: 0, 0.3, 0.6
		collector.RecordCPU(target, actual)
	}

	result, _ := collector.Compute(MetricCPU)
	passed, failures := collector.CheckAcceptance(result)

	if !passed {
		t.Errorf("expected acceptance to pass, got failures: %v", failures)
	}
}

func TestCheckAcceptanceFailing(t *testing.T) {
	// Use strict criteria
	criteria := AcceptanceCriteria{
		MaxRMSE:            1.0,  // Very strict
		MinCorrelation:     0.99, // Very strict
		MinWithin5Percent:  95.0, // Very strict
		MinWithin10Percent: 99.0, // Very strict
		MaxPeakError:       1.0,  // Very strict
	}
	collector := NewCollector(criteria)

	// Record data with significant variation
	collector.RecordCPU(100.0, 80.0)  // 20% error
	collector.RecordCPU(100.0, 120.0) // 20% error

	result, _ := collector.Compute(MetricCPU)
	passed, failures := collector.CheckAcceptance(result)

	if passed {
		t.Error("expected acceptance to fail with high variation")
	}
	if len(failures) == 0 {
		t.Error("expected failure reasons to be populated")
	}
}

func TestWriteCSV(t *testing.T) {
	collector := NewCollector(DefaultAcceptanceCriteria())

	collector.RecordCPU(50.0, 52.0)
	collector.RecordCPU(60.0, 58.0)
	collector.RecordMemory(1000.0, 1010.0)

	tmpFile := "/tmp/validation_test.csv"
	defer os.Remove(tmpFile)

	err := collector.WriteCSV(tmpFile)
	if err != nil {
		t.Fatalf("WriteCSV failed: %v", err)
	}

	// Verify file exists and has content
	info, err := os.Stat(tmpFile)
	if err != nil {
		t.Fatalf("CSV file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Error("CSV file is empty")
	}
}

func TestSummary(t *testing.T) {
	collector := NewCollector(DefaultAcceptanceCriteria())

	for i := 0; i < 10; i++ {
		collector.RecordCPU(50.0, 50.0)
	}

	summary := collector.Summary()

	if summary == "" {
		t.Error("Summary returned empty string")
	}
	if !contains(summary, "REPLAY VALIDATION SUMMARY") {
		t.Error("Summary missing header")
	}
	if !contains(summary, "cpu") {
		t.Error("Summary missing CPU metric")
	}
}

func TestSummaryNoData(t *testing.T) {
	collector := NewCollector(DefaultAcceptanceCriteria())

	summary := collector.Summary()

	if summary != "No validation data collected" {
		t.Errorf("unexpected summary for empty collector: %s", summary)
	}
}

func TestConcurrentRecording(t *testing.T) {
	collector := NewCollector(DefaultAcceptanceCriteria())

	// Simulate concurrent recording from multiple goroutines
	done := make(chan bool, 3)

	go func() {
		for i := 0; i < 100; i++ {
			collector.RecordCPU(float64(i), float64(i))
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			collector.RecordMemory(float64(i*100), float64(i*100))
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			collector.RecordDiskRead(float64(i*10), float64(i*10))
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	results := collector.ComputeAll()
	if len(results) != 3 {
		t.Errorf("expected 3 metric types, got %d", len(results))
	}

	for metricType, result := range results {
		if result.SampleCount != 100 {
			t.Errorf("%s: expected 100 samples, got %d", metricType, result.SampleCount)
		}
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Benchmarks
func BenchmarkRecord(b *testing.B) {
	collector := NewCollector(DefaultAcceptanceCriteria())
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		collector.RecordCPU(50.0, 50.0)
	}
}

func BenchmarkCompute(b *testing.B) {
	collector := NewCollector(DefaultAcceptanceCriteria())
	for i := 0; i < 1000; i++ {
		collector.RecordCPU(float64(i), float64(i)+1.0)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		collector.Compute(MetricCPU)
	}
}
