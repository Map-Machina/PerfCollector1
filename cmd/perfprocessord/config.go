// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/businessperformancetuning/license/license"
	"github.com/businessperformancetuning/perfcollector/cmd/perfprocessord/sharedconfig"
	"github.com/businessperformancetuning/perfcollector/database"
	"github.com/businessperformancetuning/perfcollector/database/postgres"
	"github.com/businessperformancetuning/perfcollector/util"
	flags "github.com/jessevdk/go-flags"
	cp "golang.org/x/crypto/chacha20poly1305"
)

const (
	defaultLogLevel    = "info"
	defaultLogDirname  = "logs"
	defaultLogFilename = "perfprocessord.log"
)

var (
	defaultSSHKeyFile = filepath.Join(sharedconfig.DefaultHomeDir, "id_ed25519")
	defaultLogDir     = filepath.Join(sharedconfig.DefaultHomeDir, defaultLogDirname)
	nicRe             = regexp.MustCompile("^([0-9a-fA-F][0-9a-fA-F]:){5}([0-9a-fA-F][0-9a-fA-F])$")
)

// runServiceCommand is only set to a real function on Windows.  It is used
// to parse and execute service commands specified via the -s flag.
var runServiceCommand func(string) error

type HostIdentifier struct {
	Site uint64
	Host uint64
	IP   string // ip:port
}

// config defines the configuration options for the server.
//
// See loadConfig for details on the configuration load process.
type config struct {
	HomeDir     string `short:"A" long:"appdata" description:"Path to application home directory"`
	ShowVersion bool   `short:"V" long:"version" description:"Display version information and exit"`
	ConfigFile  string `short:"C" long:"configfile" description:"Path to configuration file"`
	DataDir     string `short:"b" long:"datadir" description:"Directory to store data"`
	LogDir      string `long:"logdir" description:"Directory to log output."`
	Profile     string `long:"profile" description:"Enable HTTP profiling on given port -- NOTE port must be between 1024 and 65536"`
	CPUProfile  string `long:"cpuprofile" description:"Write CPU profile to the specified file"`
	MemProfile  string `long:"memprofile" description:"Write mem profile to the specified file"`
	DebugLevel  string `short:"d" long:"debuglevel" description:"Logging level for all subsystems {trace, debug, info, warn, error, critical} -- You may also specify <subsystem>=<level>,<subsystem2>=<level>,... to set the log level for individual subsystems -- Use show to list available subsystems"`
	Version     string
	SSHKeyFile  string   `long:"sshid" description:"File containing the ssh identity"`
	Hosts       []string `long:"hosts" description:"Add perfcollector host <siteid:hostid/ip:port>"`

	// Database
	DBURI    string `long:"dburi" description:"Database URI"`
	DB       string `long:"db" description:"Database type -- supported types: postgres"`
	DBCreate bool   `long:"dbcreate" description:"Create database and exit, requires db and admin credentials on dburi"`

	// Journal
	Journal bool `long:"journal" description:"Enable journaling of raw data."`

	journalFilename string      // Journal filename including path
	aead            cipher.AEAD // journal encryption cipher

	// License
	SiteID   string `long:"siteid" description:"Site identifier"`
	SiteName string `long:"sitename" description:"Site name"`
	License  string `long:"license" description:"License"`
	license  *license.LicenseKey

	HostsId map[string]HostIdentifier
}

// serviceOptions defines the configuration options for the rpc as a service
// on Windows.
type serviceOptions struct {
	ServiceCommand string `short:"s" long:"service" description:"Service command {install, remove, start, stop}"`
}

// cleanAndExpandPath expands environment variables and leading ~ in the
// passed path, cleans the result, and returns it.
func cleanAndExpandPath(path string) string {
	// Expand initial ~ to OS specific home directory.
	if strings.HasPrefix(path, "~") {
		homeDir := filepath.Dir(sharedconfig.DefaultHomeDir)
		path = strings.Replace(path, "~", homeDir, 1)
	}

	// NOTE: The os.ExpandEnv doesn't work with Windows-style %VARIABLE%,
	// but they variables can still be expanded via POSIX-style $VARIABLE.
	return filepath.Clean(os.ExpandEnv(path))
}

