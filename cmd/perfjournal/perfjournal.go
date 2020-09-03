package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"os"

	"github.com/businessperformancetuning/perfcollector/cmd/perfprocessord/journal"
	"github.com/davecgh/go-spew/spew"
	cp "golang.org/x/crypto/chacha20poly1305"
)

func _main() error {
	args := os.Args
	if len(args) != 5 {
		return fmt.Errorf("usage %v <site_id> <site_name> "+
			"<license> <filename>", args[0])
	}

	// Generate journal key from license material. There is no function for
	// this in order to obfudcate this terrible trick.
	mac := hmac.New(sha256.New, []byte(args[3]))
	mac.Write([]byte(args[1]))
	mac.Write([]byte(args[2]))
	aead, err := cp.NewX(mac.Sum(nil))
	if err != nil {
		return err
	}

	f, err := os.Open(args[4])
	if err != nil {
		return err
	}

	for {
		wc, err := journal.ReadEncryptedJournalEntry(f, aead)
		if err != nil {
			return err
		}
		spew.Dump(wc)
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
