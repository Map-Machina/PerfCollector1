package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// FilesExists reports whether the named file or directory exists.
func FileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func ValidSystem(s string) bool {
	path := filepath.Clean(s)
	if !strings.HasPrefix(path, "/proc/") {
		return false
	}
	return FileExists(path)
}

func Measure(s string) ([]byte, error) {
	path := filepath.Clean(s)
	if !ValidSystem(path) {
		return nil, fmt.Errorf("invalid system: %v", path)
	}
	return ioutil.ReadFile(path)
}