// validLogLevel returns whether or not logLevel is a valid debug log level.
func validLogLevel(logLevel string) bool {
	switch logLevel {
	case "trace":
		fallthrough
	case "debug":
		fallthrough
	case "info":
		fallthrough
	case "warn":
		fallthrough
	case "error":
		fallthrough
	case "critical":
		return true
	}
	return false
}

// supportedSubsystems returns a sorted slice of the supported subsystems for
// logging purposes.
func supportedSubsystems() []string {
	// Convert the subsystemLoggers map keys to a slice.
	subsystems := make([]string, 0, len(subsystemLoggers))
	for subsysID := range subsystemLoggers {
		subsystems = append(subsystems, subsysID)
	}

	// Sort the subsytems for stable display.
	sort.Strings(subsystems)
	return subsystems
}

// parseAndSetDebugLevels attempts to parse the specified debug level and set
// the levels accordingly.  An appropriate error is returned if anything is
// invalid.
func parseAndSetDebugLevels(debugLevel string) error {
	// When the specified string doesn't have any delimters, treat it as
	// the log level for all subsystems.
	if !strings.Contains(debugLevel, ",") && !strings.Contains(debugLevel, "=") {
		// Validate debug log level.
		if !validLogLevel(debugLevel) {
			str := "The specified debug level [%v] is invalid"
			return fmt.Errorf(str, debugLevel)
		}

		// Change the logging level for all subsystems.
		setLogLevels(debugLevel)

		return nil
	}

	// Split the specified string into subsystem/level pairs while detecting
	// issues and update the log levels accordingly.
	for _, logLevelPair := range strings.Split(debugLevel, ",") {
		if !strings.Contains(logLevelPair, "=") {
			str := "The specified debug level contains an invalid " +
				"subsystem/level pair [%v]"
			return fmt.Errorf(str, logLevelPair)
		}

		// Extract the specified subsystem and log level.
		fields := strings.Split(logLevelPair, "=")
		subsysID, logLevel := fields[0], fields[1]

		// Validate subsystem.
		if _, exists := subsystemLoggers[subsysID]; !exists {
			str := "The specified subsystem [%v] is invalid -- " +
				"supported subsytems %v"
			return fmt.Errorf(str, subsysID, supportedSubsystems())
		}

		// Validate log level.
		if !validLogLevel(logLevel) {
			str := "The specified debug level [%v] is invalid"
			return fmt.Errorf(str, logLevel)
		}

		setLogLevel(subsysID, logLevel)
	}

	return nil
}

// removeDuplicateAddresses returns a new slice with all duplicate entries in
// addrs removed.
func removeDuplicateAddresses(addrs []string) []string {
	result := make([]string, 0, len(addrs))
	seen := map[string]struct{}{}
	for _, val := range addrs {
		if _, ok := seen[val]; !ok {
			result = append(result, val)
			seen[val] = struct{}{}
		}
	}
	return result
}

// normalizeAddresses returns a new slice with all the passed peer addresses
// normalized with the given default port, and all duplicates removed.
func normalizeAddresses(addrs []string, defaultPort string) []string {
	for i, addr := range addrs {
		addrs[i] = util.NormalizeAddress(addr, defaultPort)
	}

	return removeDuplicateAddresses(addrs)
}

// filesExists reports whether the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// newConfigParser returns a new command line flags parser.
func newConfigParser(cfg *config, so *serviceOptions, options flags.Options) *flags.Parser {
	parser := flags.NewParser(cfg, options)
	if runtime.GOOS == "windows" {
		parser.AddGroup("Service Options", "Service Options", so)
	}
	return parser
}

