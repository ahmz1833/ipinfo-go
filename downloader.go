package main

import (
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func downloadFileWithProgress(rawURL, destPath string) error {
	client := &http.Client{}

	totalSize, supportsRange, err := probeRemoteFile(client, rawURL)
	if err != nil {
		return err
	}

	if supportsRange && totalSize > 0 {
		return downloadMultiStream(client, rawURL, destPath, totalSize, downloadConnections)
	}

	log.Printf("Range requests not available; falling back to single stream")
	return downloadSingleStream(client, rawURL, destPath, totalSize)
}

func probeRemoteFile(client *http.Client, rawURL string) (int64, bool, error) {
	var totalSize int64
	supportsRange := false

	headReq, err := http.NewRequest(http.MethodHead, rawURL, nil)
	if err != nil {
		return 0, false, fmt.Errorf("failed to build HEAD request: %w", err)
	}

	headResp, err := client.Do(headReq)
	if err == nil {
		totalSize = headResp.ContentLength
		supportsRange = strings.Contains(strings.ToLower(headResp.Header.Get("Accept-Ranges")), "bytes")
		headResp.Body.Close()
		if supportsRange && totalSize > 0 {
			return totalSize, true, nil
		}
	}

	probeReq, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, false, fmt.Errorf("failed to build range probe request: %w", err)
	}
	probeReq.Header.Set("Range", "bytes=0-0")

	probeResp, err := client.Do(probeReq)
	if err != nil {
		return 0, false, fmt.Errorf("range probe failed: %w", err)
	}
	defer probeResp.Body.Close()

	if probeResp.StatusCode == http.StatusPartialContent {
		supportsRange = true
		if totalSize <= 0 {
			totalSize = parseContentRangeTotal(probeResp.Header.Get("Content-Range"))
		}
	}

	if totalSize <= 0 && probeResp.ContentLength > 0 {
		totalSize = probeResp.ContentLength
	}

	return totalSize, supportsRange, nil
}

