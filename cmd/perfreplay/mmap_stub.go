// +build !linux

package main

import "errors"

var errMmapNotLinux = errors.New("mmap is only supported on Linux")

// mmap allocates size bytes using memory mapping.
// This stub returns an error on non-Linux platforms.
func mmap(size uint64) ([]byte, error) {
	return nil, errMmapNotLinux
}

// munmap returns a previously allocated mmap region back to the kernel.
// This stub returns an error on non-Linux platforms.
func munmap(p []byte) error {
	return errMmapNotLinux
}
