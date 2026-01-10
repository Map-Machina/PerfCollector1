package parser

import (
	"testing"
)

// Sample /proc/stat content for testing
const sampleProcStat = `cpu  74608 2520 18574 829888 1084 0 289 0 0 0
cpu0 37399 1157 9523 413968 508 0 169 0 0 0
cpu1 37208 1362 9050 415920 576 0 119 0 0 0
intr 8388256 7 0 0 0 0 0 0 0 1 0 0 0 156 0 0 2779 0 0 0 0 0 0 0 0 0 0 0 0 0 0
ctxt 16783911
btime 1609459200
processes 12345
procs_running 2
procs_blocked 0
softirq 3282017 1 1159519 2 20958 252429 0 10567 813396 0 1025145
`

func TestProcessStat(t *testing.T) {
	stat, err := ProcessStat([]byte(sampleProcStat))
	if err != nil {
		t.Fatalf("ProcessStat failed: %v", err)
	}

	// Check boot time
	if stat.BootTime != 1609459200 {
		t.Errorf("expected BootTime=1609459200, got %d", stat.BootTime)
	}

	// Check context switches
	if stat.ContextSwitches != 16783911 {
		t.Errorf("expected ContextSwitches=16783911, got %d", stat.ContextSwitches)
	}

	// Check processes created
	if stat.ProcessCreated != 12345 {
		t.Errorf("expected ProcessCreated=12345, got %d", stat.ProcessCreated)
	}

	// Check running processes
	if stat.ProcessesRunning != 2 {
		t.Errorf("expected ProcessesRunning=2, got %d", stat.ProcessesRunning)
	}

	// Check blocked processes
	if stat.ProcessesBlocked != 0 {
		t.Errorf("expected ProcessesBlocked=0, got %d", stat.ProcessesBlocked)
	}

	// Check CPU count
	if len(stat.CPU) != 2 {
		t.Errorf("expected 2 CPUs, got %d", len(stat.CPU))
	}

	// Check IRQ total
	if stat.IRQTotal != 8388256 {
		t.Errorf("expected IRQTotal=8388256, got %d", stat.IRQTotal)
	}

	// Check softirq total
	if stat.SoftIRQTotal != 3282017 {
		t.Errorf("expected SoftIRQTotal=3282017, got %d", stat.SoftIRQTotal)
	}
}

func TestProcessStatCPUTotal(t *testing.T) {
	stat, err := ProcessStat([]byte(sampleProcStat))
	if err != nil {
		t.Fatalf("ProcessStat failed: %v", err)
	}

	// Values are divided by userHZ (100)
	// cpu  74608 2520 18574 829888 1084 0 289 0 0 0
	expectedUser := 74608.0 / 100.0
	expectedNice := 2520.0 / 100.0
	expectedSystem := 18574.0 / 100.0
	expectedIdle := 829888.0 / 100.0

	if stat.CPUTotal.User != expectedUser {
		t.Errorf("expected CPUTotal.User=%.2f, got %.2f", expectedUser, stat.CPUTotal.User)
	}
	if stat.CPUTotal.Nice != expectedNice {
		t.Errorf("expected CPUTotal.Nice=%.2f, got %.2f", expectedNice, stat.CPUTotal.Nice)
	}
	if stat.CPUTotal.System != expectedSystem {
		t.Errorf("expected CPUTotal.System=%.2f, got %.2f", expectedSystem, stat.CPUTotal.System)
	}
	if stat.CPUTotal.Idle != expectedIdle {
		t.Errorf("expected CPUTotal.Idle=%.2f, got %.2f", expectedIdle, stat.CPUTotal.Idle)
	}
}

func TestProcessStatPerCPU(t *testing.T) {
	stat, err := ProcessStat([]byte(sampleProcStat))
	if err != nil {
		t.Fatalf("ProcessStat failed: %v", err)
	}

	// Check cpu0
	// cpu0 37399 1157 9523 413968 508 0 169 0 0 0
	expectedCPU0User := 37399.0 / 100.0
	if stat.CPU[0].User != expectedCPU0User {
		t.Errorf("expected CPU[0].User=%.2f, got %.2f", expectedCPU0User, stat.CPU[0].User)
	}

	// Check cpu1
	// cpu1 37208 1362 9050 415920 576 0 119 0 0 0
	expectedCPU1User := 37208.0 / 100.0
	if stat.CPU[1].User != expectedCPU1User {
		t.Errorf("expected CPU[1].User=%.2f, got %.2f", expectedCPU1User, stat.CPU[1].User)
	}
}

