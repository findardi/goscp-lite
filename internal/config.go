package internal

import (
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
)

const defaultConnTimeout = 3 * time.Second

func NewSSHCfgPrivateKey(username string, privPem []byte, passphrase ...string) (cfg *ssh.ClientConfig, err error) {
	var priv ssh.Signer

	if len(passphrase) > 0 && len(passphrase[0]) > 0 {
		pw := passphrase[0]
		priv, err = ssh.ParsePrivateKeyWithPassphrase(privPem, []byte(pw))
	} else {
		priv, err = ssh.ParsePrivateKey(privPem)
	}

	if err != nil {
		return
	}

	cfg = &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(priv),
		},
		HostKeyCallback: TOFUHostKeyCallback(),
		Timeout:         defaultConnTimeout,
	}

	return
}

func NewSSHCfgWithAllKeys(username string) (*ssh.ClientConfig, error) {
	var signers []ssh.Signer

	for _, path := range DefaultKeyPaths() {
		keyData, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		signer, err := ssh.ParsePrivateKey(keyData)
		if err != nil {
			continue
		}

		signers = append(signers, signer)
	}

	if len(signers) == 0 {
		return nil, ErrNoSSHClients
	}

	return &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signers...),
		},
		HostKeyCallback: TOFUHostKeyCallback(),
		Timeout:         defaultConnTimeout,
	}, nil
}

func DefaultKeyPaths() []string {
	home, _ := os.UserHomeDir()
	sshDir := filepath.Join(home, ".ssh")

	return []string{
		filepath.Join(sshDir, "id_ecdsa"),
		filepath.Join(sshDir, "id_ed25519"),
		filepath.Join(sshDir, "id_rsa"),
	}
}

func FindFirstKey() (string, error) {
	for _, path := range DefaultKeyPaths() {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", ErrNoSSHClients
}
