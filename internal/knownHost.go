package internal

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

func knownHostsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ssh", "known_hosts")
}

func isHostsKnown(hostname string, key ssh.PublicKey) (bool, error) {
	file, err := os.Open(knownHostsPath())
	if os.IsNotExist(err) {
		return false, nil
	}

	if err != nil {
		return false, err
	}
	defer file.Close()

	hostname = normalizeHostname(hostname)
	keyType := key.Type()
	keyData := base64.StdEncoding.EncodeToString(key.Marshal())

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || len(line) == 0 {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 3 {
			hosts := strings.Split(parts[0], ",")
			for _, h := range hosts {
				if h == hostname && parts[1] == keyType && parts[2] == keyData {
					return true, nil
				}
			}
		}
	}

	return false, scanner.Err()
}

func AddHostKey(hostname string, key ssh.PublicKey) error {
	sshDir := filepath.Dir(knownHostsPath())
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return err
	}

	file, err := os.OpenFile(knownHostsPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	hostname = normalizeHostname(hostname)
	keyType := key.Type()
	keyData := base64.StdEncoding.EncodeToString(key.Marshal())

	line := fmt.Sprintf("%s %s %s\n", hostname, keyType, keyData)
	_, err = file.WriteString(line)
	return err
}

func Fingerprint(key ssh.PublicKey) string {
	hash := sha256.Sum256(key.Marshal())
	return "SHA256:" + base64.StdEncoding.EncodeToString(hash[:])
}

func normalizeHostname(hostname string) string {
	host, port, err := net.SplitHostPort(hostname)
	if err != nil {
		return hostname
	}

	if port == "22" {
		return host
	}

	return fmt.Sprintf("[%s]:%s", host, port)
}

func TOFUHostKeyCallback() ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		known, err := isHostsKnown(hostname, key)
		if err != nil {
			return fmt.Errorf("error while check known hosts: %w", err)
		}

		if known {
			return nil
		}

		fingerprint := Fingerprint(key)

		fmt.Printf("\n")
		fmt.Printf("The authenticity of host '%s' can't be established.\n", hostname)
		fmt.Printf("%s key fingerprint is %s\n", key.Type(), fingerprint)
		fmt.Printf("Are you sure you want to continue connecting (yes/no)? ")
		var answer string
		fmt.Scanln(&answer)
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "yes" {
			return fmt.Errorf("host key verification rejected by user")
		}

		if err := AddHostKey(hostname, key); err != nil {
			fmt.Printf("Warning: Could not save host key: %v\n", key)
		} else {
			fmt.Printf("Warning: Permanently added '%s' to known hosts.\n", hostname)
		}
		return nil
	}
}
