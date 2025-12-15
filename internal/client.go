package internal

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

var (
	ErrNoClientOption = errors.New("client options not provided")
	ErrNoSSHClients   = errors.New("no SSH key found in ~/.ssh/ (tried: id_ed25519, id_rsa, id_ecdsa)")
)

type Client struct {
	*ssh.Client
	sftp *sftp.Client
}

func NewClient(serverAddr string, sshCfg *ssh.ClientConfig) (*Client, error) {
	// Dial SSH
	sshClient, err := dialServer(serverAddr, sshCfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial failed: %w", err)
	}

	// create sftp session
	sftpClient, err := sftp.NewClient(
		sshClient,
		sftp.MaxPacket(32*1024),
		sftp.MaxConcurrentRequestsPerFile(64),
	)
	if err != nil {
		sshClient.Close()
		return nil, fmt.Errorf("sftp session failed: %w", err)
	}

	return &Client{
		Client: sshClient,
		sftp:   sftpClient,
	}, nil
}

func (c *Client) SFTP() *sftp.Client {
	return c.sftp
}

func (c *Client) Close() error {
	if c.sftp != nil {
		c.sftp.Close()
	}
	return c.Client.Close()
}

func dialServer(addr string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
	addr, err := addDefaultPort(addr)
	if err != nil {
		return nil, err
	}
	return func() (*ssh.Client, error) {
		d := &net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}

		conn, err := d.Dial("tcp", addr)
		if err != nil {
			return nil, err
		}

		sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
		if err != nil {
			return nil, err
		}

		return ssh.NewClient(sshConn, chans, reqs), nil
	}()
}

func addDefaultPort(addr string) (string, error) {
	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.Contains(err.Error(), "missing port") {
			newAddr := net.JoinHostPort(strings.Trim(addr, "[]"), "22")
			if _, _, err := net.SplitHostPort(newAddr); err != nil {
				return newAddr, nil
			}
		}

		return "", fmt.Errorf("error parsing server address: %s", err)
	}

	return addr, nil
}

func Initiate(user, host, keyPath string, port int) (string, *ssh.ClientConfig) {
	if user == "" {
		user = "root"
	}

	if port == 0 {
		port = 22
	}

	serverAddr := fmt.Sprintf("%s:%d", host, port)

	var (
		sshCfg *ssh.ClientConfig
		err    error
	)

	if keyPath != "" {
		keyData, err := os.ReadFile(keyPath)
		if err != nil {
			fmt.Printf("✗ Failed to read key: %v\n", err)
			os.Exit(1)
		}
		sshCfg, err = NewSSHCfgPrivateKey(user, keyData)
		if err != nil {
			fmt.Printf("✗ Failed to create SSH config: %v\n", err)
			os.Exit(1)
		}
	} else {
		sshCfg, err = NewSSHCfgWithAllKeys(user)
		if err != nil {
			fmt.Printf("✗ %v\n", err)
			os.Exit(1)
		}
	}

	sshCfg.Config = ssh.Config{
		Ciphers: []string{
			"chacha20-poly1305@openssh.com",
			"aes128-gcm@openssh.com",
		},
		MACs: []string{
			"hmac-sha2-256",
		},
	}

	return serverAddr, sshCfg
}
