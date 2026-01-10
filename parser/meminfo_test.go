package parser

import (
	"testing"
)

// Sample /proc/meminfo content for testing
const sampleProcMeminfo = `MemTotal:       16291508 kB
MemFree:         1234567 kB
MemAvailable:   12345678 kB
Buffers:          456789 kB
Cached:          5678901 kB
SwapCached:            0 kB
Active:          8765432 kB
Inactive:        2345678 kB
Active(anon):    4567890 kB
Inactive(anon):   123456 kB
Active(file):    4197542 kB
Inactive(file):  2222222 kB
Unevictable:           0 kB
Mlocked:               0 kB
SwapTotal:       8388604 kB
SwapFree:        8388604 kB
Dirty:               123 kB
Writeback:             0 kB
AnonPages:       4444444 kB
Mapped:           555555 kB
Shmem:            666666 kB
KReclaimable:     777777 kB
Slab:             888888 kB
SReclaimable:     500000 kB
SUnreclaim:       388888 kB
KernelStack:       12345 kB
PageTables:        67890 kB
NFS_Unstable:          0 kB
Bounce:                0 kB
WritebackTmp:          0 kB
CommitLimit:    16534356 kB
Committed_AS:   11111111 kB
VmallocTotal:   34359738367 kB
VmallocUsed:       99999 kB
VmallocChunk:          0 kB
Percpu:            12800 kB
HardwareCorrupted:     0 kB
AnonHugePages:         0 kB
ShmemHugePages:        0 kB
ShmemPmdMapped:        0 kB
FileHugePages:         0 kB
FilePmdMapped:         0 kB
HugePages_Total:       0
HugePages_Free:        0
HugePages_Rsvd:        0
HugePages_Surp:        0
Hugepagesize:       2048 kB
Hugetlb:               0 kB
DirectMap4k:      234567 kB
DirectMap2M:     8765432 kB
DirectMap1G:     9437184 kB
`

func TestProcessMeminfo(t *testing.T) {
	meminfo, err := ProcessMeminfo([]byte(sampleProcMeminfo))
	if err != nil {
		t.Fatalf("ProcessMeminfo failed: %v", err)
	}

	// Check MemTotal
	if meminfo.MemTotal != 16291508 {
		t.Errorf("expected MemTotal=16291508, got %d", meminfo.MemTotal)
	}

	// Check MemFree
	if meminfo.MemFree != 1234567 {
		t.Errorf("expected MemFree=1234567, got %d", meminfo.MemFree)
	}

	// Check MemAvailable
	if meminfo.MemAvailable != 12345678 {
		t.Errorf("expected MemAvailable=12345678, got %d", meminfo.MemAvailable)
	}

	// Check Buffers
	if meminfo.Buffers != 456789 {
		t.Errorf("expected Buffers=456789, got %d", meminfo.Buffers)
	}

	// Check Cached
	if meminfo.Cached != 5678901 {
		t.Errorf("expected Cached=5678901, got %d", meminfo.Cached)
	}

	// Check SwapTotal
	if meminfo.SwapTotal != 8388604 {
		t.Errorf("expected SwapTotal=8388604, got %d", meminfo.SwapTotal)
	}

	// Check SwapFree
	if meminfo.SwapFree != 8388604 {
		t.Errorf("expected SwapFree=8388604, got %d", meminfo.SwapFree)
	}

	// Check Active
	if meminfo.Active != 8765432 {
		t.Errorf("expected Active=8765432, got %d", meminfo.Active)
	}

	// Check Inactive
	if meminfo.Inactive != 2345678 {
		t.Errorf("expected Inactive=2345678, got %d", meminfo.Inactive)
	}

	// Check Dirty
	if meminfo.Dirty != 123 {
		t.Errorf("expected Dirty=123, got %d", meminfo.Dirty)
	}
}

