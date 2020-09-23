package util

import "testing"

func TestGenerateSSHKey(t *testing.T) {
	err := NewSSHKeyPair("pub", "priv")
	if err != nil {
		t.Fatal(err)
	}
}
