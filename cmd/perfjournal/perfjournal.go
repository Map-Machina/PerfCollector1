package main

import (
	"bytes"
	"compress/gzip"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"

	"github.com/businessperformancetuning/perfcollector/types"
	"github.com/davecgh/go-spew/spew"
	cp "golang.org/x/crypto/chacha20poly1305"
)

// XXX this needs to be shared
type WrapPCCollection struct {
	Site        uint64
	Host        uint64
	Run         uint64
	Measurement *types.PCCollection
}

func decrypt(aead cipher.AEAD, nonce, ciphertext []byte) ([]byte, error) {
	// Decrypt the message and check it wasn't tampered with.
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

func readEncryptedJournalEntry(f *os.File, aead cipher.AEAD) (*WrapPCCollection, error) {
	// Read nonce + ciphertext length.
	length := make([]byte, 4)
	n, err := f.Read(length)
	if err != nil {
		return nil, err
	}
	if n != 4 {
		return nil, fmt.Errorf("length short read: %v", n)
	}
	l := int(binary.LittleEndian.Uint32(length))

	// Read nonce + ciphertext.
	blob := make([]byte, l)
	n, err = f.Read(blob)
	if err != nil {
		return nil, err
	}
	if n != l {
		return nil, fmt.Errorf("short read: got %v expected %v",
			n, l)
	}

	// Decrypt.
	plain, err := decrypt(aead, blob[:aead.NonceSize()],
		blob[aead.NonceSize():])
	if err != nil {
		return nil, err
	}

	// Decompress and decode JSON.
	zr, err := gzip.NewReader(bytes.NewReader(plain))
	var wc WrapPCCollection
	jd := json.NewDecoder(zr)
	err = jd.Decode(&wc)
	if err != nil {
		return nil, err
	}
	return &wc, nil
}

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
		wc, err := readEncryptedJournalEntry(f, aead)
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