func downloadMultiStream(client *http.Client, rawURL, destPath string, totalSize int64, maxConnections int) error {
	if maxConnections < 1 {
		maxConnections = 1
	}

	connections := maxConnections
	if totalSize < int64(maxConnections) {
		connections = int(totalSize)
	}
	if connections < 1 {
		connections = 1
	}

	log.Printf("Starting multi-stream download with %d connections (size: %s)", connections, humanBytes(totalSize))

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer file.Close()

	if err := file.Truncate(totalSize); err != nil {
		return fmt.Errorf("failed to preallocate destination file: %w", err)
	}

	var downloaded atomic.Uint64
	stopProgress := startProgressLogger(&downloaded, totalSize)
	defer stopProgress()

	chunkSize := int64(math.Ceil(float64(totalSize) / float64(connections)))
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex

	for i := 0; i < connections; i++ {
		start := int64(i) * chunkSize
		if start >= totalSize {
			break
		}
		end := start + chunkSize - 1
		if end >= totalSize {
			end = totalSize - 1
		}

		wg.Add(1)
		go func(part, rangeStart, rangeEnd int64) {
			defer wg.Done()

			req, err := http.NewRequest(http.MethodGet, rawURL, nil)
			if err != nil {
				setDownloadErr(&errMu, &firstErr, fmt.Errorf("part %d request build failed: %w", part, err))
				return
			}
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", rangeStart, rangeEnd))

			resp, err := client.Do(req)
			if err != nil {
				setDownloadErr(&errMu, &firstErr, fmt.Errorf("part %d request failed: %w", part, err))
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusPartialContent {
				setDownloadErr(&errMu, &firstErr, fmt.Errorf("part %d unexpected status: %s", part, resp.Status))
				return
			}

			n, err := copyRangeToFile(file, resp.Body, rangeStart, &downloaded)
			if err != nil {
				setDownloadErr(&errMu, &firstErr, fmt.Errorf("part %d copy failed: %w", part, err))
				return
			}

			expected := rangeEnd - rangeStart + 1
			if n != expected {
				setDownloadErr(&errMu, &firstErr, fmt.Errorf("part %d incomplete: wrote %d of %d bytes", part, n, expected))
				return
			}
		}(int64(i), start, end)
	}

	wg.Wait()
	if firstErr != nil {
		return firstErr
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync destination file: %w", err)
	}

	return nil
}

func copyRangeToFile(file *os.File, src io.Reader, offset int64, counter *atomic.Uint64) (int64, error) {
	buf := make([]byte, 256*1024)
	var totalWritten int64

	for {
		nr, readErr := src.Read(buf)
		if nr > 0 {
			nw, writeErr := file.WriteAt(buf[:nr], offset+totalWritten)
			if nw > 0 {
				totalWritten += int64(nw)
				if counter != nil {
					counter.Add(uint64(nw))
				}
			}
			if writeErr != nil {
				return totalWritten, writeErr
			}
			if nw != nr {
				return totalWritten, io.ErrShortWrite
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return totalWritten, readErr
		}
	}

	return totalWritten, nil
}

func downloadSingleStream(client *http.Client, rawURL, destPath string, totalSize int64) error {
	log.Printf("Starting single-stream download")

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("failed to build GET request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned non-200 status: %s", resp.Status)
	}

	if totalSize <= 0 && resp.ContentLength > 0 {
		totalSize = resp.ContentLength
	}

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer file.Close()

	var downloaded atomic.Uint64
	stopProgress := startProgressLogger(&downloaded, totalSize)
	defer stopProgress()

	if _, err := io.Copy(file, io.TeeReader(resp.Body, &countingWriter{counter: &downloaded})); err != nil {
		return fmt.Errorf("failed to write downloaded data: %w", err)
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync destination file: %w", err)
	}

	return nil
}

func setDownloadErr(mu *sync.Mutex, dst *error, err error) {
	mu.Lock()
	defer mu.Unlock()
	if *dst == nil {
		*dst = err
	}
}

func parseContentRangeTotal(contentRange string) int64 {
	parts := strings.Split(contentRange, "/")
	if len(parts) != 2 {
		return 0
	}

	total, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0
	}

	return total
}

func startProgressLogger(downloaded *atomic.Uint64, totalSize int64) func() {
	ticker := time.NewTicker(progressInterval)
	stop := make(chan struct{})

	go func() {
		var lastBytes uint64
		lastTime := time.Now()

		for {
			select {
			case <-ticker.C:
				currentBytes := downloaded.Load()
				now := time.Now()
				elapsed := now.Sub(lastTime).Seconds()
				if elapsed <= 0 {
					elapsed = 1
				}

				rate := float64(currentBytes-lastBytes) / elapsed
				if totalSize > 0 {
					percent := (float64(currentBytes) / float64(totalSize)) * 100
					log.Printf("Download progress: %.1f%% (%s/%s) speed %s/s", percent, humanBytes(int64(currentBytes)), humanBytes(totalSize), humanBytes(int64(rate)))
				} else {
					log.Printf("Download progress: %s speed %s/s", humanBytes(int64(currentBytes)), humanBytes(int64(rate)))
				}

				lastBytes = currentBytes
				lastTime = now
			case <-stop:
				currentBytes := downloaded.Load()
				if totalSize > 0 {
					log.Printf("Download finished: %s/%s", humanBytes(int64(currentBytes)), humanBytes(totalSize))
				} else {
					log.Printf("Download finished: %s", humanBytes(int64(currentBytes)))
				}
				return
			}
		}
	}()

	return func() {
		ticker.Stop()
		close(stop)
	}
}

type countingWriter struct {
	counter *atomic.Uint64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	if w.counter != nil {
		w.counter.Add(uint64(len(p)))
	}
	return len(p), nil
}

func humanBytes(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}

	units := []string{"KB", "MB", "GB", "TB"}
	value := float64(size)
	unitIdx := -1
	for value >= 1024 && unitIdx+1 < len(units) {
		value /= 1024
		unitIdx++
	}

	return fmt.Sprintf("%.2f %s", value, units[unitIdx])
}
