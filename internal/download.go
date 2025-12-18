package internal

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"github.com/schollz/progressbar/v3"
)

type Downloader struct {
	client     *sftp.Client
	workers    int
	bufferSize int
}

func NewDownloader(c *sftp.Client) Downloader {
	return Downloader{
		client:     c,
		workers:    autoWorker(),
		bufferSize: PACKET_SIZE,
	}
}

func (d *Downloader) DownloadFile(localPath, remotePath string, offset int64, progress func(int)) error {
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

	type chunk struct {
		data   []byte
		n      int
		offset int64
	}

	ch := make(chan chunk, d.workers)
	wg := sync.WaitGroup{}
	errChan := make(chan error, 1)

	for i := 0; i < d.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range ch {
				_, err := local.WriteAt(c.data[:c.n], c.offset)
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
	defer wg.Wait()
	defer close(ch)

	buf := make([]byte, d.bufferSize)
	currentOffset := offset

	for {
		select {
		case err := <-errChan:
			return err
		default:
		}

		n, err := remote.Read(buf)
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

	bar := progressbar.NewOptions64(
		remoteInfo.Size(),
		progressbar.OptionSetDescription(truncateString(filepath.Base(remotePath), 15)),
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

	downloader := NewDownloader(sftpClient)
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
	if err := verifyDownloadIntegrity(client, localPath, remotePath); err != nil {
		return fmt.Errorf("integrity check failed: %w", err)
	}
	fmt.Printf("✓ Integrity check passed\n")
	return nil
}

func verifyDownloadIntegrity(client *Client, localPath, remotePath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	localsum := hex.EncodeToString(h.Sum(nil))

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create ssh session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(fmt.Sprintf("md5sum '%s'", remotePath))
	if err != nil {
		return fmt.Errorf("remote md5sum failed: %w, output: %s", err, string(output))
	}
	fields := strings.Fields(string(output))
	if len(fields) < 1 {
		return fmt.Errorf("unexpected md5sum output: %s", string(output))
	}
	remoteSum := fields[0]
	if localsum != remoteSum {
		return fmt.Errorf("checksum mismatch: local=%s, remote=%s", localsum, remoteSum)
	}
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
