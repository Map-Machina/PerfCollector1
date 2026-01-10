// +build linux

package main

import "syscall"

// This is linux specific.

const (
	// pageSize has to match the arch in order to paint every page to force
	// allocation.
	pageSize = 4096

	// Default linux mmap flags.
	mmapFlags = syscall.MAP_PRIVATE | syscall.MAP_ANONYMOUS
)

// mmap allocates size bytes. It is recommended that size is divisible by
// pageSize.
func mmap(size uint64) ([]byte, error) {
	buffer, err := syscall.Mmap(0, 0, int(size),
		syscall.PROT_READ|syscall.PROT_WRITE,
		mmapFlags,
	)
	if err != nil {
		return nil, err
	}

	// Paint memory or it wont be allocated
	for i := uint64(0); i < size; i += pageSize {
		buffer[i] = 0xff
	}

	return buffer, nil
}

// munmap return a previously allocated mmap region back to the kernel.
func munmap(p []byte) error {
	return syscall.Munmap(p)
}
