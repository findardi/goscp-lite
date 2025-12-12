package internal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/sftp"
	"github.com/schollz/progressbar/v3"
)

const (
	MAX_FILE_SIZE = 100 * 1024 * 1024
)

func Upload(user, host string, port int, keypath, localPath, remotePath string) {
	serverAddr, sshCfg := Initiate(user, host, keypath, port)

	client, err := NewClient(serverAddr, sshCfg)
	if err != nil {
		fmt.Printf("✗ Connection failed: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	dataInfo, err := os.Stat(localPath)
	if err != nil {
		fmt.Printf("✗ Cannot access local path: %v\n", err)
		os.Exit(1)
	}

	if dataInfo.IsDir() {
		// directory handle
		fmt.Printf("hit this upload dir\n")
		err = uploadDir(client.SFTP(), localPath, remotePath)
	} else {
		// file handle
		fmt.Printf("hit this upload file\n")
		remoteInfo, statErr := client.SFTP().Stat(remotePath)
		if statErr == nil && remoteInfo.IsDir() {
			remotePath = filepath.ToSlash(filepath.Join(remotePath, filepath.Base(localPath)))
		}
		err = uploadFile(client.SFTP(), localPath, remotePath)
	}

	if err != nil {
		fmt.Printf("✗ Upload failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Upload successful\n")
}

func uploadFile(sftpClient *sftp.Client, localPath, remotePath string) error {
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("cannot open local file: %w", err)
	}
	defer localFile.Close()

	fileInfo, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("cannot stat local file: %w", err)
	}

	// if fileInfo.Size() > MAX_FILE_SIZE {
	// 	// do something goroutine

	// }

	remoteFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("cannot create remote file: %w", err)
	}
	defer remoteFile.Close()

	bar := progressbar.NewOptions64(
		fileInfo.Size(),
		progressbar.OptionSetDescription(filepath.Base(localPath)),
		progressbar.OptionSetWidth(30),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    ">",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionOnCompletion(func() {
			fmt.Println()
		}),
	)

	buf := make([]byte, 32*1024)
	_, err = io.CopyBuffer(io.MultiWriter(remoteFile, bar), localFile, buf)
	if err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}

	// if err := sftpClient.Chmod(remotePath, fileInfo.Mode()); err != nil {
	// 	fmt.Printf("warning: cannot set permissions: %v\n", err)
	// }

	return nil
}

func uploadDir(sftpClient *sftp.Client, localDir, remoteDir string) error {
	if err := sftpClient.MkdirAll(remoteDir); err != nil {
		return fmt.Errorf("cannot create remote directory: %w", err)
	}

	return filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(localDir, path)
		if err != nil {
			return err
		}

		remotePath := filepath.Join(remoteDir, relPath)
		remotePath = filepath.ToSlash(remotePath)

		if info.IsDir() {
			return sftpClient.MkdirAll(remotePath)
		}

		return uploadFile(sftpClient, path, remotePath)
	})
}
