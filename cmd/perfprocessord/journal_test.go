package main

import (
	"crypto/rand"
	"io"
	"os"
	"testing"
	"time"

	"github.com/businessperformancetuning/perfcollector/types"
	cp "golang.org/x/crypto/chacha20poly1305"
)

func TestEncryptedJournal(t *testing.T) {
	key := make([]byte, cp.KeySize)
	if _, err := rand.Read(key); err != nil {
		panic(err)
	}
	aead, err := cp.NewX(key)
	if err != nil {
		t.Fatal(err)
	}
	p := PerfCtl{cfg: &config{
		aead:            aead,
		journalFilename: "journal",
	}}
	for i := uint64(0); i < 100; i++ {
		err = p.journal(i+1, 2, 3, types.PCCollection{
			Timestamp: time.Now(),
			Start:     time.Now(),
			Duration:  500 * time.Microsecond,
			Frequency: 5 * time.Second,
			System:    "/proc/stat",
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	f, err := os.Open(p.cfg.journalFilename)
	if err != nil {
		t.Fatal(err)
	}
	i := uint64(1)
	for {
		wc, err := readEncryptedJournalEntry(f, aead)
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
		if wc.Site != i {
			t.Fatalf("invalid site: got %v, want %v", wc.Site, i)
		}
		i++
	}
}
