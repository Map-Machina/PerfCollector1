package main

import (
	"context"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/businessperformancetuning/perfcollector/load"
	"github.com/businessperformancetuning/perfcollector/util"
	"github.com/decred/dcrd/dcrutil"
	"github.com/jrick/flagfile"
)

var (
	defaultHomeDir    = dcrutil.AppDataDir("perfload", false)
	defaultConfigFile = filepath.Join(defaultHomeDir, "perfload.conf")
)

func versionString() string {
	return "1.0.0"
}

type config struct {
	Config      flag.Value
	ShowVersion bool
	Verbose     bool
	Mode        string
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage of perfload:
  perfload [flags] action <args...>
Flags:
  -C value
        config file
  -v    verbose
  -V	Show version and exit
Actions:
  load findprimes=<number>
	Measure duration to find the provided number of primes
`)
	os.Exit(2)
}

func (c *config) FlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("perfload", flag.ExitOnError)
	configParser := flagfile.Parser{AllowUnknown: false}
	c.Config = configParser.ConfigFlag(fs)
	fs.Var(c.Config, "C", "config file")
	fs.BoolVar(&c.ShowVersion, "V", false, "")
	fs.BoolVar(&c.Verbose, "v", false, "")
	fs.StringVar(&c.Mode, "findprimes", "", "")
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

	return cfg, fs.Args(), nil
}

func cpuLoad(ctx context.Context, a map[string]string) error {
	// Work units to perform
	units, err := util.ArgAsUint("units", a)
	if err != nil {
		return err
	}

	// Load type
	loadType, err := util.ArgAsString("type", a)
	if err != nil {
		return err
	}
	var loadFunc func(*big.Int) bool
	switch loadType {
	case "rmw":
		loadFunc = load.RMW
	case "findprimes":
		loadFunc = load.Prime
	default:
		return fmt.Errorf("unknown type: %v", loadType)
	}

	// Number of workers
	workers, err := util.ArgAsUint("workers", a)
	if err != nil {
		workers = uint(runtime.NumCPU())
		fmt.Printf("workers defaulting to: %v\n", workers)
	}

	timeout, err := util.ArgAsDuration("timeout", a)
	if err != nil {
		timeout = 60 * time.Second
		fmt.Printf("timeout defaulting to: %v\n", timeout)
	}

	d, actual, err := load.ExecuteParallel(ctx, timeout,
		uint64(units), uint64(workers), loadFunc)
	if err != nil {
		return fmt.Errorf("timeout, units completed %v/%v: %v",
			actual, units, err)
	}
	fmt.Printf("units executed %v in %v\n", actual, d)

	return nil
}

func _main() error {
	cfg, args, err := loadConfig()
	if err != nil {
		return err
	}
	_ = cfg

	// Deal with command line
	a, err := util.ParseArgs(args)
	if err != nil {
		return err
	}

	ctx := context.Background()

	switch args[0] {
	case "load":
		return cpuLoad(ctx, a)

	default:
		return fmt.Errorf("unknwon action: %v", args[0])
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
