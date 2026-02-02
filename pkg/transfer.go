package pkg

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/webrtc/v3"
)

const (
	ChunkSize         = 65536      // 64KB chunks for better throughput with large files
	MaxConcurrency    = 8          // Number of concurrent file transfers
	BufferSize        = 100        // Channel buffer size
	MetadataLabel     = "metadata" // Control channel for metadata
	FilePrefix        = "file_"    // Prefix for file data channels
	MaxBufferedAmount = 16777216   // 16MB - max buffered before backpressure
)

// FileMetadata contains information about a file to be transferred
type FileMetadata struct {
	Path     string `json:"path"`     // Relative path
	Size     int64  `json:"size"`     // File size in bytes
	IsDir    bool   `json:"is_dir"`   // Is directory
	Checksum string `json:"checksum"` // SHA256 checksum
	Index    int    `json:"index"`    // Transfer index
}

// TransferManager handles concurrent file transfers over WebRTC
type TransferManager struct {
	pc               *webrtc.PeerConnection
	metadataChannel  *webrtc.DataChannel
	fileChannels     map[int]*webrtc.DataChannel
	channelMu        sync.RWMutex
	totalFiles       int
	totalBytes       uint64
	transferredBytes uint64
	errors           []error
	errorMu          sync.Mutex
	wg               sync.WaitGroup
	rootIsDir        bool // Whether rootPath is a directory
}

// NewTransferManager creates a new transfer manager
func NewTransferManager(pc *webrtc.PeerConnection) *TransferManager {
	return &TransferManager{
		pc:           pc,
		fileChannels: make(map[int]*webrtc.DataChannel),
	}
}

// SendFiles sends files/directories concurrently over WebRTC
func (tm *TransferManager) SendFiles(rootPath string) error {
	// Check if rootPath is a directory
	rootInfo, err := os.Stat(rootPath)
	if err != nil {
		return fmt.Errorf("failed to access path: %w", err)
	}
	tm.rootIsDir = rootInfo.IsDir()

	// Collect all files to send
	files, err := tm.collectFiles(rootPath)
	if err != nil {
		return fmt.Errorf("failed to collect files: %w", err)
	}

	// Count only actual files (not directories)
	fileCount := 0
	for _, f := range files {
		if !f.IsDir {
			fileCount++
		}
	}
	tm.totalFiles = fileCount
	fmt.Printf("\nPreparing to send %d files...\n", tm.totalFiles)

	// Create metadata channel
	metadataChannel, err := tm.pc.CreateDataChannel(MetadataLabel, nil)
	if err != nil {
		return fmt.Errorf("failed to create metadata channel: %w", err)
	}
	tm.metadataChannel = metadataChannel

	// Wait for metadata channel to open
	metadataReady := make(chan struct{})
	metadataChannel.OnOpen(func() {
		close(metadataReady)
	})

	<-metadataReady

	// Send file metadata
	metadata := map[string]interface{}{
		"total_files": tm.totalFiles,
		"files":       files,
	}
	metadataJSON, _ := json.Marshal(metadata)
	if err := metadataChannel.Send(metadataJSON); err != nil {
		return fmt.Errorf("failed to send metadata: %w", err)
	}

	// Create worker pool for concurrent transfers
	fileChan := make(chan FileMetadata, MaxConcurrency)

	// Start workers
	for i := 0; i < MaxConcurrency; i++ {
		tm.wg.Add(1)
		go tm.fileWorker(rootPath, fileChan)
	}

	// Feed files to workers
	for _, file := range files {
		if !file.IsDir {
			fileChan <- file
		}
	}
	close(fileChan)

	// Wait for all transfers to complete
	tm.wg.Wait()

	// Check for errors
	if len(tm.errors) > 0 {
		fmt.Println("\nErrors occurred during transfer:")
		for _, err := range tm.errors {
			fmt.Printf("  - %v\n", err)
		}
		return fmt.Errorf("transfer completed with %d errors", len(tm.errors))
	}

	fmt.Printf("\n All %d files transferred successfully!\n", tm.totalFiles)
	return nil
}

// ReceiveFiles receives files concurrently over WebRTC
func (tm *TransferManager) ReceiveFiles(destDir string) error {
	metadataReceived := make(chan map[string]any)

	// Setup metadata channel handler
	tm.pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() == MetadataLabel {
			tm.metadataChannel = dc
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				var metadata map[string]interface{}
				json.Unmarshal(msg.Data, &metadata)
				metadataReceived <- metadata
			})
		} else if len(dc.Label()) > len(FilePrefix) && dc.Label()[:len(FilePrefix)] == FilePrefix {
			// File data channel
			tm.handleFileChannel(dc, destDir)
		}
	})

	// Wait for metadata
	metadata := <-metadataReceived
	tm.totalFiles = int(metadata["total_files"].(float64))

	fmt.Printf("\nReceiving %d files...\n", tm.totalFiles)

	// Wait for all files to be received
	tm.wg.Wait()

	if len(tm.errors) > 0 {
		fmt.Println("\n Errors occurred during transfer:")
		for _, err := range tm.errors {
			fmt.Printf("  • %v\n", err)
		}
		return fmt.Errorf("transfer completed with %d errors", len(tm.errors))
	}

	fmt.Printf("\n✓ All %d files received successfully!\n", tm.totalFiles)
	return nil
}

