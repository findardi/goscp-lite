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

const (
	MAX_FILE_SIZE = 100 * 1024 * 1024
	PACKET_SIZE   = 32 * 1024
)

type transfer struct {
	client     *sftp.Client
	workers    int
	bufferSize int
}

func NewTransfer(c *sftp.Client) transfer {
	return transfer{
		client:     c,
		workers:    autoWorker(),
		bufferSize: PACKET_SIZE,
	}
}

func autoWorker() int {
	return 8
}

func (t *transfer) Chunker(reader io.Reader, writer io.WriterAt, offset int64, progress func(int)) error {
	type chunk struct {
		data   []byte
		n      int
		offset int64
	}

	ch := make(chan chunk, t.workers)
	wg := sync.WaitGroup{}
	errChan := make(chan error, 1)

	for i := 0; i < t.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range ch {
				_, err := writer.WriteAt(c.data[:c.n], c.offset)
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

	buf := make([]byte, t.bufferSize)
	currentOffset := offset

	for {
		select {
		case err := <-errChan:
			return err
		default:
		}

		n, err := reader.Read(buf)
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

func progressBar(file os.FileInfo, path string) *progressbar.ProgressBar {
	return progressbar.NewOptions64(
		file.Size(),
		progressbar.OptionSetDescription(truncateString(filepath.Base(path), 15)),
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
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
