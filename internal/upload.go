package internal

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func (u *transfer) UploadFile(localPath, remotePath string, offset int64, progress func(int)) error {
	local, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer local.Close()

	if offset > 0 {
		_, err = local.Seek(offset, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to seek local file: %w", err)
		}
	}

	flags := os.O_RDWR | os.O_CREATE
	if offset == 0 {
		flags |= os.O_TRUNC
	}

	remote, err := u.client.OpenFile(remotePath, flags)
	if err != nil {
		return err
	}
	defer remote.Close()

	return u.Chunker(local, remote, offset, progress)
}

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
		err = uploadDir(client, localPath, remotePath)
	} else {
		// file handle
		remoteInfo, statErr := client.SFTP().Stat(remotePath)
		isRemoteDir := statErr == nil && remoteInfo.IsDir()

		if isRemoteDir || strings.HasSuffix(remotePath, "/") {
			if !isRemoteDir {
				if err := client.SFTP().MkdirAll(remotePath); err != nil {
					fmt.Printf("✗ Cannot create remote directory: %v\n", err)
					os.Exit(1)
				}
			}
			remotePath = filepath.ToSlash(filepath.Join(remotePath, filepath.Base(localPath)))
		}
		err = uploadFile(client, localPath, remotePath)
	}

	if err != nil {
		fmt.Printf("✗ Upload failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Upload successful\n")
}

func uploadFile(client *Client, localPath, remotePath string) error {
	sftpClient := client.SFTP()
	partPath := remotePath + ".part"

	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("cannot open local file: %w", err)
	}
	defer localFile.Close()

	fileInfo, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("cannot stat local file: %w", err)
	}

	var offset int64 = 0
	remoteFileInfo, err := sftpClient.Stat(partPath)
	if err == nil {
		// .part file exists
		if remoteFileInfo.Size() < fileInfo.Size() {
			offset = remoteFileInfo.Size()
			fmt.Printf("Resuming upload from %d bytes (%.2f%%)\n", offset, float64(offset)/float64(fileInfo.Size())*100)
		}
	}

	bar := progressBar(fileInfo, localPath)
	if offset > 0 {
		bar.Add64(offset)
	}

	uploader := NewTransfer(sftpClient)
	err = uploader.UploadFile(localPath, partPath, offset, func(n int) {
		bar.Add(n)
	})
	if err != nil {
		return err
	}

	// delete if exists
	_ = sftpClient.Remove(remotePath)
	// Rename .part to actual filename
	err = sftpClient.Rename(partPath, remotePath)
	if err != nil {
		return fmt.Errorf("failed to rename part file: %w", err)
	}

	// Verify File
	fmt.Printf("Verifying integrity...\n")
	if err := verifyIntegrity(client, localPath, remotePath); err != nil {
		return fmt.Errorf("integrity check failed: %w", err)
	}
	fmt.Printf("✓ Integrity check passed\n")

	return nil
}

func uploadDir(client *Client, localDir, remoteDir string) error {
	sftpClient := client.SFTP()
	localDir = filepath.Clean(localDir)

	if err := sftpClient.MkdirAll(remoteDir); err != nil {
		return fmt.Errorf("cannot create remote directory: %w", err)
	}

	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	var ferr error
	var mu sync.Mutex

	err := filepath.WalkDir(localDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		mu.Lock()
		if ferr != nil {
			mu.Unlock()
			return filepath.SkipAll
		}
		mu.Unlock()

		relPath, err := filepath.Rel(localDir, path)
		if err != nil {
			return err
		}

		remotePath := filepath.Join(remoteDir, relPath)
		remotePath = filepath.ToSlash(remotePath)

		if d.IsDir() {
			return sftpClient.MkdirAll(remotePath)
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(src, dst string) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := uploadFile(client, src, dst); err != nil {
				mu.Lock()
				if ferr == nil {
					ferr = err
				}
				mu.Unlock()
			}
		}(path, remotePath)

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
