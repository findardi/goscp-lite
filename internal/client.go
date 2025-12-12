package internal

import (
	"errors"
	"fmt"
	"net"
	"strings"

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
	sftpClient, err := sftp.NewClient(sshClient)
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
	ep, err := addDefaultPort(addr)
	if err != nil {
		return nil, err
	}
	return ssh.Dial("tcp", ep, cfg)
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
