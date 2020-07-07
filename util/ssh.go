package util

import (
	"io/ioutil"

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
