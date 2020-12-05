package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/businessperformancetuning/perfcollector/cmd/perfcpumeasure/training"
	"github.com/businessperformancetuning/perfcollector/load"
	"github.com/decred/dcrd/dcrutil"
	"github.com/jrick/flagfile"
)

var (
	defaultHomeDir    = dcrutil.AppDataDir("perfperfcpumeasure", false)
	defaultConfigFile = filepath.Join(defaultHomeDir, "perfcpumeasure.conf")
)

func versionString() string {
	return "1.0.0"
}

type config struct {
	Config      flag.Value
	ShowVersion bool
	Verbose     bool
	Site        uint64
	Host        uint64
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage of perfreplay:
  perfreplay [flags] action <args...>
Flags:
  -C value
        config file
  -v    verbose
	Output is sent to stderr.
  -V	Show version and exit
  --siteid unsigned integer
	Numerical site id, e.g. 1
  --host unsigned integer
	Host ID that is being measured.
`)
	os.Exit(2)
}

const invalidHostID = 0xffffffffffffffff

func (c *config) FlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("perfreplay", flag.ExitOnError)
	configParser := flagfile.Parser{AllowUnknown: false}
	c.Config = configParser.ConfigFlag(fs)
	fs.Var(c.Config, "C", "config file")
	fs.BoolVar(&c.ShowVersion, "V", false, "")
	fs.BoolVar(&c.Verbose, "v", false, "")
	fs.Uint64Var(&c.Site, "siteid", 0, "")
	fs.Uint64Var(&c.Host, "host", invalidHostID, "")
	fs.Usage = usage
	return fs
}

// fileExists reports whether the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// cleanAndExpandPath expands environment variables and leading ~ in the
// passed path, cleans the result, and returns it.
func cleanAndExpandPath(path string) string {
	// Expand initial ~ to OS specific home directory.
	if strings.HasPrefix(path, "~") {
		homeDir := filepath.Dir(defaultHomeDir)
		path = strings.Replace(path, "~", homeDir, 1)
	}

	// NOTE: The os.ExpandEnv doesn't work with Windows-style %VARIABLE%,
	// but they variables can still be expanded via POSIX-style $VARIABLE.
	return filepath.Clean(os.ExpandEnv(path))
}

// loadConfig initializes and parses the config using a config file and command
// line options.
//
// The configuration proceeds as follows:
// 	1) Start with a default config with sane settings
// 	2) Pre-parse the command line to check for an alternative config file
// 	3) Load configuration file overwriting defaults with any specified options
// 	4) Parse CLI options and overwrite/add any specified options
//
// The above results in functioning properly without any config settings
// while still allowing the user to override settings with config files and
// command line options.  Command line options always take precedence.
func loadConfig() (*config, []string, error) {
	// Default config.
	cfg := &config{}
	fs := cfg.FlagSet()
	args := os.Args[1:]

	// Determine config file to read (if any).  When -C is the first
	// parameter, configure flags from the specified config file rather than
	// using the application default path.  Otherwise the default config
	// will be parsed if the file exists.
	//
	// If further -C options are specified in later arguments, the config
	// file parameter is used to modify the current state of the config.
	//
	// If you want to read the application default config first, and other
	// configs later, explicitly specify the default path with the first
	// flag argument.
	if len(args) >= 2 && args[0] == "-C" {
		err := cfg.Config.Set(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid value %q for flag "+
				"-C: %s\n", args[1], err)
			os.Exit(1)
		}
		args = args[2:]
	} else if fileExists(defaultConfigFile) {
		err := cfg.Config.Set(defaultConfigFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	fs.Parse(args)

	// Show the version and exit if the version flag was specified.
	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))
	if cfg.ShowVersion {
		fmt.Printf("%s version %s (Go version %s %s/%s)\n", appName,
			versionString(), runtime.Version(), runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}

	if cfg.Site == 0 {
		fmt.Fprintln(os.Stderr, "Must provide --siteid")
		os.Exit(1)
	}

	if cfg.Host == invalidHostID {
		fmt.Fprintln(os.Stderr, "Must provide --host")
		os.Exit(1)
	}

	return cfg, fs.Args(), nil
}

// loadPercentage figures out how many units can run in the provided percentage
// of a second.
//func loadPercentage(percent uint) (uint,error) {
//}

func _main() error {
	cfg, _, err := loadConfig()
	if err != nil {
		return err
	}
	ctx := context.Background()
	wp, err := load.NewWorkerPool(ctx, 1)
	if err != nil {
		return err
	}
	measurement, err := wp.Train(cfg.Verbose)
	if err != nil {
		return err
	}

	// Dump JSON
	keys := make([]int, len(measurement))
	i := 0
	for k := range measurement {
		keys[i] = k
		i++
	}
	sort.Ints(keys)

	je := json.NewEncoder(os.Stdout)
	for _, k := range keys {
		err = je.Encode(training.Training{
			Busy:   uint(k),
			Units:  uint(measurement[k]),
			SiteID: cfg.Site,
			Host:   cfg.Host,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func main() {
	err := _main()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
