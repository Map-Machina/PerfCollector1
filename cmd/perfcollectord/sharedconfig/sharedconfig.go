package sharedconfig

import (
	"path/filepath"
	"os"
)

const (
	DefaultConfigFilename = "perfcollectord.conf"
	DefaultDataDirname    = "data"
)

var (
	// DefaultHomeDir points to logdump ui daemon home directory
	DefaultHomeDir = filepath.Join(os.Getenv("$HOME"),".perfcollectord")

	// DefaultConfigFile points to perfcollectord daemon configuration file
	DefaultConfigFile = filepath.Join(DefaultHomeDir, DefaultConfigFilename)

	// DefaultDataDir points to perfcollectord daemon default data directory.
	DefaultDataDir = filepath.Join(DefaultHomeDir, DefaultDataDirname)
)
