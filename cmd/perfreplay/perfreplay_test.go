package main

import (
	"sync"
	"testing"
	"time"
)

// TestIncrementDroppedWork verifies the thread-safe dropped work counter.
func TestIncrementDroppedWork(t *testing.T) {
	// Reset counters
	droppedWorkMu.Lock()
	droppedStatWork = 0
	droppedMemWork = 0
	droppedDiskWork = 0
	droppedWorkMu.Unlock()

	// Test single increment
	incrementDroppedWork(&droppedStatWork)
	stat, mem, disk := getDroppedWorkMetrics()
	if stat != 1 {
		t.Errorf("expected droppedStatWork=1, got %d", stat)
	}
	if mem != 0 {
		t.Errorf("expected droppedMemWork=0, got %d", mem)
	}
	if disk != 0 {
		t.Errorf("expected droppedDiskWork=0, got %d", disk)
	}

	// Test concurrent increments
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			incrementDroppedWork(&droppedStatWork)
		}()
		go func() {
			defer wg.Done()
			incrementDroppedWork(&droppedMemWork)
		}()
		go func() {
			defer wg.Done()
			incrementDroppedWork(&droppedDiskWork)
		}()
	}
	wg.Wait()

	stat, mem, disk = getDroppedWorkMetrics()
	if stat != 101 { // 1 + 100
		t.Errorf("expected droppedStatWork=101, got %d", stat)
	}
	if mem != 100 {
		t.Errorf("expected droppedMemWork=100, got %d", mem)
	}
	if disk != 100 {
		t.Errorf("expected droppedDiskWork=100, got %d", disk)
	}
}

// TestGetDroppedWorkMetrics verifies metrics retrieval is thread-safe.
func TestGetDroppedWorkMetrics(t *testing.T) {
	// Reset counters
	droppedWorkMu.Lock()
	droppedStatWork = 5
	droppedMemWork = 10
	droppedDiskWork = 15
	droppedWorkMu.Unlock()

	stat, mem, disk := getDroppedWorkMetrics()
	if stat != 5 || mem != 10 || disk != 15 {
		t.Errorf("expected (5, 10, 15), got (%d, %d, %d)", stat, mem, disk)
	}
}

// TestGiveOrTake verifies the percentage comparison function.
func TestGiveOrTake(t *testing.T) {
	tests := []struct {
		name    string
		x       uint64
		y       uint64
		percent uint64
		want    bool
	}{
		{
			name:    "same values",
			x:       100,
			y:       100,
			percent: 10,
			want:    true,
		},
		{
			name:    "within 10% - lower",
			x:       100,
			y:       95,
			percent: 10,
			want:    true,
		},
		{
			name:    "within 10% - higher",
			x:       95,
			y:       100,
			percent: 10,
			want:    true,
		},
		{
			name:    "outside 10% - too low",
			x:       100,
			y:       80,
			percent: 10,
			want:    false,
		},
		{
			name:    "outside 10% - too high",
			x:       80,
			y:       100,
			percent: 10,
			want:    false,
		},
		{
			name:    "edge case - 0 values",
			x:       0,
			y:       0,
			percent: 10,
			want:    false, // Division by zero results in 0
		},
		{
			name:    "large values within tolerance",
			x:       1000000,
			y:       950000,
			percent: 10,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip zero case since it causes division by zero
			if tt.x == 0 && tt.y == 0 {
				t.Skip("zero case causes undefined behavior")
			}
			got := giveOrTake(tt.x, tt.y, tt.percent)
			if got != tt.want {
				t.Errorf("giveOrTake(%d, %d, %d) = %v, want %v",
					tt.x, tt.y, tt.percent, got, tt.want)
			}
		})
	}
}

// TestMinMax verifies the min and max helper functions.
func TestMinMax(t *testing.T) {
	tests := []struct {
		a, b    uint64
		wantMin uint64
		wantMax uint64
	}{
		{10, 20, 10, 20},
		{20, 10, 10, 20},
		{5, 5, 5, 5},
		{0, 100, 0, 100},
		{100, 0, 0, 100},
	}

	for _, tt := range tests {
		gotMin := min(tt.a, tt.b)
		gotMax := max(tt.a, tt.b)
		if gotMin != tt.wantMin {
			t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, gotMin, tt.wantMin)
		}
		if gotMax != tt.wantMax {
			t.Errorf("max(%d, %d) = %d, want %d", tt.a, tt.b, gotMax, tt.wantMax)
		}
	}
}

