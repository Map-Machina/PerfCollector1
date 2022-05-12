package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/businessperformancetuning/license/cmd/licensed/database"
	"github.com/businessperformancetuning/license/cmd/licensed/database/postgres"
	"github.com/businessperformancetuning/license/license"
	"github.com/businessperformancetuning/perfcollector/util"
	"github.com/jrick/flagfile"
)

var (
	re = regexp.MustCompile("^([0-9a-fA-F][0-9a-fA-F]:){5}([0-9a-fA-F][0-9a-fA-F])$")

	homeDir           = filepath.Join(os.Getenv("HOME"), ".perflicense")
	defaultConfigFile = filepath.Join(homeDir, "perflicense.conf")

	defaultDB = "postgres"
)

func versionString() string {
	return "1.0.0"
}

// config defines the configuration options for perflicense.
//
// See loadConfig for details on the configuration load process.
type config struct {
	Config      flag.Value
	ShowVersion bool
	CreateDB    bool
	DB          string
	DBURI       string
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage of perflicense:
  perflicense [flags] action <args...>
Flags:
  -C value
        config file
  -v    show version and exit
  --createdb
	Create database for first time use
  --db string
        Database backend (default postgres)
  --dburi
        Database uri
Actions:
  useradd email=<emailaddress> admin=<bool>
	Add user that is allowed to create licenses. Example:
	email=marco@peereboom.us admin=false
  siteadd name=<name>
	Add site. Example
	name="Evil Corp LLC."
  licenseadd siteid=<id> sitename=<name> mac=<mac_address> duration=<in_days> user=Muser_id>
	Add license for a site. Example:
	siteid=1337 sitename="Evil Database App" mac="00:22:4d:81:a1:41" duration=10 user=1
`)
	os.Exit(2)
}

func (c *config) FlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("perflicense", flag.ExitOnError)
	configParser := flagfile.Parser{AllowUnknown: false}
	c.Config = configParser.ConfigFlag(fs)
	fs.Var(c.Config, "C", "config file")
	fs.BoolVar(&c.ShowVersion, "v", false, "")
	fs.BoolVar(&c.CreateDB, "createdb", false, "")
	fs.StringVar(&c.DB, "db", "postgres", "")
	fs.StringVar(&c.DBURI, "dburi", "", "")
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
	cfg := &config{
		DB: defaultDB,
	}
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

	if cfg.CreateDB {
		db, err := postgres.New("license", cfg.DBURI)
		if err != nil {
			fmt.Fprintf(os.Stderr, "db new: %v\n", err)
			os.Exit(1)
		}
		if err := db.Create(); err != nil {
			fmt.Fprintf(os.Stderr, "db create: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	return cfg, fs.Args(), nil
}

func userAdd(ctx context.Context, db database.Database, a map[string]string) error {
	email, err := util.ArgAsString("email", a)
	if err != nil {
		return err
	}
	admin, err := util.ArgAsBool("admin", a)
	if err != nil {
		return err
	}
	user := database.User{
		Email: email,
		Admin: admin,
	}
	id, err := db.UserInsert(ctx, &user)
	if err != nil {
		return fmt.Errorf("user insert error: %v", err)
	}
	fmt.Printf("User id: %v\n", id)
	return nil
}

func siteAdd(ctx context.Context, db database.Database, a map[string]string) error {
	name, err := util.ArgAsString("name", a)
	if err != nil {
		return err
	}
	site := database.Site{
		Name: name,
	}
	id, err := db.SiteInsert(ctx, &site)
	if err != nil {
		return fmt.Errorf("site insert error: %v", err)
	}
	fmt.Printf("Site id: %v\n", id)
	return nil
}

func licenseAdd(ctx context.Context, db database.Database, a map[string]string) error {
	// Creator.
	//userID, err := util.ArgAsUint("user", a)
	//if err != nil {
	//	return err
	//}
	//user, err := db.UserSelect(ctx, userID)
	//if err != nil {
	//	return err
	//}

	// Site information
	sid, err := util.ArgAsUint("siteid", a)
	if err != nil {
		return err
	}
	siteID := strconv.FormatUint(uint64(sid), 10)
	siteName, err := util.ArgAsString("sitename", a)
	if err != nil {
		return err
	}

	// MAC address
	mac, err := util.ArgAsString("mac", a)
	if err != nil {
		return err
	}
	if mac == "00:00:00:00:00:00" {
		return fmt.Errorf("localhost mac address")
	}
	if !re.MatchString(mac) {
		return fmt.Errorf("invalid mac address")
	}

	// Version
	version, err := util.ArgAsByte("version", a)
	if err != nil {
		fmt.Printf("version not specified, defaulting to 1\n")
		version = 1
	}

	// Duration
	d, err := util.ArgAsInt("duration", a)
	if err != nil {
		fmt.Printf("duration not specified, defaulting to 30 days\n")
		d = 30
	}
	duration := time.Duration(d)

	lic, err := license.NewFromStrings(siteID, siteName)
	if err != nil {
		return err
	}
	b, err := lic.Encode(version, mac, duration*24*time.Hour)
	if err != nil {
		return err
	}
	ls, err := lic.Decode(b, version, mac)
	if err != nil {
		return err
	}
	_ = ls

	// Insert into db.
	//l := database.License{
	//	CreatedBy:       user.ID,
	//	Timestamp:       time.Now().Unix(),
	//	Version:         license.VersionLicenseKey,
	//	ExternalVersion: version,
	//	SiteID:          sid,
	//	SiteName:        siteName,
	//	UniqueID:        mac,
	//	Duration:        int64(duration),
	//	Expiration:      ls.Timestamp.Unix(),
	//}
	//err = db.LicenseInsert(ctx, &l)
	//if err != nil {
	//	return fmt.Errorf("user insert error: %v", err)
	//}
	//_ = l

	fmt.Printf("# License information\n")
	fmt.Printf("siteid=%v\n", siteID)
	fmt.Printf("sitename=%v\n", siteName)
	fmt.Printf("license=%v\n", license.LicenseString(b))

	return nil
}

func _main() error {
	cfg, args, err := loadConfig()
	if err != nil {
		return err
	}
	_ = cfg

	if len(args) == 0 {
		return fmt.Errorf("no action provided")
	}

	var db database.Database
	//switch cfg.DB {
	//case "postgres":
	//	db, err = postgres.New("license", cfg.DBURI)
	//	if err != nil {
	//		return fmt.Errorf("database new: %v", err)
	//	}
	//default:
	//	return fmt.Errorf("invalid database: %v", cfg.DB)
	//}
	//err = db.Open()
	//if err != nil {
	//	return fmt.Errorf("database open: %v", err)
	//}
	//defer db.Close()

	// Deal with command line
	a, err := util.ParseArgs(args)
	if err != nil {
		return err
	}

	ctx := context.Background()

	switch args[0] {
	case "useradd":
		return userAdd(ctx, db, a)

	case "siteadd":
		return siteAdd(ctx, db, a)

	case "licenseadd":
		return licenseAdd(ctx, db, a)

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
