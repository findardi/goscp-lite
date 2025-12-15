package internal

import (
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

	// Use OpenFile with O_RDWR|O_CREATE to allow random access and resuming.
	// Only truncate if we are starting from the beginning (offset 0).
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

	// Ensure workers are waited on and channel is closed upon return
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
		// Check for errors from workers
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

	// Double check error channel after loop
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
		err = uploadDir(client.SFTP(), localPath, remotePath)
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

	var offset int64 = 0
	remoteFileInfo, err := sftpClient.Stat(remotePath)
	if err == nil {
		// Remote file exists
		if remoteFileInfo.Size() < fileInfo.Size() {
			offset = remoteFileInfo.Size()
			fmt.Printf("Resuming upload from %d bytes (%.2f%%)\n", offset, float64(offset)/float64(fileInfo.Size())*100)
		}
	}

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

	if offset > 0 {
		bar.Add64(offset)
	}

	uploader := NewUploader(sftpClient)

	return uploader.UploadFile(localPath, remotePath, offset, func(n int) {
		bar.Add(n)
	})
}

func uploadDir(sftpClient *sftp.Client, localDir, remoteDir string) error {
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

			if err := uploadFile(sftpClient, src, dst); err != nil {
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
