package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/businessperformancetuning/perfcollector/cmd/perfprocessord/journal"
	"github.com/businessperformancetuning/perfcollector/parser"
	"github.com/decred/dcrd/dcrutil"
	"github.com/jrick/flagfile"
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
	Verbose     bool
	SiteID      string
	Mode        string
	Cache       string
	SiteName    string
	License     string
	InputFile   string
	Output      string
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage of perfjournal:
  perfjournal [flags] action <args...>
Flags:
  -C value
        config file
  -v    verbose
  -V	Show version and exit
  --mode string
	Output mode: csv, json (default csv)
  --cache string
	filename of JSON file that caching data.
  --siteid unsigned integer
	Numerical site id, e.g. 1
  --sitename string
        Site name, e.g. "Evil Database Site"
  --license string
        License string, e.g. "6f37-6904-1f83-92f4-595a-0efd"
  --cache JSON
	JSON file that will cache values. This is used to create caches such as the NIC speed.
  --input string
	Input directory, e.g. ~/journal
  --output string
	Output file or directory depending on mode, - outputs to stdout in JSON mode
	e.g. ~/datadump.csv or ~/journal.json
`)
	os.Exit(2)
}

func (c *config) FlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("perfjournal", flag.ExitOnError)
	configParser := flagfile.Parser{AllowUnknown: false}
	c.Config = configParser.ConfigFlag(fs)
	fs.Var(c.Config, "C", "config file")
	fs.BoolVar(&c.ShowVersion, "V", false, "")
	fs.BoolVar(&c.Verbose, "v", false, "")
	fs.StringVar(&c.SiteID, "siteid", "", "")
	fs.StringVar(&c.Mode, "mode", "csv", "")
	fs.StringVar(&c.Cache, "cache", "", "")
	fs.StringVar(&c.SiteName, "sitename", "", "")
	fs.StringVar(&c.License, "license", "", "")
	fs.StringVar(&c.InputFile, "input", "", "")
	fs.StringVar(&c.Output, "output", "", "")
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

	if cfg.SiteID == "" {
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

	if cfg.Output == "" {
		fmt.Fprintln(os.Stderr, "Must provide --output")
		os.Exit(1)
	}
	cfg.Output = cleanAndExpandPath(cfg.Output)

	if cfg.Cache != "" {
		cfg.Cache = cleanAndExpandPath(cfg.Cache)
	}

	return cfg, fs.Args(), nil
}

var (
	// map key site_host_run_system
	previousCache = make(map[string]*journal.WrapPCCollection)

	// map key is filename
	fileCache = make(map[string]*os.File)
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
	case "/proc/loadavg":
		// XXX do notthing
	default:
		return "", fmt.Errorf("unsupported system: %v",
			wc.Measurement.System)
	}
	return "#site,host,timestamp," + mHdr, nil
}

var (
	reDuplex = regexp.MustCompile("/sys/class/net/[[:alnum:]]*/duplex")
	reSpeed  = regexp.MustCompile("/sys/class/net/[[:alnum:]]*/speed")
	reNIC    = regexp.MustCompile("(/[[:alnum:]]+){4}")
)

func createNetCache(cache map[string]string) (map[string]parser.NIC, error) {
	// Incoming data example:
	// 0 0 0 /sys/class/net/lo/speed
	// 0 1 0 /sys/class/net/eno1/duplex
	// Translate cache to NIC cache
	r := make(map[string]parser.NIC)
	for k, v := range cache {
		if !(reDuplex.MatchString(k) || reSpeed.MatchString(k)) {
			// Skip non NIC items we don't understand.
			continue
		}

		var (
			site, host, run uint64
			system          string
		)
		n, err := fmt.Sscanf(k, "%d %d %d %s", &site, &host, &run, &system)
		if err != nil || n != 4 {
			fmt.Printf("invalid cache entry: %v", k)
			continue
		}
		//fmt.Printf("%v %v %v %v -- %v %v\n", site, host, run, system, n, err)

		// \/sys\/class\/net\/[[:print:]]*\/duplex
		// ([[:alnum:]]*){4}

		// Returns 2 matches, we want index 1 which is prefixed with a /
		s := reNIC.FindStringSubmatch(k)
		if len(s) != 2 {
			continue
		}
		nic := strings.Trim(s[1], "/")
		if nic == "" {
			continue
		}

		// Skip localhost
		if nic == "lo" {
			continue
		}

		key := fmt.Sprintf("%v %v %v %v", site, host, run, nic)
		cc, ok := r[key]
		if !ok {
			cc = parser.NIC{}
		}

		switch filepath.Base(k) {
		case "duplex":
			cc.Duplex = strings.TrimSpace(v)
		case "speed":
			var err error
			cc.Speed, err = strconv.ParseUint(strings.TrimSpace(v),
				10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid speed %v:%v %v: %v",
					site, host, nic, err)
			}
		default:
			return nil, fmt.Errorf("unsupported netcache item: %v",
				filepath.Base(k))
		}

		// Overwrite old value
		r[key] = cc
	}

	return r, nil
}

func csv(cfg *config, cur *journal.WrapPCCollection, cache map[string]parser.NIC) error {
	if strconv.Itoa(int(cur.Site)) != cfg.SiteID {
		// File should not have decrypted
		return fmt.Errorf("unexpected site: %v", cur.Site)
	}

	// XXX work around trailing /
	// XXX FIXME
	if cur.Measurement.System == "/proc/net/dev/" {
		cur.Measurement.System = "/proc/net/dev"
	}
	if cur.Measurement.System == "/proc/loadavg" {
		return nil
	}

	// Construct previousCache map key
	name := strconv.FormatUint(cur.Site, 10) + "_" +
		strconv.FormatUint(cur.Host, 10) + "_" +
		strconv.FormatUint(cur.Run, 10) + "_" +
		cur.Measurement.System
	var (
		prev *journal.WrapPCCollection
		ok   bool
	)
	if prev, ok = previousCache[name]; !ok {
		// Store as previous measurement.
		previousCache[name] = cur
		return nil
	}

	var (
		f *os.File
	)
	filename := filepath.Join(cfg.Output, cur.Measurement.System)
	if f, ok = fileCache[filename]; !ok {
		// File not seen, create file, write header and cache

		// Create dirs.
		dir := filepath.Join(cfg.Output,
			filepath.Dir(cur.Measurement.System))
		err := os.MkdirAll(dir, 0754)
		if err != nil {
			return err
		}

		// Overwrite old files.
		file := filepath.Join(cfg.Output, cur.Measurement.System)
		if cfg.Verbose {
			fmt.Printf("open %v\n", file)
		}
		f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY|os.O_CREATE|
			os.O_TRUNC, 0644)
		if err != nil {
			return err
		}
		fileCache[file] = f

		// Fetch header
		header, err := constructHeader(cur)
		if err != nil {
			return err
		}

		_, err = fmt.Fprintf(f, "%v\n", header)
		return err
	}

	switch cur.Measurement.System {
	case "/proc/stat":
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

		// Store cur into previousCache
		previousCache[name] = cur

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
		r, err := parser.CubeNetDev(cur.Site, cur.Host, cur.Run,
			0, /* timestamp */
			0, /* start */
			0, /* duration */
			p, c, tvi, cache)
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

		// Store cur into previousCache
		previousCache[name] = cur

	case "/proc/diskstats":
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

		// Store cur into previousCache
		previousCache[name] = cur

	default:
	}

	return nil
}

var jsonFile *os.File

func doJSON(cfg *config, cur *journal.WrapPCCollection) error {
	b, err := json.Marshal(cur)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(jsonFile, "%v\n", string(b))
	return err
}

func readCache(filename string) (map[string]string, error) {
	fc, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("cache: %v", err)
	}
	defer fc.Close()

	cache := make(map[string]string, 16)
	d := json.NewDecoder(fc)
	entry := 0
	for {
		var wc journal.WrapPCCollection
		err := d.Decode(&wc)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode cache: %v %v",
				entry, err)
		}
		k := fmt.Sprintf("%v %v %v %v", wc.Site, wc.Host, wc.Run,
			wc.Measurement.System)
		cache[k] = wc.Measurement.Measurement
		entry++
	}

	return cache, nil
}

func _main() error {
	cfg, _, err := loadConfig()
	if err != nil {
		return err
	}

	// Generate journal key from license material. There is no function for
	// this in order to obfuscate this terrible trick.
	aead, err := journal.CreateAEAD(cfg.License, cfg.SiteID, cfg.SiteName)
	if err != nil {
		return fmt.Errorf("could not setup aead: %v", err)
	}

	type modeT int
	const (
		modeInvalid = 0
		modeCSV     = 1
		modeJSON    = 2
	)
	mode := modeInvalid

	switch cfg.Mode {
	case "csv":
		mode = modeCSV
	case "json":
		mode = modeJSON
	default:
		return fmt.Errorf("invalid mode: %v", cfg.Mode)
	}

	if mode == modeCSV {
		if !fileExists(cfg.Output) {
			return fmt.Errorf("output dir does not exist: %v",
				cfg.Output)
		}
	}

	// Read cache, XXX this is meanigless in JSON mode
	var netCache map[string]parser.NIC
	if cfg.Cache != "" {
		cache, err := readCache(cfg.Cache)
		if err != nil {
			return err
		}
		netCache, err = createNetCache(cache)
		if err != nil {
			return err
		}
	}

	// Open input file
	f, err := os.Open(cfg.InputFile)
	if err != nil {
		return fmt.Errorf("input: %v", err)
	}

	// Pre-process
	switch mode {
	case modeJSON:
		if cfg.Output == "-" {
			jsonFile = os.Stdout
		} else {
			jsonFile, err = os.Create(cfg.Output)
			if err != nil {
				return err
			}
		}
	}

	// Process
	entries := 0
	start := time.Now()
	s := time.Now().Add(5 * time.Second)
	for {
		wc, err := journal.ReadEncryptedJournalEntry(f, aead)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch mode {
		case modeCSV:
			if err != nil {
				return fmt.Errorf("createCache: %v", err)
			}

			err = csv(cfg, wc, netCache)
			if err != nil {
				return err
			}

		case modeJSON:
			err = doJSON(cfg, wc)
			if err != nil {
				return err
			}
		}

		entries++

		if cfg.Verbose && time.Now().After(s) {
			fmt.Printf("Entries processed: %v\n", entries)
			s = time.Now().Add(5 * time.Second)
		}
	}

	// Post process.
	switch mode {
	case modeCSV:
		// Close all files in file cache.
		for _, v := range fileCache {
			v.Close()
		}
	}

	if cfg.Verbose {
		end := time.Now()
		fmt.Printf("Total entries processed: %v in %v\n",
			entries, end.Sub(start))
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
