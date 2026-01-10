package util

import (
	"os"
	"testing"
)

func TestGenerateSSHKey(t *testing.T) {
	// Create temp file for private key
	tmpFile, err := os.CreateTemp("", "ssh_test_key")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())
	defer os.Remove(tmpFile.Name() + ".pub")

	err = NewSSHKeyPair(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Verify private key file exists
	if _, err := os.Stat(tmpFile.Name()); os.IsNotExist(err) {
		t.Error("private key file not created")
	}

	// Verify public key file exists
	if _, err := os.Stat(tmpFile.Name() + ".pub"); os.IsNotExist(err) {
		t.Error("public key file not created")
	}
}
