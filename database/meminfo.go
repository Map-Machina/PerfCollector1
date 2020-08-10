package database

type Meminfo struct {
	MemFree       uint64  // kbmemfree
	MemAvailable  uint64  // kbavail
	MemUsed       uint64  // kbmemused
	PercentUsed   float64 // %memused
	Buffers       uint64  // kbbuffers
	Cached        uint64  // kbcached
	Commit        uint64  // kbcommit
	PercentCommit float64 // %commit
	Active        uint64  // kbactive
	Inactive      uint64  // kbinactive
	Dirty         uint64  // kbdirty
}