func TestProcessMeminfoSwap(t *testing.T) {
	meminfo, err := ProcessMeminfo([]byte(sampleProcMeminfo))
	if err != nil {
		t.Fatalf("ProcessMeminfo failed: %v", err)
	}

	// Check swap fields
	if meminfo.SwapTotal != 8388604 {
		t.Errorf("expected SwapTotal=8388604, got %d", meminfo.SwapTotal)
	}
	if meminfo.SwapFree != 8388604 {
		t.Errorf("expected SwapFree=8388604, got %d", meminfo.SwapFree)
	}
	if meminfo.SwapCached != 0 {
		t.Errorf("expected SwapCached=0, got %d", meminfo.SwapCached)
	}
}

func TestProcessMeminfoVmalloc(t *testing.T) {
	meminfo, err := ProcessMeminfo([]byte(sampleProcMeminfo))
	if err != nil {
		t.Fatalf("ProcessMeminfo failed: %v", err)
	}

	// Check vmalloc fields
	if meminfo.VmallocTotal != 34359738367 {
		t.Errorf("expected VmallocTotal=34359738367, got %d", meminfo.VmallocTotal)
	}
	if meminfo.VmallocUsed != 99999 {
		t.Errorf("expected VmallocUsed=99999, got %d", meminfo.VmallocUsed)
	}
}

func TestProcessMeminfoSlab(t *testing.T) {
	meminfo, err := ProcessMeminfo([]byte(sampleProcMeminfo))
	if err != nil {
		t.Fatalf("ProcessMeminfo failed: %v", err)
	}

	// Check slab fields
	if meminfo.Slab != 888888 {
		t.Errorf("expected Slab=888888, got %d", meminfo.Slab)
	}
	if meminfo.SReclaimable != 500000 {
		t.Errorf("expected SReclaimable=500000, got %d", meminfo.SReclaimable)
	}
	if meminfo.SUnreclaim != 388888 {
		t.Errorf("expected SUnreclaim=388888, got %d", meminfo.SUnreclaim)
	}
}

func TestProcessMeminfoEmpty(t *testing.T) {
	meminfo, err := ProcessMeminfo([]byte(""))
	if err != nil {
		t.Fatalf("ProcessMeminfo failed on empty input: %v", err)
	}
	// All fields should be zero for empty input
	if meminfo.MemTotal != 0 {
		t.Errorf("expected MemTotal=0 for empty input, got %d", meminfo.MemTotal)
	}
}

func TestProcessMeminfoMinimal(t *testing.T) {
	minimal := `MemTotal:       8000000 kB
MemFree:        4000000 kB
`
	meminfo, err := ProcessMeminfo([]byte(minimal))
	if err != nil {
		t.Fatalf("ProcessMeminfo failed: %v", err)
	}

	if meminfo.MemTotal != 8000000 {
		t.Errorf("expected MemTotal=8000000, got %d", meminfo.MemTotal)
	}
	if meminfo.MemFree != 4000000 {
		t.Errorf("expected MemFree=4000000, got %d", meminfo.MemFree)
	}
}

func TestProcessMeminfoInvalidFormat(t *testing.T) {
	invalid := `MemTotal: notanumber kB
`
	_, err := ProcessMeminfo([]byte(invalid))
	if err == nil {
		t.Error("expected error for invalid meminfo format")
	}
}

func TestProcessMeminfoDirectMap(t *testing.T) {
	meminfo, err := ProcessMeminfo([]byte(sampleProcMeminfo))
	if err != nil {
		t.Fatalf("ProcessMeminfo failed: %v", err)
	}

	if meminfo.DirectMap4k != 234567 {
		t.Errorf("expected DirectMap4k=234567, got %d", meminfo.DirectMap4k)
	}
	if meminfo.DirectMap2M != 8765432 {
		t.Errorf("expected DirectMap2M=8765432, got %d", meminfo.DirectMap2M)
	}
	if meminfo.DirectMap1G != 9437184 {
		t.Errorf("expected DirectMap1G=9437184, got %d", meminfo.DirectMap1G)
	}
}

// Benchmark tests
func BenchmarkProcessMeminfo(b *testing.B) {
	data := []byte(sampleProcMeminfo)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ProcessMeminfo(data)
	}
}
