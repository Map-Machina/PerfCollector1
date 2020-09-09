package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/businessperformancetuning/perfcollector/cmd/perfprocessord/journal"
	"github.com/businessperformancetuning/perfcollector/parser"
	"github.com/decred/dcrd/dcrutil"
	"github.com/jrick/flagfile"
	cp "golang.org/x/crypto/chacha20poly1305"
)

var (
	defaultHomeDir    = dcrutil.AppDataDir("perfjournal", false)
	defaultConfigFile = filepath.Join(defaultHomeDir, "perfjournal.conf")
)

func versionString() string {
	return "1.0.0"
}

type config struct {
	Config      flag.Value
	ShowVersion bool
	SiteID      uint
	SiteName    string
	License     string
	InputFile   string
	OutputDir   string
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage of perflicense:
  perflicense [flags] action <args...>
Flags:
  -C value
        config file
  -v    show version and exit
  --siteid unsigned integer
	Numerical site id, e.g. 1
  --sitename string
        Site name, e.g. "Evil Database Site"
  --license string
        License string, e.g. "6f37-6904-1f83-92f4-595a-0efd"
  --input string
	Input directory, e.g. ~/journal
  --output string
	Output directory, e.g. ~/datadump
`)
	os.Exit(2)
}

func (c *config) FlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("perflicense", flag.ExitOnError)
	configParser := flagfile.Parser{AllowUnknown: false}
	c.Config = configParser.ConfigFlag(fs)
	fs.Var(c.Config, "C", "config file")
	fs.BoolVar(&c.ShowVersion, "v", false, "")
	fs.UintVar(&c.SiteID, "siteid", 0, "")
	fs.StringVar(&c.SiteName, "sitename", "", "")
	fs.StringVar(&c.License, "license", "", "")
	fs.StringVar(&c.InputFile, "input", "", "")
	fs.StringVar(&c.OutputDir, "output", "", "")
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

	if cfg.SiteID == 0 {
		fmt.Fprintln(os.Stderr, "Must provide --siteid")
		os.Exit(1)
	}

	if cfg.SiteName == "" {
		fmt.Fprintln(os.Stderr, "Must provide --sitename")
		os.Exit(1)
	}

	if cfg.License == "" {
		fmt.Fprintln(os.Stderr, "Must provide --license")
		os.Exit(1)
	}

	if cfg.InputFile == "" {
		fmt.Fprintln(os.Stderr, "Must provide --input")
		os.Exit(1)
	}
	cfg.InputFile = cleanAndExpandPath(cfg.InputFile)

	if cfg.OutputDir == "" {
		fmt.Fprintln(os.Stderr, "Must provide --output")
		os.Exit(1)
	}
	cfg.OutputDir = cleanAndExpandPath(cfg.OutputDir)

	return cfg, fs.Args(), nil
}

var (
	// map key site_host_run_system
	seen = make(map[string]*journal.WrapPCCollection)
)

func constructHeader(wc *journal.WrapPCCollection) (string, error) {
	var mHdr string
	switch wc.Measurement.System {
	case "/proc/stat":
		mHdr = "CPU,%usr,%nice,%system,%iowait,%steal,%idle"
	case "/proc/meminfo":
		mHdr = "kbmemfree,kbavail,kbmemused,%memused,kbbuffers," +
			"kbcached,kbcommit,%commit,kbactive,kbinact," +
			"kbdirty"
	case "/proc/net/dev":
		mHdr = "IFACE,rxpck/s,txpck/s,rxkB/s,txkB/s,rxcmp/s," +
			"txcmp/s,rxmcst/s,%ifutil"
	case "/proc/diskstats":
		mHdr = "DEV,tps,rtps,wtps,dtps,bread/s,bwrtn/s,bdscd/s"
	//case "/proc/loadavg":
	default:
		return "", fmt.Errorf("unsupported system: %v",
			wc.Measurement.System)
	}
	return "#site,host,timestamp," + mHdr, nil
}

func csv(cfg *config, cur *journal.WrapPCCollection) error {
	if cur.Site != uint64(cfg.SiteID) {
		// File should not have decrypted
		return fmt.Errorf("unexpected site: %v", cur.Site)
	}

	// Construct map key
	name := strconv.FormatUint(cur.Site, 10) + "_" +
		strconv.FormatUint(cur.Host, 10) + "_" +
		strconv.FormatUint(cur.Run, 10) + "_" +
		cur.Measurement.System
	var (
		prev *journal.WrapPCCollection
		ok   bool
	)
	if prev, ok = seen[name]; !ok {
		// Write header and store previous measurement.
		seen[name] = cur

		// Create dirs. There is some overlap but for now just do it
		// all the time. Cache this and be more clever later.
		dir := filepath.Join(cfg.OutputDir,
			filepath.Dir(cur.Measurement.System))
		err := os.MkdirAll(dir, 0754)
		if err != nil {
			return err
		}

		// Fetch header
		header, err := constructHeader(cur)
		if err != nil {
			return err
		}

		// Overwrite old files.
		file := filepath.Join(cfg.OutputDir, cur.Measurement.System)
		f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_TRUNC,
			0644)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = fmt.Fprintf(f, "%v\n", header)
		return err
	}

	// Open file
	// XXX cache the open files
	file := filepath.Join(cfg.OutputDir, cur.Measurement.System)
	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	switch cur.Measurement.System {
	case "/proc/stat":
		// XXX don't convert prev again, fix
		p, err := parser.ProcessStat([]byte(prev.Measurement.Measurement))
		if err != nil {
			return fmt.Errorf("ProcessStat prev: %v", err)
		}
		c, err := parser.ProcessStat([]byte(cur.Measurement.Measurement))
		if err != nil {
			return fmt.Errorf("ProcessStat cur: %v", err)
		}
		// Ignore database bits
		r, err := parser.CubeStat(0, 0, 0, 0, &p, &c)
		if err != nil {
			return fmt.Errorf("CubeStat: %v", err)
		}

		// Write out records
		for k := range r {
			fmt.Fprintf(f, "%v,%v,%v,%v,%v,%v,%v,%v,%v,%v\n",
				cur.Site, cur.Host, cur.Measurement.Timestamp.Unix(),
				r[k].CPU, r[k].UserT, r[k].Nice, r[k].System,
				r[k].IOWait, r[k].Steal, r[k].Idle)
		}

		// Mark as seen
		seen[name] = cur

	case "/proc/meminfo":
		// MemInfo isn't differential so just toss first measurement.
		c, err := parser.ProcessMeminfo([]byte(cur.Measurement.Measurement))
		if err != nil {
			return fmt.Errorf("ProcessMeminfo cur: %v", err)
		}
		// Ignore database bits
		r, err := parser.CubeMeminfo(0, 0, 0, 0, &c)
		if err != nil {
			return fmt.Errorf("CubeMeminfo: %v", err)
		}

		// Write out records
		fmt.Fprintf(f, "%v,%v,%v,%v,%v,%v,%v,%v,%v,%v,%v,%v,%v,%v\n",
			cur.Site, cur.Host, cur.Measurement.Timestamp.Unix(),
			r.MemFree, r.MemAvailable, r.MemUsed, r.PercentUsed,
			r.Buffers, r.Cached, r.Commit, r.PercentCommit,
			r.Active, r.Inactive, r.Dirty)

	case "/proc/net/dev":
		// XXX don't convert prev again, fix
		p, err := parser.ProcessNetDev([]byte(prev.Measurement.Measurement))
		if err != nil {
			return fmt.Errorf("ProcessNetDev prev: %v", err)
		}
		c, err := parser.ProcessNetDev([]byte(cur.Measurement.Measurement))
		if err != nil {
			return fmt.Errorf("ProcessNetDev cur: %v", err)
		}
		// Ignore database bits
		tvi := uint64(cur.Measurement.Frequency.Seconds()) *
			parser.UserHZ
		// XXX there is no nic cache here, fix
		r, err := parser.CubeNetDev(0, 0, 0, 0, p, c, tvi, nil)
		if err != nil {
			return fmt.Errorf("CubeNetDev: %v", err)
		}

		// Write out records
		for k := range r {
			fmt.Fprintf(f, "%v,%v,%v,%v,%v,%v,%v,%v,%v,%v,%v,%v\n",
				cur.Site, cur.Host, cur.Measurement.Timestamp.Unix(),
				r[k].Name, r[k].RxPackets, r[k].TxPackets,
				r[k].RxKBytes, r[k].TxKBytes, r[k].RxCompressed,
				r[k].TxCompressed, r[k].RxMulticast, r[k].IfUtil)
		}

		// Mark as seen
		seen[name] = cur

	case "/proc/diskstats":
		// XXX don't convert prev again, fix
		p, err := parser.ProcessDiskstats([]byte(prev.Measurement.Measurement))
		if err != nil {
			return fmt.Errorf("ProcessDiskstats prev: %v", err)
		}
		c, err := parser.ProcessDiskstats([]byte(cur.Measurement.Measurement))
		if err != nil {
			return fmt.Errorf("ProcessDiskstats cur: %v", err)
		}
		// Ignore database bits
		tvi := uint64(cur.Measurement.Frequency.Seconds()) *
			parser.UserHZ
		// XXX there is no nic cache here, fix
		r, err := parser.CubeDiskstats(0, 0, 0, 0, p, c, tvi)
		if err != nil {
			return fmt.Errorf("CubeDiskstats: %v", err)
		}

		// Write out records
		for k := range r {
			fmt.Fprintf(f, "%v,%v,%v,%v,%v,%v,%v,%v,%v,%v,%v\n",
				cur.Site, cur.Host, cur.Measurement.Timestamp.Unix(),
				r[k].Name, r[k].Tps, r[k].Rtps, r[k].Wtps,
				r[k].Dtps, r[k].Bread, r[k].Bwrtn, r[k].Bdscd)
		}

		// Mark as seen
		seen[name] = cur

	default:
	}

	return nil
}

func _main() error {
	cfg, _, err := loadConfig()
	if err != nil {
		return err
	}

	// Generate journal key from license material. There is no function for
	// this in order to obfudcate this terrible trick.
	mac := hmac.New(sha256.New, []byte(cfg.License))
	mac.Write([]byte(strconv.FormatUint(uint64(cfg.SiteID), 10)))
	mac.Write([]byte(cfg.SiteName))
	aead, err := cp.NewX(mac.Sum(nil))
	if err != nil {
		return err
	}

	f, err := os.Open(cfg.InputFile)
	if err != nil {
		return fmt.Errorf("input file %v", err)
	}

	type modeT int
	const modeCSV = 1
	mode := modeCSV
	if mode == modeCSV {
		if !fileExists(cfg.OutputDir) {
			return fmt.Errorf("output dir does not exist: %v",
				cfg.OutputDir)
		}
	}

	entries := 0
	for {
		wc, err := journal.ReadEncryptedJournalEntry(f, aead)
		if err != nil {
			if err == io.EOF {
				fmt.Printf("Processed raw entries: %v\n",
					entries)
				return nil
			}
			return err
		}

		switch mode {
		case modeCSV:
			err = csv(cfg, wc)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown mode: %v", mode)
		}

		entries++
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