func TestProcessStatSoftIRQ(t *testing.T) {
	stat, err := ProcessStat([]byte(sampleProcStat))
	if err != nil {
		t.Fatalf("ProcessStat failed: %v", err)
	}

	// softirq 3282017 1 1159519 2 20958 252429 0 10567 813396 0 1025145
	if stat.SoftIRQ.Hi != 1 {
		t.Errorf("expected SoftIRQ.Hi=1, got %d", stat.SoftIRQ.Hi)
	}
	if stat.SoftIRQ.Timer != 1159519 {
		t.Errorf("expected SoftIRQ.Timer=1159519, got %d", stat.SoftIRQ.Timer)
	}
	if stat.SoftIRQ.NetTx != 2 {
		t.Errorf("expected SoftIRQ.NetTx=2, got %d", stat.SoftIRQ.NetTx)
	}
	if stat.SoftIRQ.NetRx != 20958 {
		t.Errorf("expected SoftIRQ.NetRx=20958, got %d", stat.SoftIRQ.NetRx)
	}
}

func TestProcessStatEmpty(t *testing.T) {
	stat, err := ProcessStat([]byte(""))
	if err != nil {
		t.Fatalf("ProcessStat failed on empty input: %v", err)
	}
	if stat.BootTime != 0 {
		t.Errorf("expected BootTime=0 for empty input, got %d", stat.BootTime)
	}
}

func TestProcessStatMinimal(t *testing.T) {
	// Minimal valid stat with just boot time
	minimal := `btime 1234567890
`
	stat, err := ProcessStat([]byte(minimal))
	if err != nil {
		t.Fatalf("ProcessStat failed: %v", err)
	}
	if stat.BootTime != 1234567890 {
		t.Errorf("expected BootTime=1234567890, got %d", stat.BootTime)
	}
}

func TestParseCPUStat(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantCPUID  int64
		wantUser   float64
		wantErr    bool
	}{
		{
			name:      "total cpu",
			line:      "cpu  74608 2520 18574 829888 1084 0 289 0 0 0",
			wantCPUID: -1,
			wantUser:  746.08,
			wantErr:   false,
		},
		{
			name:      "cpu0",
			line:      "cpu0 37399 1157 9523 413968 508 0 169 0 0 0",
			wantCPUID: 0,
			wantUser:  373.99,
			wantErr:   false,
		},
		{
			name:      "cpu15",
			line:      "cpu15 10000 0 0 90000 0 0 0 0 0 0",
			wantCPUID: 15,
			wantUser:  100.0,
			wantErr:   false,
		},
		{
			name:    "invalid line",
			line:    "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpuStat, cpuID, err := parseCPUStat(tt.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCPUStat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if cpuID != tt.wantCPUID {
				t.Errorf("parseCPUStat() cpuID = %v, want %v", cpuID, tt.wantCPUID)
			}
			if cpuStat.User != tt.wantUser {
				t.Errorf("parseCPUStat() User = %v, want %v", cpuStat.User, tt.wantUser)
			}
		})
	}
}

func TestParseSoftIRQStat(t *testing.T) {
	line := "softirq 3282017 1 1159519 2 20958 252429 0 10567 813396 0 1025145"

	softirq, total, err := parseSoftIRQStat(line)
	if err != nil {
		t.Fatalf("parseSoftIRQStat failed: %v", err)
	}

	if total != 3282017 {
		t.Errorf("expected total=3282017, got %d", total)
	}
	if softirq.Hi != 1 {
		t.Errorf("expected Hi=1, got %d", softirq.Hi)
	}
	if softirq.Timer != 1159519 {
		t.Errorf("expected Timer=1159519, got %d", softirq.Timer)
	}
}

func TestUserHZConstant(t *testing.T) {
	if UserHZ != 100 {
		t.Errorf("expected UserHZ=100, got %d", UserHZ)
	}
}

// Benchmark tests
func BenchmarkProcessStat(b *testing.B) {
	data := []byte(sampleProcStat)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ProcessStat(data)
	}
}

func BenchmarkParseCPUStat(b *testing.B) {
	line := "cpu  74608 2520 18574 829888 1084 0 289 0 0 0"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = parseCPUStat(line)
	}
}
