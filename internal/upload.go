package internal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	workes     int
	bufferSize int
}

func NewUploader(c *sftp.Client) Uploader {
	return Uploader{
		client:     c,
		workes:     autoWorker(),
		bufferSize: PACKET_SIZE,
	}
}

func autoWorker() int {
	return 8
}

func (u *Uploader) UploadFile(localPath, remotePath string, progress func(int)) error {
	local, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer local.Close()

	remote, err := u.client.Create(remotePath)
	if err != nil {
		return err
	}
	defer remote.Close()

	type chunk struct {
		data []byte
		n    int
	}

	ch := make(chan chunk, u.workes)
	wg := sync.WaitGroup{}

	for i := 0; i < u.workes; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range ch {
				_, _ = remote.Write(c.data[:c.n])
				if progress != nil {
					progress(c.n)
				}
			}
		}()
	}

	buf := make([]byte, u.bufferSize)
	for {
		n, err := local.Read(buf)
		if n > 0 {
			tmp := make([]byte, n)
			copy(tmp, buf[:n])
			ch <- chunk{
				data: tmp,
				n:    n,
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			close(ch)
			wg.Wait()
			return err
		}
	}

	close(ch)
	wg.Wait()
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

	uploader := NewUploader(sftpClient)

	return uploader.UploadFile(localPath, remotePath, func(n int) {
		bar.Add(n)
	})
}

func uploadDir(sftpClient *sftp.Client, localDir, remotePath string) error {
	if err := sftpClient.MkdirAll(remotePath); err != nil {
		return fmt.Errorf("cannot create remote directory: %w", err)
	}

	sem := make(chan struct{}, 4)
	return filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return sftpClient.MkdirAll(remotePath)
		}

		sem <- struct{}{}
		go func() {
			defer func() { <-sem }()
			_ = uploadFile(sftpClient, path, remotePath)
		}()

		return nil
	})
}
