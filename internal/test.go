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

func Test(user, host, keyPath string, port int) {
	serverAddr, sshCfg := Initiate(user, host, keyPath, port)

	if err := testConn(serverAddr, sshCfg); err != nil {
		fmt.Printf("✗ Connection failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Connection successful to %s\n", serverAddr)
}