// fileWorker handles sending individual files concurrently
func (tm *TransferManager) fileWorker(rootPath string, fileChan <-chan FileMetadata) {
	defer tm.wg.Done()

	for file := range fileChan {
		if err := tm.sendFile(rootPath, file); err != nil {
			tm.addError(fmt.Errorf("failed to send %s: %w", file.Path, err))
		}
	}
}

// sendFile sends a single file over its own data channel
func (tm *TransferManager) sendFile(rootPath string, file FileMetadata) error {
	// Create dedicated data channel for this file with ordered and reliable delivery
	// Default WebRTC data channel is reliable (like TCP) when Ordered=true
	dcOptions := &webrtc.DataChannelInit{
		Ordered: func() *bool { b := true; return &b }(),
		// Note: Not setting MaxRetransmits means unlimited retries (reliable)
	}
	channelLabel := fmt.Sprintf("%s%d", FilePrefix, file.Index)
	dc, err := tm.pc.CreateDataChannel(channelLabel, dcOptions)
	if err != nil {
		return err
	}

	// Wait for channel to open
	channelReady := make(chan struct{})
	dc.OnOpen(func() {
		close(channelReady)
	})

	<-channelReady

	// Send file metadata first
	metadataJSON, _ := json.Marshal(file)
	if err := dc.Send(metadataJSON); err != nil {
		return err
	}

	// Open and send file
	var fullPath string
	if tm.rootIsDir {
		fullPath = filepath.Join(rootPath, file.Path)
	} else {
		// Single file case - rootPath is already the full path
		fullPath = rootPath
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Use buffered reader for better performance with large files
	reader := io.NewSectionReader(f, 0, file.Size)
	buffer := make([]byte, ChunkSize)
	var sent int64
	lastProgress := -1

	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			// Handle backpressure - wait if buffer is full
			maxWaitTime := 30 // 30 iterations ~ 3 seconds max wait
			waitCount := 0
			for dc.BufferedAmount() > MaxBufferedAmount {
				if waitCount >= maxWaitTime {
					return fmt.Errorf("timeout waiting for buffer to drain")
				}
				// Check if channel is still open
				if dc.ReadyState() != webrtc.DataChannelStateOpen {
					return fmt.Errorf("channel closed while sending")
				}
				// Small sleep to allow buffer to drain (100ms)
				select {
				case <-channelReady:
					return fmt.Errorf("channel closed signal received")
				default:
					waitCount++
					time.Sleep(100 * time.Millisecond)
				}
			}

			if sendErr := dc.Send(buffer[:n]); sendErr != nil {
				return sendErr
			}
			sent += int64(n)
			atomic.AddUint64(&tm.transferredBytes, uint64(n))

			// Progress indicator (update every 5%)
			progress := int(float64(sent) / float64(file.Size) * 100)
			if progress != lastProgress && progress%5 == 0 {
				fmt.Printf("\r  %s: %d%%", file.Path, progress)
				lastProgress = progress
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	fmt.Printf("\r✓ %s: 100%% (%s)\n", file.Path, formatBytes(uint64(file.Size)))

	// Wait for buffered data to be sent before closing
	// Give receiver time to process all chunks
	for dc.BufferedAmount() > 0 {
		time.Sleep(10 * time.Millisecond)
	}

	// Additional delay to ensure receiver processes everything
	time.Sleep(100 * time.Millisecond)

	// Close the data channel
	dc.Close()
	return nil
}

// handleFileChannel receives a file from a data channel
func (tm *TransferManager) handleFileChannel(dc *webrtc.DataChannel, destDir string) {
	tm.wg.Add(1)

	var file FileMetadata
	var f *os.File
	var received int64
	var lastProgress = -1
	var metadataReceived bool
	var completed bool
	var mu sync.Mutex // Protect shared state

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		mu.Lock()
		defer mu.Unlock()

		if !metadataReceived {
			// First message is metadata
			if err := json.Unmarshal(msg.Data, &file); err != nil {
				tm.addError(fmt.Errorf("failed to parse metadata: %w", err))
				completed = true
				tm.wg.Done()
				return
			}
			metadataReceived = true

			// Create file
			fullPath := filepath.Join(destDir, file.Path)
			if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
				tm.addError(fmt.Errorf("failed to create directory: %w", err))
				completed = true
				tm.wg.Done()
				return
			}

			var err error
			f, err = os.Create(fullPath)
			if err != nil {
				tm.addError(fmt.Errorf("failed to create file %s: %w", file.Path, err))
				completed = true
				tm.wg.Done()
				return
			}
			return
		}

		// Safety check
		if f == nil {
			tm.addError(fmt.Errorf("file not initialized for %s", file.Path))
			if !completed {
				completed = true
				tm.wg.Done()
			}
			return
		}

		// Write data chunk
		n, err := f.Write(msg.Data)
		if err != nil {
			tm.addError(fmt.Errorf("failed to write to %s: %w", file.Path, err))
			f.Close()
			if !completed {
				completed = true
				tm.wg.Done()
			}
			return
		}

		received += int64(n)
		atomic.AddUint64(&tm.transferredBytes, uint64(n))

		// Check if complete
		if received >= file.Size {
			if err := f.Sync(); err != nil {
				tm.addError(fmt.Errorf("failed to sync %s: %w", file.Path, err))
			}

			// Close file before checksum verification
			filePath := f.Name()
			f.Close()
			f = nil

			// Verify checksum
			receivedChecksum, err := calculateChecksum(filePath)
			if err != nil {
				tm.addError(fmt.Errorf("failed to calculate checksum for %s: %w", file.Path, err))
			} else if receivedChecksum != file.Checksum {
				tm.addError(fmt.Errorf("checksum mismatch for %s: expected %s, got %s (file corrupted)",
					file.Path, file.Checksum, receivedChecksum))
				fmt.Printf("✗ %s: CORRUPTED (checksum mismatch)\n", file.Path)
			} else {
				fmt.Printf("✓ %s: 100%% (%s) [verified]\n", file.Path, formatBytes(uint64(file.Size)))
			}

			if !completed {
				completed = true
				tm.wg.Done()
			}
		} else {
			// Progress indicator (update every 5%)
			progress := int(float64(received) / float64(file.Size) * 100)
			if progress != lastProgress && progress%5 == 0 {
				fmt.Printf("\r  %s: %d%%", file.Path, progress)
				lastProgress = progress
			}
		}
	})

	dc.OnClose(func() {
		mu.Lock()
		defer mu.Unlock()

		// Close file if still open
		if f != nil {
			f.Close()
			f = nil
		}

		// If transfer wasn't completed, report error and signal done
		if !completed {
			if metadataReceived {
				tm.addError(fmt.Errorf("channel closed before completing transfer of %s (received %d/%d bytes)",
					file.Path, received, file.Size))
			} else {
				tm.addError(fmt.Errorf("channel closed before receiving metadata"))
			}
			completed = true
			tm.wg.Done()
		}
	})
}

