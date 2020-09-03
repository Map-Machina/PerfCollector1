package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/businessperformancetuning/license/license"
)

var (
	re = regexp.MustCompile("^([0-9a-fA-F][0-9a-fA-F]:){5}([0-9a-fA-F][0-9a-fA-F])$")
)

func _main() error {
	args := os.Args
	if len(args) != 4 {
		return fmt.Errorf("usage %v <site_id> <site_name> "+
			"<mac_address>", args[0])
	}
	if _, err := strconv.ParseUint(args[1], 10, 64); err != nil {
		return fmt.Errorf("Invalid site_id: %v", err)
	}
	if args[3] == "00:00:00:00:00:00" {
		return fmt.Errorf("localhost mac address")
	}
	if !re.MatchString(args[3]) {
		return fmt.Errorf("invalid mac address")
	}
	l, err := license.NewFromStrings(args[1], args[2])
	if err != nil {
		return err
	}
	b, err := l.Encode(1, args[3], 24*time.Hour)
	if err != nil {
		return err
	}

	fmt.Printf("siteid=%v\n", args[1])
	fmt.Printf("sitename=%v\n", args[2])
	fmt.Printf("license=%v\n", license.LicenseString(b))

	return nil
}

func main() {
	err := _main()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
