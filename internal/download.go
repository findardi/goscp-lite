package internal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/sftp"
)

func (d *transfer) DownloadFile(localPath, remotePath string, offset int64, progress func(int)) error {
	remote, err := d.client.Open(remotePath)
	if err != nil {
		return err
	}
	defer remote.Close()

	if offset > 0 {
		_, err = remote.Seek(offset, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to seek remote file: %w", err)
		}
	}

	flags := os.O_RDWR | os.O_CREATE
	if offset == 0 {
		flags |= os.O_TRUNC
	}

	local, err := os.OpenFile(localPath, flags, 0644)
	if err != nil {
		return err
	}
	defer local.Close()

	return d.Chunker(remote, local, offset, progress)
}

func Download(user, host string, port int, keypath, localpath, remotepath string) {
	serverAddr, sshCfg := Initiate(user, host, keypath, port)
	client, err := NewClient(serverAddr, sshCfg)
	if err != nil {
		fmt.Printf("✗ Connection failed: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	dataInfo, err := client.SFTP().Stat(remotepath)
	if err != nil {
		fmt.Printf("✗ Cannot access remote path: %v\n", err)
		os.Exit(1)
	}

	if dataInfo.IsDir() {
		err = downloadDir(client, remotepath, localpath)
	} else {
		localInfo, statErr := os.Stat(localpath)
		isLocalDir := statErr == nil && localInfo.IsDir()

		if isLocalDir || strings.HasSuffix(localpath, "/") {
			if !isLocalDir {
				if err := os.MkdirAll(localpath, 0755); err != nil {
					fmt.Printf("✗ Cannot create local directory: %v\n", err)
					os.Exit(1)
				}
			}
			localpath = filepath.ToSlash(filepath.Join(localpath, filepath.Base(remotepath)))
		}
		err = downloadFile(client, remotepath, localpath)
	}

	if err != nil {
		fmt.Printf("✗ Download failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Download successful\n")
}

func downloadFile(client *Client, remotePath, localPath string) error {
	sftpClient := client.SFTP()
	partPath := localPath + ".part"

	remoteInfo, err := sftpClient.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("cannot stat remote file : %w", err)
	}

	var offset int64 = 0
	localPartInfo, err := os.Stat(partPath)
	if err == nil {
		if localPartInfo.Size() < remoteInfo.Size() {
			offset = localPartInfo.Size()
			fmt.Printf("Resuming download from %d bytes (%.2f%%)\n",
				offset, float64(offset)/float64(remoteInfo.Size())*100)
		}
	}

	bar := progressBar(remoteInfo, remotePath)
	if offset > 0 {
		bar.Add64(offset)
	}

	downloader := NewTransfer(sftpClient)
	err = downloader.DownloadFile(partPath, remotePath, offset, func(n int) {
		bar.Add(n)
	})

	if err != nil {
		return err
	}

	_ = os.Remove(localPath)
	err = os.Rename(partPath, localPath)

	if err != nil {
		return fmt.Errorf("failed to rename part file: %w", err)
	}

	fmt.Printf("Verifying integrity...\n")
	if err := verifyIntegrity(client, localPath, remotePath); err != nil {
		return fmt.Errorf("integrity check failed: %w", err)
	}
	fmt.Printf("✓ Integrity check passed\n")
	return nil
}

func downloadDir(client *Client, remoteDir, localDir string) error {
	sftpClient := client.SFTP()

	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("cannot create local directory : %w", err)
	}

	sem := make(chan struct{}, 4)
	var (
		wg   sync.WaitGroup
		ferr error
		mu   sync.Mutex
	)

	err := walkRemoteDir(sftpClient, remoteDir, func(path string, fi os.FileInfo) error {
		mu.Lock()
		if ferr != nil {
			mu.Unlock()
			return fmt.Errorf("previous error")
		}
		mu.Unlock()

		relpath, err := filepath.Rel(remoteDir, path)
		if err != nil {
			return err
		}

		localPath := filepath.Join(localDir, relpath)
		if fi.IsDir() {
			return os.MkdirAll(localPath, 0755)
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(src, dst string) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := downloadFile(client, src, dst); err != nil {
				mu.Lock()
				if ferr == nil {
					ferr = err
				}
				mu.Unlock()
			}
		}(path, localPath)

		return nil
	})

	wg.Wait()
	if err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()
	return ferr
}

func walkRemoteDir(client *sftp.Client, dir string, fn func(string, os.FileInfo) error) error {
	entries, err := client.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		path := filepath.ToSlash(filepath.Join(dir, entry.Name()))

		if err := fn(path, entry); err != nil {
			return err
		}

		if entry.IsDir() {
			if err := walkRemoteDir(client, path, fn); err != nil {
				return err
			}
		}
	}
	return nil
}
