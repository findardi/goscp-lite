package internal

import (
	"fmt"
	"os"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func testConn(serverAddr string, sshCfg *ssh.ClientConfig) error {
	sshClient, err := dialServer(serverAddr, sshCfg)
	if err != nil {
		return fmt.Errorf("ssh dial failed: %w", err)
	}
	defer sshClient.Close()

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return fmt.Errorf("sftp session failed: %w", err)
	}
	defer sftpClient.Close()

	return nil
}

func Test(user, host string, port int, keyPath string) {
	if user == "" {
		user = "root"
	}

	if port == 0 {
		port = 22
	}

	if keyPath == "" {
		detected, err := FindFirstKey()
		if err != nil {
			fmt.Printf("✗ %v\n", err)
			os.Exit(1)
		}
		keyPath = detected
	}

	serverAddr := fmt.Sprintf("%s:%d", host, port)

	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		fmt.Printf("✗ Failed to read key: %v\n", err)
		os.Exit(1)
	}

	sshCfg, err := NewSSHCfgPrivateKey(user, keyData)
	if err != nil {
		fmt.Printf("✗ Failed to create SSH config: %v\n", err)
		os.Exit(1)
	}

	if err := testConn(serverAddr, sshCfg); err != nil {
		fmt.Printf("✗ Connection failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Connection successful to %s\n", serverAddr)
}
