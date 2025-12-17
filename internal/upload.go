package internal

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"github.com/schollz/progressbar/v3"
)

const (
	MAX_FILE_SIZE = 100 * 1024 * 1024
	PACKET_SIZE   = 32 * 1024
)

type Uploader struct {
	client     *sftp.Client
	workers    int
	bufferSize int
}

func NewUploader(c *sftp.Client) Uploader {
	return Uploader{
		client:     c,
		workers:    autoWorker(),
		bufferSize: PACKET_SIZE,
	}
}

func autoWorker() int {
	return 8
}

func (u *Uploader) UploadFile(localPath, remotePath string, offset int64, progress func(int)) error {
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

	type chunk struct {
		data   []byte
		n      int
		offset int64
	}

	ch := make(chan chunk, u.workers)
	wg := sync.WaitGroup{}
	errChan := make(chan error, 1)

	defer wg.Wait()
	defer close(ch)

	for i := 0; i < u.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range ch {
				_, err := remote.WriteAt(c.data[:c.n], c.offset)
				if err != nil {
					select {
					case errChan <- err:
					default:
					}
					return
				}
				if progress != nil {
					progress(c.n)
				}
			}
		}()
	}

	buf := make([]byte, u.bufferSize)
	currentOffset := offset

	for {
		select {
		case err := <-errChan:
			return err
		default:
		}

		n, err := local.Read(buf)
		if n > 0 {
			tmp := make([]byte, n)
			copy(tmp, buf[:n])

			select {
			case ch <- chunk{
				data:   tmp,
				n:      n,
				offset: currentOffset,
			}:
			case err := <-errChan:
				return err
			}
			currentOffset += int64(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	select {
	case err := <-errChan:
		return err
	default:
	}

	return nil
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

	bar := progressbar.NewOptions64(
		fileInfo.Size(),
		progressbar.OptionSetDescription(truncateString(filepath.Base(localPath), 15)),
		progressbar.OptionSetWidth(20),
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

	if offset > 0 {
		bar.Add64(offset)
	}

	uploader := NewUploader(sftpClient)

	// Upload to .part file
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

func verifyIntegrity(client *Client, localPath, remotePath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	localSum := hex.EncodeToString(h.Sum(nil))

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create ssh session for verification: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(fmt.Sprintf("md5sum '%s'", remotePath))
	if err != nil {
		return fmt.Errorf("remote md5sum command failed: %w, output: %s", err, string(output))
	}

	fields := strings.Fields(string(output))
	if len(fields) < 1 {
		return fmt.Errorf("unexpected output from md5sum: %s", string(output))
	}
	remoteSum := fields[0]

	if localSum != remoteSum {
		return fmt.Errorf("checksum mismatch: local=%s, remote=%s", localSum, remoteSum)
	}

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

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
