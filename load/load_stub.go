// +build !linux

package load

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/businessperformancetuning/perfcollector/parser"
	"github.com/inhies/go-bytesize"
)

var errNotLinux = errors.New("load generation is only supported on Linux")

// NumCores returns the number of CPU cores and the number of threads.
// This stub returns an error on non-Linux platforms.
func NumCores() (uint, uint, error) {
	return 0, 0, errNotLinux
}

// CPUStat takes CPU measurement.
// This stub returns an error on non-Linux platforms.
func CPUStat() (parser.Stat, error) {
	return parser.Stat{}, errNotLinux
}

// MeasureUnitsPerSecond is a stub for non-Linux platforms.
func MeasureUnitsPerSecond(ctx context.Context, cores, fine uint) (float64, error) {
	return 0, errNotLinux
}

// RMW Read-Modify-Write test stub. Always returns false for the continue condition.
func RMW(p *big.Int) bool {
	return false
}

// Prime test stub. Returns false on non-Linux platforms.
func Prime(p *big.Int) bool {
	return false
}

// ExecuteParallel is a stub for non-Linux platforms.
// Matches Linux signature: (ctx, maxDuration, n, workers uint64, f func(*big.Int) bool)
func ExecuteParallel(parent context.Context, maxDuration time.Duration, n, workers uint64, f func(*big.Int) bool) (time.Duration, uint64, error) {
	return 0, 0, errNotLinux
}

// UserWork is a stub for non-Linux platforms.
func UserWork(units int) time.Duration {
	return 0
}

// UserLoad is a stub for non-Linux platforms.
func UserLoad(d time.Duration) int {
	return 0
}

// UserIdle is a stub for non-Linux platforms.
func UserIdle(d time.Duration) {
}

// MeasureCombined is a stub for non-Linux platforms.
func MeasureCombined(userPercent, sysPercent float64, d time.Duration) (int, int, error) {
	return 0, 0, errNotLinux
}

// DiskRead is a stub for non-Linux platforms.
func DiskRead(parent context.Context, maxDuration time.Duration, filename string, units, size uint64) (time.Duration, uint64, error) {
	return 0, 0, errNotLinux
}

// DiskWrite is a stub for non-Linux platforms.
func DiskWrite(parent context.Context, maxDuration time.Duration, filename string, units, size uint64) (time.Duration, uint64, error) {
	return 0, 0, errNotLinux
}

// NetServerResult holds the result of a network server operation.
type NetServerResult struct {
	Duration   time.Duration
	Bytes      uint64
	Verb       string
	RemoteAddr string
}

// NetServer is a stub for non-Linux platforms.
// Matches Linux signature: (ctx, maxDuration, listen string) (NetServerResult, error)
func NetServer(parent context.Context, maxDuration time.Duration, listen string) (NetServerResult, error) {
	return NetServerResult{}, errNotLinux
}

// NetClient is a stub for non-Linux platforms.
// Matches Linux signature: (ctx, maxDuration, command, connect string, units uint, size bytesize.ByteSize)
func NetClient(parent context.Context, maxDuration time.Duration, command, connect string, units uint, size bytesize.ByteSize) (time.Duration, uint64, error) {
	return 0, 0, errNotLinux
}
