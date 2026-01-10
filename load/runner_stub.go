// +build !linux

package load

import (
	"context"
	"errors"

	"github.com/businessperformancetuning/perfcollector/database"
)

var errRunnerNotLinux = errors.New("worker pool is only supported on Linux")

// LoadError contains error information for load generation.
type LoadError struct {
	msg string
}

func (l LoadError) Error() string {
	return l.msg
}

// WorkLoad describes a unit of work for CPU load generation.
type WorkLoad struct {
	units int
}

// Worker manages a pool of CPU load workers.
type Worker struct {
	virtualCores uint
}

// NewWorkerPool creates a new worker pool.
// This stub returns an error on non-Linux platforms.
func NewWorkerPool(ctx context.Context, frequency int) (*Worker, error) {
	return nil, errRunnerNotLinux
}

// WorkCPU executes CPU work based on the provided stats.
// This stub returns an error on non-Linux platforms.
func (w *Worker) WorkCPU(s []database.Stat) (int, error) {
	return 0, errRunnerNotLinux
}

// WaitForExit waits for all workers to exit.
// This stub does nothing on non-Linux platforms.
func (w *Worker) WaitForExit() {
}

// Train calibrates the worker pool for different CPU loads.
// This stub returns an error on non-Linux platforms.
func (w *Worker) Train(verbose bool) (map[int]int, error) {
	return nil, errRunnerNotLinux
}