// collectFiles recursively collects all files in a directory or single file
func (tm *TransferManager) collectFiles(rootPath string) ([]FileMetadata, error) {
	var files []FileMetadata
	index := 0

	// Check if rootPath is a single file
	rootInfo, err := os.Stat(rootPath)
	if err != nil {
		return nil, err
	}

	if !rootInfo.IsDir() {
		// Single file case
		checksum, err := calculateChecksum(rootPath)
		if err != nil {
			return nil, err
		}

		file := FileMetadata{
			Path:     filepath.Base(rootPath),
			Size:     rootInfo.Size(),
			IsDir:    false,
			Checksum: checksum,
			Index:    0,
		}
		tm.totalBytes += uint64(rootInfo.Size())
		return []FileMetadata{file}, nil
	}

	// Directory case
	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(rootPath, path)
		if relPath == "." {
			return nil
		}

		file := FileMetadata{
			Path:  relPath,
			Size:  info.Size(),
			IsDir: info.IsDir(),
			Index: index,
		}

		// Calculate checksum for files
		if !info.IsDir() {
			checksum, err := calculateChecksum(path)
			if err != nil {
				return err
			}
			file.Checksum = checksum
			tm.totalBytes += uint64(info.Size())
		}

		files = append(files, file)
		index++
		return nil
	})

	return files, err
}

// calculateChecksum calculates SHA256 checksum of a file
func calculateChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// addError safely adds an error to the error list
func (tm *TransferManager) addError(err error) {
	tm.errorMu.Lock()
	defer tm.errorMu.Unlock()
	tm.errors = append(tm.errors, err)
}

// GetProgress returns current transfer progress
func (tm *TransferManager) GetProgress() (transferred, total uint64, percentage float64) {
	transferred = atomic.LoadUint64(&tm.transferredBytes)
	total = tm.totalBytes
	if total > 0 {
		percentage = float64(transferred) / float64(total) * 100
	}
	return
}

// formatBytes converts bytes to human-readable format
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
