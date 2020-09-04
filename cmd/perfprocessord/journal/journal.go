package journal

import (
	"bytes"
	"compress/gzip"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/businessperformancetuning/perfcollector/types"
)

var (
	mtx sync.Mutex
)

func encrypt(aead cipher.AEAD, msg []byte) ([]byte, error) {
	// Select a random nonce, and leave capacity for the ciphertext.
	length := 4 + aead.NonceSize() + len(msg) + aead.Overhead()
	nonce := make([]byte, 4+aead.NonceSize(), length)
	if _, err := rand.Read(nonce[4 : 4+aead.NonceSize()]); err != nil {
		return nil, err
	}
	binary.LittleEndian.PutUint32(nonce, uint32(length)-4)
	// Encrypt the message and append the ciphertext to the nonce.
	return aead.Seal(nonce, nonce[4:4+aead.NonceSize()], msg, nil), nil
}

func decrypt(aead cipher.AEAD, nonce, ciphertext []byte) ([]byte, error) {
	// Decrypt the message and check it wasn't tampered with.
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

func Journal(filename string, aead cipher.AEAD, payload interface{}) error {
	// Compress the encoded JSON
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	e := json.NewEncoder(zw)
	err := e.Encode(payload)
	if err != nil {
		return err
	}
	zw.Close()

	// Encrypt compressed JSON.
	blob, err := encrypt(aead, buf.Bytes())
	if err != nil {
		return err
	}

	// Write to file
	mtx.Lock()
	defer mtx.Unlock()
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0640)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(blob)
	if err != nil {
		return err
	}
	return nil
}

// XXX this really doesn't belong here.
type WrapPCCollection struct {
	Site        uint64
	Host        uint64
	Run         uint64
	Measurement *types.PCCollection
}

func ReadEncryptedJournalEntry(f *os.File, aead cipher.AEAD) (*WrapPCCollection, error) {
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
