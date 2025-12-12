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

	if err != nil {
		return nil, err
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

func DefaultKeyPaths() []string {
	home, _ := os.UserHomeDir()
	sshDir := filepath.Join(home, ".ssh")

	return []string{
		filepath.Join(sshDir, "id_rsa"),
		filepath.Join(sshDir, "id_ed25519"),
		filepath.Join(sshDir, "id_ecdsa"),
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