// getAllMacs returns all MAC addressess on a system except for localhost.
func getAllMacs() ([]string, error) {
	dir := "/sys/class/net/"
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	macs := make([]string, 0, len(files))
	for _, file := range files {
		if file.Name() == "lo" {
			continue
		}
		f, err := os.Open(filepath.Join(dir, file.Name(), "address"))
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			address := scanner.Text()
			if !nicRe.MatchString(address) {
				f.Close()
				return nil, fmt.Errorf("invalid mac address")
			}
			macs = append(macs, address)
		}

		if err := scanner.Err(); err != nil {
			f.Close()
			return nil, err
		}
		f.Close()
	}
	return macs, nil
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
// The above results in rpc functioning properly without any config settings
// while still allowing the user to override settings with config files and
// command line options.  Command line options always take precedence.
func loadConfig() (*config, []string, error) {
	// Default config.
	cfg := config{
		HomeDir:    sharedconfig.DefaultHomeDir,
		ConfigFile: sharedconfig.DefaultConfigFile,
		DebugLevel: defaultLogLevel,
		DataDir:    sharedconfig.DefaultDataDir,
		LogDir:     defaultLogDir,
		SSHKeyFile: defaultSSHKeyFile,
		Version:    version(),
		HostsId:    make(map[string]HostIdentifier),
	}

	// Service options which are only added on Windows.
	serviceOpts := serviceOptions{}

	// Pre-parse the command line options to see if an alternative config
	// file or the version flag was specified.  Any errors aside from the
	// help message error can be ignored here since they will be caught by
	// the final parse below.
	preCfg := cfg
	preParser := newConfigParser(&preCfg, &serviceOpts, flags.HelpFlag)
	_, err := preParser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
			fmt.Fprintln(os.Stderr, err)
			return nil, nil, err
		}
	}

	// Show the version and exit if the version flag was specified.
	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))
	usageMessage := fmt.Sprintf("Use %s -h to show usage", appName)
	if preCfg.ShowVersion {
		fmt.Println(appName, "version", version())
		os.Exit(0)
	}

	// Perform service command and exit if specified.  Invalid service
	// commands show an appropriate error.  Only runs on Windows since
	// the runServiceCommand function will be nil when not on Windows.
	if serviceOpts.ServiceCommand != "" && runServiceCommand != nil {
		err := runServiceCommand(serviceOpts.ServiceCommand)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(0)
	}

	// Update the home directory for stakepoold if specified. Since the
	// home directory is updated, other variables need to be updated to
	// reflect the new changes.
	if preCfg.HomeDir != "" {
		cfg.HomeDir, _ = filepath.Abs(preCfg.HomeDir)

		if preCfg.ConfigFile == sharedconfig.DefaultConfigFile {
			cfg.ConfigFile = filepath.Join(cfg.HomeDir, sharedconfig.DefaultConfigFilename)
		} else {
			cfg.ConfigFile = preCfg.ConfigFile
		}
		if preCfg.DataDir == sharedconfig.DefaultDataDir {
			cfg.DataDir = filepath.Join(cfg.HomeDir, sharedconfig.DefaultDataDirname)
		} else {
			cfg.DataDir = preCfg.DataDir
		}
		if preCfg.SSHKeyFile == defaultSSHKeyFile {
			cfg.SSHKeyFile = filepath.Join(cfg.HomeDir, "id_ed25519")
		} else {
			cfg.SSHKeyFile = preCfg.SSHKeyFile
		}
		if preCfg.LogDir == defaultLogDir {
			cfg.LogDir = filepath.Join(cfg.HomeDir, defaultLogDirname)
		} else {
			cfg.LogDir = preCfg.LogDir
		}
	}

	// Load additional config from file.
	var configFileError error
	parser := newConfigParser(&cfg, &serviceOpts, flags.Default)
	err = flags.NewIniParser(parser).ParseFile(cfg.ConfigFile)
	if err != nil {
		if _, ok := err.(*os.PathError); !ok {
			fmt.Fprintf(os.Stderr, "Error parsing config "+
				"file: %v\n", err)
			fmt.Fprintln(os.Stderr, usageMessage)
			return nil, nil, err
		}
		configFileError = err
	}

	// Parse command line options again to ensure they take precedence.
	remainingArgs, err := parser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); !ok || e.Type != flags.ErrHelp {
			fmt.Fprintln(os.Stderr, usageMessage)
		}
		return nil, nil, err
	}

	// Create the home directory if it doesn't already exist.
	funcName := "loadConfig"
	err = os.MkdirAll(sharedconfig.DefaultHomeDir, 0700)
	if err != nil {
		// Show a nicer error message if it's because a symlink is
		// linked to a directory that does not exist (probably because
		// it's not mounted).
		if e, ok := err.(*os.PathError); ok && os.IsExist(err) {
			if link, lerr := os.Readlink(e.Path); lerr == nil {
				str := "is symlink %s -> %s mounted?"
				err = fmt.Errorf(str, e.Path, link)
			}
		}

		str := "%s: Failed to create home directory: %v"
		err := fmt.Errorf(str, funcName, err)
		fmt.Fprintln(os.Stderr, err)
		return nil, nil, err
	}

	// Append the network type to the data directory so it is "namespaced"
	// per network.  In addition to the block database, there are other
	// pieces of data that are saved to disk such as address manager state.
	// All data is specific to a network, so namespacing the data directory
	// means each individual piece of serialized data does not have to
	// worry about changing names per network and such.
	cfg.DataDir = cleanAndExpandPath(cfg.DataDir)

	// Journal filename and cipher
	cfg.journalFilename = filepath.Join(cfg.DataDir,
		sharedconfig.DefaultJournalFilename)

	// Append the network type to the log directory so it is "namespaced"
	// per network in the same fashion as the data directory.
	cfg.LogDir = cleanAndExpandPath(cfg.LogDir)
	cfg.SSHKeyFile = cleanAndExpandPath(cfg.SSHKeyFile)

	// Special show command to list supported subsystems and exit.
	if cfg.DebugLevel == "show" {
		fmt.Println("Supported subsystems", supportedSubsystems())
		os.Exit(0)
	}

	// Initialize log rotation.  After log rotation has been initialized,
	// the logger variables may be used.
	initLogRotator(filepath.Join(cfg.LogDir, defaultLogFilename))

	// Parse, validate, and set debug log level(s).
	if err := parseAndSetDebugLevels(cfg.DebugLevel); err != nil {
		err := fmt.Errorf("%s: %v", funcName, err.Error())
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usageMessage)
		return nil, nil, err
	}

	// Validate profile port number
	if cfg.Profile != "" {
		profilePort, err := strconv.Atoi(cfg.Profile)
		if err != nil || profilePort < 1024 || profilePort > 65535 {
			str := "%s: The profile port must be between 1024 and 65535"
			err := fmt.Errorf(str, funcName)
			fmt.Fprintln(os.Stderr, err)
			fmt.Fprintln(os.Stderr, usageMessage)
			return nil, nil, err
		}
	}

	// Hosts.
	dedupID := make(map[string]struct{}, len(cfg.Hosts))
	for _, v := range cfg.Hosts {
		// Split identifier/ipaddress.
		a := strings.SplitN(v, "/", 2)
		if len(a) != 2 {
			return nil, nil, fmt.Errorf("Invalid Hosts: %v", v)
		}
		ipAddress := a[1]

		// Split site:host.
		h := strings.SplitN(a[0], ":", 2)
		if len(h) != 2 {
			return nil, nil, fmt.Errorf("Invalid identifier: %v",
				a[0])
		}

		// Site ID.
		siteId, err := strconv.ParseUint(h[0], 10, 64)
		if err != nil {
			return nil, nil, fmt.Errorf("Invalid site %v: %v",
				h[0], err)
		}

		// Host ID.
		hostId, err := strconv.ParseUint(h[1], 10, 64)
		if err != nil {
			return nil, nil, fmt.Errorf("Invalid host %v: %v",
				h[1], err)
		}

		// Make sure there is no duplicate IP.
		if _, ok := cfg.HostsId[ipAddress]; ok {
			return nil, nil, fmt.Errorf("duplicate ip address: %v",
				ipAddress)
		}

		// Make sure there is no duplicate identifier.
		if _, ok := dedupID[a[0]]; ok {
			return nil, nil, fmt.Errorf("duplicate host identifier: %v",
				a[0])
		}

		// Insert into dedup map.
		dedupID[a[0]] = struct{}{}

		// Insert into lookup map.
		cfg.HostsId[a[1]] = HostIdentifier{
			Site: siteId,
			Host: hostId,
			IP:   a[1],
		}
	}

	// We only need db commands if we are in sink mode.
	if cfg.DBURI == "" && len(remainingArgs) != 0 &&
		remainingArgs[0] == "sink" {
		err := fmt.Errorf("%s: must provide database URI",
			funcName)
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usageMessage)
		return nil, nil, err
	}
	if cfg.DBCreate {
		switch cfg.DB {
		case "postgres":
			db, err := postgres.New(database.Name, cfg.DBURI)
			if err != nil {
				err := fmt.Errorf("%s: %v", funcName, err)
				fmt.Fprintln(os.Stderr, err)
				fmt.Fprintln(os.Stderr, usageMessage)
				return nil, nil, err
			}
			if err := db.Create(); err != nil {
				err := fmt.Errorf("%s: %v", funcName, err)
				fmt.Fprintln(os.Stderr, err)
				fmt.Fprintln(os.Stderr, usageMessage)
				return nil, nil, err
			}
		default:
			err := fmt.Errorf("%s: invalid database type %v",
				funcName, cfg.DB)
			fmt.Fprintln(os.Stderr, err)
			fmt.Fprintln(os.Stderr, usageMessage)
			return nil, nil, err
		}

		// Always exit.
		os.Exit(0)
	}

	// Deal with license.
	if cfg.SiteID == "" || cfg.SiteName == "" || cfg.License == "" {
		return nil, nil, fmt.Errorf("Must provide: siteid, sitename " +
			"and license")
	}
	l, err := license.NewFromStrings(cfg.SiteID, cfg.SiteName)
	if err != nil {
		return nil, nil, err
	}
	bb, err := license.LicenseFromHuman(cfg.License)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid license: %v", err)
	}
	macs, err := getAllMacs()
	if err != nil {
		return nil, nil, fmt.Errorf("could not obtain mac "+
			"addressess: %v", err)
	}
	found := false
	for k := range macs {
		cfg.license, err = l.Decode(bb, 1, macs[k])
		if err != nil {
			log.Debugf("unrecognized mac address: %v", macs[k])
			continue
		} else {
			found = true
			break
		}
	}
	if !found {
		return nil, nil, fmt.Errorf("No suitable MAC address found " +
			"for provided license")
	}
	if cfg.license.Expired() {
		return nil, nil, fmt.Errorf("License expired")
	}

	// Generate journal key from license material.
	mac := hmac.New(sha256.New, []byte(cfg.License))
	mac.Write([]byte(cfg.SiteID))
	mac.Write([]byte(cfg.SiteName))
	cfg.aead, err = cp.NewX(mac.Sum(nil))
	if err != nil {
		return nil, nil, err
	}

	// Warn about missing config file only after all other configuration is
	// done.  This prevents the warning on help messages and invalid
	// options.  Note this should go directly before the return.
	if configFileError != nil {
		log.Warnf("%v", configFileError)
	}

	return &cfg, remainingArgs, nil
}