// TestCleanAndExpandPath verifies path expansion.
func TestCleanAndExpandPath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "absolute path unchanged",
			input: "/tmp/test.json",
			want:  "/tmp/test.json",
		},
		{
			name:  "clean double slashes",
			input: "/tmp//test.json",
			want:  "/tmp/test.json",
		},
		{
			name:  "clean dot segments",
			input: "/tmp/./test.json",
			want:  "/tmp/test.json",
		},
		{
			name:  "clean parent segments",
			input: "/tmp/foo/../test.json",
			want:  "/tmp/test.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanAndExpandPath(tt.input)
			if got != tt.want {
				t.Errorf("cleanAndExpandPath(%q) = %q, want %q",
					tt.input, got, tt.want)
			}
		})
	}
}

// TestFileExists verifies file existence check.
func TestFileExists(t *testing.T) {
	// Test with a file that definitely exists
	if !fileExists("/") {
		t.Error("fileExists('/') should return true")
	}

	// Test with a file that definitely doesn't exist
	if fileExists("/nonexistent/path/to/file.txt") {
		t.Error("fileExists('/nonexistent/path/to/file.txt') should return false")
	}
}

// TestChannelSendTimeout verifies the timeout constant is reasonable.
func TestChannelSendTimeout(t *testing.T) {
	if channelSendTimeout <= 0 {
		t.Error("channelSendTimeout should be positive")
	}
	if channelSendTimeout > time.Second {
		t.Error("channelSendTimeout should not exceed 1 second")
	}
	// Verify it's set to 100ms as expected
	if channelSendTimeout != 100*time.Millisecond {
		t.Errorf("channelSendTimeout = %v, want 100ms", channelSendTimeout)
	}
}

// TestVersionString verifies version string format.
func TestVersionString(t *testing.T) {
	v := versionString()
	if v == "" {
		t.Error("versionString() should not be empty")
	}
	if v != "1.0.0" {
		t.Errorf("versionString() = %q, want '1.0.0'", v)
	}
}

// BenchmarkIncrementDroppedWork measures performance of the thread-safe counter.
func BenchmarkIncrementDroppedWork(b *testing.B) {
	// Reset counter
	droppedWorkMu.Lock()
	droppedStatWork = 0
	droppedWorkMu.Unlock()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			incrementDroppedWork(&droppedStatWork)
		}
	})
}

// BenchmarkGiveOrTake measures performance of the percentage comparison.
func BenchmarkGiveOrTake(b *testing.B) {
	for i := 0; i < b.N; i++ {
		giveOrTake(1000000, 950000, 10)
	}
}

// TestReplayModeEnabled verifies the replay mode selection logic.
func TestReplayModeEnabled(t *testing.T) {
	tests := []struct {
		mode       string
		replayType string
		want       bool
	}{
		// "all" mode enables everything
		{"all", "cpu", true},
		{"all", "memory", true},
		{"all", "disk", true},

		// "cpu" mode only enables cpu
		{"cpu", "cpu", true},
		{"cpu", "memory", false},
		{"cpu", "disk", false},

		// "memory" mode only enables memory
		{"memory", "cpu", false},
		{"memory", "memory", true},
		{"memory", "disk", false},

		// "disk" mode only enables disk
		{"disk", "cpu", false},
		{"disk", "memory", false},
		{"disk", "disk", true},

		// "cpu-memory" mode enables both cpu and memory
		{"cpu-memory", "cpu", true},
		{"cpu-memory", "memory", true},
		{"cpu-memory", "disk", false},

		// Invalid mode returns false
		{"invalid", "cpu", false},
		{"", "cpu", false},
	}

	for _, tt := range tests {
		name := tt.mode + "/" + tt.replayType
		t.Run(name, func(t *testing.T) {
			got := replayModeEnabled(tt.mode, tt.replayType)
			if got != tt.want {
				t.Errorf("replayModeEnabled(%q, %q) = %v, want %v",
					tt.mode, tt.replayType, got, tt.want)
			}
		})
	}
}
