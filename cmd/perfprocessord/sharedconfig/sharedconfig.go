package sharedconfig

import (
	"path/filepath"
	"os"
)

const (
	DefaultConfigFilename  = "perfprocessord.conf"
	DefaultDataDirname     = "data"
	DefaultJournalFilename = "journal"
	DefaultSocketFilename  = ".socket"
)

var (
	// DefaultHomeDir points to logdump ui daemon home directory
	DefaultHomeDir = filepath.Join(os.Getenv("HOME"), ".perfprocessord")

	// DefaultConfigFile points to perfcollectord daemon configuration file
	DefaultConfigFile = filepath.Join(DefaultHomeDir, DefaultConfigFilename)

	// DefaultDataDir points to perfcollectord daemon default data directory.
	DefaultDataDir = filepath.Join(DefaultHomeDir, DefaultDataDirname)

	// DefaultSocket points to perfcollectord default socket filename.
	DefaultSocketFile = filepath.Join(DefaultHomeDir, DefaultSocketFilename)
)
