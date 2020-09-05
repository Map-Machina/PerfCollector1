package util

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"io/ioutil"
	"os"

	"github.com/businessperformancetuning/perfcollector/util/edkey"
	"golang.org/x/crypto/ssh"
)

func SSHKey(filename string) (ssh.Signer, error) {
	hostKeyData, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKey(hostKeyData)
}

func PublicKeyFile(filename string) (ssh.AuthMethod, error) {
	buffer, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(key), nil
}

// NewSSHKeyPair creates and writes out ed25519 public and private key files.
func NewSSHKeyPair(privateKeyPath string) error {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}

	privateKeyFile, err := os.OpenFile(privateKeyPath,
		os.O_WRONLY|os.O_CREATE, 0600)
	defer privateKeyFile.Close()
	if err != nil {
		return err
	}
	privateKeyPEM := &pem.Block{
		Type:  "OPENSSH PRIVATE KEY",
		Bytes: edkey.MarshalED25519PrivateKey(privateKey),
	}
	if err := pem.Encode(privateKeyFile, privateKeyPEM); err != nil {
		return err
	}

	pub, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(privateKeyPath+".pub",
		ssh.MarshalAuthorizedKey(pub), 0600)
}
