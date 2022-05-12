package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/businessperformancetuning/perfcollector/cmd/perfcpumeasure/training"
	"github.com/businessperformancetuning/perfcollector/cmd/perfprocessord/journal"
	"github.com/businessperformancetuning/perfcollector/database"
	"github.com/businessperformancetuning/perfcollector/load"
	"github.com/businessperformancetuning/perfcollector/parser"
	"github.com/businessperformancetuning/perfcollector/util"
	"github.com/dustin/go-humanize"
	"github.com/jrick/flagfile"
	"github.com/juju/loggo"
)

const (
	defaultLogging = "prp=INFO"
)

var (
	defaultHomeDir    = filepath.Join(os.Getenv("HOME"), "perfreplay")
	defaultConfigFile = filepath.Join(defaultHomeDir, "perfreplay.conf")

	log = loggo.GetLogger("prp")
)

func versionString() string {
	return "1.0.0"
}

type config struct {
	Config      flag.Value
	ShowVersion bool
	Verbose     bool
	Log         string
	Cache       string
	SiteName    string
	License     string
	InputFile   string
	//Output      string
	Training   string
	DiskMapper string
	Site       uint64
	Host       uint64
	Run        uint64
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage of perfreplay:
  perfreplay [flags] action <args...>
Flags:
  -C value
        config file
  -v    verbose
  -V	Show version and exit
  --cache string
	filename of JSON file that caching data.
  --siteid unsigned integer
	Numerical site id, e.g. 1
  --sitename string
        Optional site name, e.g. "Evil Database Site". if it is not used the tool assumes an unencrypted journal.
  --license string
        Optional license string, e.g. "6f37-6904-1f83-92f4-595a-0efd". If it is not used the tool assumes an unencrypted journal.
  --cache JSON
	JSON file that will cache values. This is used to create caches such as the NIC speed.
  --host unsigned integer
	Host ID that is being replayed.
  --run unsigned integer
	Run ID that is being replayed.
  --input string
	Input file, e.g. ~/journal
  --output string
	Output file, e.g. ~/replay.json (NOT USED AT THIS TIME)
  --training string
	Training data file, e.g. ~/training.json
  --diskmapper string
	Disk mapper data file, e.g. ~/diskmapper.json
`)
	os.Exit(2)
}

func (c *config) FlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("perfreplay", flag.ExitOnError)
	configParser := flagfile.Parser{AllowUnknown: false}
	c.Config = configParser.ConfigFlag(fs)
	fs.Var(c.Config, "C", "config file")
	fs.BoolVar(&c.ShowVersion, "V", false, "")
	fs.BoolVar(&c.Verbose, "v", false, "")
	fs.StringVar(&c.Log, "log", defaultLogging, "")
	fs.StringVar(&c.Cache, "cache", "", "")
	fs.StringVar(&c.SiteName, "sitename", "", "")
	fs.StringVar(&c.License, "license", "", "")
	fs.StringVar(&c.InputFile, "input", "", "")
	//fs.StringVar(&c.Output, "output", "", "")
	fs.StringVar(&c.Training, "training", "", "")
	fs.StringVar(&c.DiskMapper, "diskmapper", "", "")
	fs.Uint64Var(&c.Site, "siteid", 0, "")
	fs.Uint64Var(&c.Host, "host", 0, "")
	fs.Uint64Var(&c.Run, "run", 0, "")
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
		fmt.Fprintln(os.Stderr, "Must provide --siteid (not 0)")
		os.Exit(1)
	}

	if cfg.InputFile == "" {
		fmt.Fprintln(os.Stderr, "Must provide --input")
		os.Exit(1)
	}
	cfg.InputFile = cleanAndExpandPath(cfg.InputFile)

	//if cfg.Output == "" {
	//	fmt.Fprintln(os.Stderr, "Must provide --output")
	//	os.Exit(1)
	//}
	//cfg.Output = cleanAndExpandPath(cfg.Output)

	if cfg.Training == "" {
		fmt.Fprintln(os.Stderr, "Must provide --training")
		os.Exit(1)
	}
	cfg.Training = cleanAndExpandPath(cfg.Training)

	if cfg.Cache != "" {
		cfg.Cache = cleanAndExpandPath(cfg.Cache)
	}

	return cfg, fs.Args(), nil
}

var (
	// map key site_host_run_system
	previousCache = make(map[string]*journal.WrapPCCollection)
)

func parse(cfg *config, cur *journal.WrapPCCollection, cache map[string]parser.NIC) (interface{}, error) {
	if cur.Site != cfg.Site {
		// File should not have decrypted
		return nil, fmt.Errorf("unexpected site: %v", cur.Site)
	}

	// Work around trailing /
	cur.Measurement.System = strings.TrimRight(cur.Measurement.System, "/")

	// Ignore loadavg for now
	if cur.Measurement.System == "/proc/loadavg" {
		return nil, nil
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
		return nil, nil
	}

	var record interface{}
	switch cur.Measurement.System {
	case "/proc/stat":
		p, err := parser.ProcessStat([]byte(prev.Measurement.Measurement))
		if err != nil {
			return nil, fmt.Errorf("ProcessStat prev: %v", err)
		}
		c, err := parser.ProcessStat([]byte(cur.Measurement.Measurement))
		if err != nil {
			return nil, fmt.Errorf("ProcessStat cur: %v", err)
		}
		// Ignore database bits
		record, err = parser.CubeStat(0, 0, 0, 0, &p, &c)
		if err != nil {
			return nil, fmt.Errorf("CubeStat: %v", err)
		}

		// Store cur into previousCache
		previousCache[name] = cur

	case "/proc/meminfo":
		// MemInfo isn't differential so just toss first measurement.
		c, err := parser.ProcessMeminfo([]byte(cur.Measurement.Measurement))
		if err != nil {
			return nil, fmt.Errorf("ProcessMeminfo cur: %v", err)
		}
		// Ignore database bits
		record, err = parser.CubeMeminfo(0, 0, 0, 0, &c)
		if err != nil {
			return nil, fmt.Errorf("CubeMeminfo: %v", err)
		}

	case "/proc/net/dev":
		p, err := parser.ProcessNetDev([]byte(prev.Measurement.Measurement))
		if err != nil {
			return nil, fmt.Errorf("ProcessNetDev prev: %v", err)
		}
		c, err := parser.ProcessNetDev([]byte(cur.Measurement.Measurement))
		if err != nil {
			return nil, fmt.Errorf("ProcessNetDev cur: %v", err)
		}
		// Ignore database bits
		tvi := uint64(cur.Measurement.Frequency.Seconds()) *
			parser.UserHZ

		// XXX there is no nic cache here, fix
		record, err = parser.CubeNetDev(cur.Site, cur.Host, cur.Run,
			0, /* timestamp */
			0, /* start */
			0, /* duration */
			p, c, tvi, cache)
		if err != nil {
			return nil, fmt.Errorf("CubeNetDev: %v", err)
		}

		// Store cur into previousCache
		previousCache[name] = cur

	case "/proc/diskstats":
		p, err := parser.ProcessDiskstats([]byte(prev.Measurement.Measurement))
		if err != nil {
			return nil, fmt.Errorf("ProcessDiskstats prev: %v", err)
		}
		c, err := parser.ProcessDiskstats([]byte(cur.Measurement.Measurement))
		if err != nil {
			return nil, fmt.Errorf("ProcessDiskstats cur: %v", err)
		}
		// Ignore database bits
		tvi := uint64(cur.Measurement.Frequency.Seconds()) *
			parser.UserHZ
		// XXX there is no nic cache here, fix
		record, err = parser.CubeDiskstats(0, 0, 0, 0, p, c, tvi)
		if err != nil {
			return nil, fmt.Errorf("CubeDiskstats: %v", err)
		}

		// Store cur into previousCache
		previousCache[name] = cur

	default:
	}

	return record, nil
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

// min returns the smallest unsigned integer.
func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

// max returns the largest unsigned integer.
func max(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

// giveOrTake see if y is within percent of x.
func giveOrTake(x, y, percent uint64) bool {
	z := (min(x, y) * 100) / max(x, y)
	if (100 - z) <= percent {
		return true
	}
	return false
}

func workerMem(ctx context.Context, wg *sync.WaitGroup, c chan *database.Meminfo) {
	defer wg.Done()

	log.Infof("workerMem: launched")
	defer log.Infof("workerMem: exit")

	var (
		memoryLocation []byte // Mmap pointer
		memorySize     uint64 // Cache value
		err            error
	)

	for {
		select {
		case <-ctx.Done():
			return
		case m, ok := <-c:
			if !ok {
				log.Errorf("workerMem channel exited")
				return
			}
			// Only reallocate memory if the size differs by >10%.
			// Painting memory is very expensive so try to avoid it
			// in order to not contaminate CPU usage.
			if giveOrTake(m.MemUsed, memorySize, 10) {
				continue
			}

			log.Tracef("reallocate MemUsed: %v\n", m.MemUsed*1024)
			// Free prior memory
			if memoryLocation != nil {
				err = munmap(memoryLocation)
				if err != nil {
					// XXX should we abort?
					log.Errorf("munmap: %v", err)
					continue
				}
			}

			// Allocate new size
			memoryLocation, err = mmap(m.MemUsed * 1024)
			if err != nil {
				// XXX should we abort?
				log.Errorf("mmap: %v", err)
				continue
			}
			memorySize = m.MemUsed
		}
	}
}

var (
	td        = make([]uint, 101) // Training data
	numCores  = -1
	frequency = -1

	dm = make(map[string]string) // Disk mapper data
)

func workerStat(ctx context.Context, wg *sync.WaitGroup, c chan []database.Stat) {
	defer wg.Done()

	log.Infof("workerStat: launched")
	defer log.Infof("workerStat: exit")

	for {
		select {
		case <-ctx.Done():
			return
		case s, ok := <-c:
			if !ok {
				log.Errorf("workerStat channel exited")
				return
			}
			if len(s) == 0 {
				log.Errorf("workerStat received empty stat array")
				continue
			}

			// We only care about CPU -1
			if s[0].CPU != -1 {
				log.Errorf("workerStat invalid CPU: %v", s[0].CPU)
				continue
			}
			busy := 100 - s[0].Idle
			if busy < 0 {
				busy = 0
			} else if busy > 100 {
				busy = 100
			}
			units := int(td[int(busy)])
			if units == 0 {
				// Nothing to do
				log.Tracef("workerStat no load on cpu")
				continue
			}

			// Divide work
			u := units / numCores
			if u == 0 {
				// Only a bit of work to be done
				log.Tracef("workerStat busy short: %v units: %v",
					busy, units*frequency)
				go func() {
					d := load.UserWork(units * frequency)
					log.Tracef("workerStat duration short %v",
						d)
				}()
				continue
			}
			units = u

			log.Tracef("workerStat busy: %v units: %v", busy,
				units*frequency)
			for i := 0; i < numCores; i++ {
				x := i
				go func(int) {
					d := load.UserWork(units * frequency)
					log.Tracef("workerStat duration %v", d)
				}(x)
			}
		}
	}
}

type modeRW int

const (
	modeRead modeRW = iota
	modeWrite
)

func loadDisk(ctx context.Context, mode modeRW, ds database.Diskstat) {
	mapping, ok := dm[ds.Name]
	if !ok {
		log.Errorf("unknown disk mapping: %v", ds.Name)
		return
	}

	var (
		tps, blocks  *float64
		ms, filename string
	)
	if mode == modeRead {
		tps = &ds.Rtps
		blocks = &ds.Bread
		ms = "R"
		log.Tracef("%v rtps %v bread %v", ds.Name, ds.Rtps, ds.Bread)

		filename = filepath.Join(mapping, "read")
	} else {
		tps = &ds.Wtps
		blocks = &ds.Bwrtn
		ms = "W"
		log.Tracef("%v wtps %v bwrtn %v", ds.Name, ds.Wtps, ds.Bwrtn)

		f, err := ioutil.TempFile(mapping, "write")
		if err != nil {
			log.Errorf("tempfile %v: %v", mapping, err)
			return
		}
		filename = f.Name()
		f.Close()
		defer os.Remove(filename)
	}
	ios := uint64(*tps)
	b := uint64(*blocks)
	if ios <= 1 {
		ios = 1
	}
	size := b / ios
	if size == 0 {
		size = 1
	}
	size <<= 9 // In blocks
	var msg string
	if size/512%2 != 0 {
		msg = " force aligned"
		size += 512
	}
	log.Tracef("%v mode %v ios %v size %v b %v%v",
		ds.Name, ms, ios, size, b, msg)

	// Hit it
	timeout := time.Duration(frequency) * time.Second
	var (
		err    error
		d      time.Duration
		actual uint64
	)
	if mode == modeRead {
		d, actual, err = load.DiskRead(ctx, timeout, filename, ios,
			size)
		if err != nil {
			log.Errorf("diskread %v ios %v size %v: %v",
				filename, ios, size, err)
			return
		}
	} else {
		d, actual, err = load.DiskWrite(ctx, timeout, filename, ios,
			size)
		if err != nil {
			log.Errorf("diskwrite %v ios %v size %v: %v",
				filename, ios, size, err)
			return
		}
	}
	_ = d
	_ = actual
	//log.Tracef("%v %v d %v actual %v", ds.Name, ms, d, actual)
}

func loadRead(ctx context.Context, ds database.Diskstat) {
	loadDisk(ctx, modeRead, ds)
}

func loadWrite(ctx context.Context, ds database.Diskstat) {
	loadDisk(ctx, modeWrite, ds)
}

func workerDisk(ctx context.Context, wg *sync.WaitGroup, c chan []database.Diskstat) {
	defer wg.Done()

	log.Infof("workerDisk: launched")
	defer log.Infof("workerDisk: exit")

	dmSeen := make(map[string]struct{})

	for {
		select {
		case <-ctx.Done():
			return
		case s, ok := <-c:
			if !ok {
				log.Errorf("workerDisk channel exited")
				return
			}
			if len(s) == 0 {
				log.Errorf("workerDisk received empty stat array")
				continue
			}

			//spew.Dump(s)
			for k := range s {
				// Complain only once
				_, ok := dm[s[k].Name]
				if !ok {
					if _, ok := dmSeen[s[k].Name]; ok {
						continue
					}
					log.Errorf("disk mapping not found: %v",
						s[k].Name)
					dmSeen[s[k].Name] = struct{}{}
					continue
				}

				// Reads
				r := s[k]
				if r.Bread != 0 {
					go loadRead(ctx, r)
				}

				// Writes
				if r.Bwrtn != 0 {
					go loadWrite(ctx, r)
				}
			}
		}
	}
}

func stage(filename, sizeS string) error {
	size, err := humanize.ParseBytes(sizeS)
	if err != nil {
		return fmt.Errorf("stage file invalid size: %v: %v",
			sizeS, err)
	}
	if size == 0 {
		return fmt.Errorf("stage file size cannot be 0")
	}

	// If file exists and is the right size don't stage it
	s, err := os.Stat(filename)
	if err == nil {
		if s.Size() == int64(size) {
			return nil
		}
	}

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("could not create stage file: %v", err)
	}
	defer f.Close()

	mb := size / (1024 * 1024)
	mbChunk, err := util.Random(1024 * 1024)
	if err != nil {
		return fmt.Errorf("could not obtain random data for staging "+
			"file: %v", err)
	}
	for i := uint64(0); i < mb; i++ {
		_, err = f.Write(mbChunk)
		if err != nil {
			return fmt.Errorf("could not write staging file: %v",
				err)
		}
	}

	return nil
}

func _main() error {
	cfg, _, err := loadConfig()
	if err != nil {
		return err
	}

	// Load training data
	tdf, err := os.Open(cfg.Training)
	if err != nil {
		return fmt.Errorf("could not open training data: %v", err)
	}
	jd := json.NewDecoder(tdf)
	for {
		var jsonTD training.Training
		err = jd.Decode(&jsonTD)
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("decoding training data: %v", err)
		}
		td[jsonTD.Busy] = jsonTD.Units

		// Enforce siteid and host
		if cfg.Site != jsonTD.SiteID || cfg.Host != jsonTD.Host {
			return fmt.Errorf("unexpected site/host found in " +
				"training data")
		}
	}

	// Fill out rest of training data
	round := false // XXX Make a flag?
	for i := 0; i < 100; i += 10 {
		min := float64(td[i])
		max := float64(td[i+10])
		step := (max - min) / 10
		log.Tracef("i %v min %v max %v step %v\n", i, min, max, step)
		for x := 1; x < 10; x++ {
			ofs := x + i
			units := min + (step * float64(x))
			if round {
				// ROund units
				td[ofs] = uint(math.Round(units))
			} else {
				// Clip units
				td[ofs] = uint(units)
			}
			log.Tracef("  x %v units %v td %v\n", ofs, units,
				td[ofs])
		}
	}

	// Initialize loggers
	loggo.ConfigureLoggers(cfg.Log)

	// Load disk mapping
	dmf, err := os.Open(cfg.DiskMapper)
	if err != nil {
		return fmt.Errorf("could not open disk mapper data: %v", err)
	}
	jdm := json.NewDecoder(dmf)
	for {
		var jsonDM training.DiskMapper
		err = jdm.Decode(&jsonDM)
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("decoding disk mapper data: %v", err)
		}

		// Verify mount point
		s, err := os.Stat(jsonDM.MountPoint)
		if err != nil {
			return fmt.Errorf("invalid mount point: %v", err)
		}
		if !s.IsDir() {
			return fmt.Errorf("not a dir: %v", jsonDM.MountPoint)
		}

		// Stage read file
		filename := filepath.Join(jsonDM.MountPoint, "read")
		log.Infof("Staging file: %v with %v", filename, jsonDM.ReadSize)
		err = stage(filename, jsonDM.ReadSize)
		if err != nil {
			return err
		}

		dm[jsonDM.DeviceName] = jsonDM.MountPoint

		// Enforce siteid and host
		if cfg.Site != jsonDM.SiteID || cfg.Host != jsonDM.Host {
			return fmt.Errorf("unexpected site/host found in " +
				"disk mapper data")
		}
	}

	log.Infof("Start of day")
	log.Infof("Version %s (Go version %s %s/%s)", versionString(),
		runtime.Version(), runtime.GOOS, runtime.GOARCH)
	if cfg.SiteName != "" {
		log.Infof("Site   : %v", cfg.SiteName)
	}
	if cfg.License != "" {
		log.Infof("License: %v", cfg.License)
	}
	log.Infof("Site ID: %v", cfg.Site)
	log.Infof("Host ID: %v", cfg.Host)
	log.Infof("Run ID : %v", cfg.Run)

	// Print disk mapping
	var sorted []string
	for k := range dm {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for k := range sorted {
		log.Infof("Mapping %v -> %v", sorted[k], dm[sorted[k]])
	}

	// Generate journal key from license material. There is no function for
	// this in order to obfuscate this terrible trick.
	aead, err := journal.CreateAEAD(cfg.Site, cfg.License, cfg.SiteName)
	if err != nil {
		return fmt.Errorf("could not setup aead: %v", err)
	}

	// Open input file
	f, err := os.Open(cfg.InputFile)
	if err != nil {
		return fmt.Errorf("input: %v", err)
	}

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

	// Detect how many systems we have to replay and at what frequency.
	var freq time.Duration
	if cfg.License == "" {
		jd = json.NewDecoder(f)
	}
	seen := make(map[string]struct{}, 16)
	for {
		var (
			wc  *journal.WrapPCCollection
			err error
		)
		if cfg.License != "" {
			wc, err = journal.ReadEncryptedJournalEntry(f, aead)
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}
		} else {
			var wrap journal.WrapPCCollection
			err = jd.Decode(&wrap)
			if err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("pre: %v", err)
			}
			wc = &wrap
		}

		if wc.Site != cfg.Site || wc.Host != cfg.Host ||
			wc.Run != cfg.Run {
			continue
		}

		freq = wc.Measurement.Frequency
		if _, ok := seen[wc.Measurement.System]; ok {
			break
		}
		seen[wc.Measurement.System] = struct{}{}

	}
	frequency = int(freq / time.Second) // Store frequency

	// Determine core count/
	_, virtualCores, err := load.NumCores()
	if err != nil {
		return fmt.Errorf("Could not determine core count: %v", err)
	}
	numCores = int(virtualCores)

	// Launch workers
	var wg sync.WaitGroup
	ctx, cancel := withShutdownCancel(context.Background())
	go shutdownListener()

	wg.Add(1)
	workerStatC := make(chan []database.Stat)
	go workerStat(ctx, &wg, workerStatC)

	wg.Add(1)
	workerMemC := make(chan *database.Meminfo)
	go workerMem(ctx, &wg, workerMemC)

	wg.Add(1)
	workerDiskC := make(chan []database.Diskstat)
	go workerDisk(ctx, &wg, workerDiskC)

	// XXX wait for thread launch
	time.Sleep(time.Second)

	// Rewind file
	fs, err := f.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}
	if fs != 0 {
		return fmt.Errorf("Rewind failed: %v", fs)
	}

	// Process
	if cfg.License == "" {
		jd = json.NewDecoder(f)
	}
	entries := 0
	recordCount := 0
	primeCounter := 0

	timer := time.NewTimer(freq)
	for {
		var (
			wc  *journal.WrapPCCollection
			err error
		)
		if cfg.License != "" {
			wc, err = journal.ReadEncryptedJournalEntry(f, aead)
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}
		} else {
			var wrap journal.WrapPCCollection
			err = jd.Decode(&wrap)
			if err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("parse: %v", err)
			}
			wc = &wrap
		}

		if wc.Site != cfg.Site || wc.Host != cfg.Host ||
			wc.Run != cfg.Run {
			continue
		}

		if primeCounter < len(seen) {
			// First measurement is tossed since we need to prime
			// the replay.
			primeCounter++
			log.Tracef("primeCounter %v -> %v", primeCounter,
				wc.Measurement.System)
			continue
		}

		record, err := parse(cfg, wc, netCache)
		if err != nil {
			return err
		}
		if record == nil {
			// Record was skipped for whatever reason.
			continue
		}

		entries++

		switch x := record.(type) {
		case []database.Stat:
			select {
			case <-ctx.Done():
				log.Errorf("workerStat context exit")
				goto done
			case workerStatC <- x:
			default:
				log.Errorf("couldn't send work to workerStat")
			}

		case *database.Meminfo:
			select {
			case <-ctx.Done():
				log.Errorf("workerMem context exit")
				goto done
			case workerMemC <- x:
			default:
				log.Errorf("couldn't send work to workerMem")
			}

		case []database.NetDev:
			// Ignore for now

		case []database.Diskstat:
			select {
			case <-ctx.Done():
				log.Errorf("workerDisk context exit")
				goto done
			case workerDiskC <- x:
			default:
				log.Errorf("couldn't send work to workerDisk")
			}

		default:
			log.Tracef("Unsuported record type: %T", record)
			continue
		}

		recordCount++
		if recordCount < len(seen) {
			continue
		}
		recordCount = 0

		log.Tracef("awaiting tick, entries processed: %v", entries)

		select {
		case <-ctx.Done():
			log.Tracef("ctx done: %v", ctx.Err())
			goto done
		case <-timer.C:
			timer = time.NewTimer(freq)
		}
	}

	// Cancel all go routines
	cancel()

done:
	wg.Wait()
	return ctx.Err()
}

func main() {
	err := _main()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
