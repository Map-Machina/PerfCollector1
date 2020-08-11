package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// FilesExists reports whether the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

//func SupportedSystem(s string) bool {
//	switch s {
//	case "stat", "meminfo", "net/dev", "diskstats":
//		return true
//	}
//	return false
//}

func ValidSystem(s string) bool {
	// XXX we really need to reject some stuff from proc and sys. There
	// definitely is data that can leak.
	path := filepath.Clean(s)
	if !(strings.HasPrefix(path, "/proc/") ||
		strings.HasPrefix(path, "/sys/")) {
		return false
	}
	return fileExists(path)
}

func Measure(s string) ([]byte, error) {
	path := filepath.Clean(s)
	if !ValidSystem(path) {
		return nil, fmt.Errorf("invalid system: %v", path)
	}
	return ioutil.ReadFile(path)
}
