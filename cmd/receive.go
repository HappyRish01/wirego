package cmd

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/HappyRish01/wirego/pkg"
	"github.com/spf13/cobra"
)

var receiveCmd = &cobra.Command{
	Use:   "receive [code] [folder-name]",
	Short: "receive files from a sender",
	Long:  "receive files from a sender using the 5-digit code they provide.",
	Args:  cobra.ExactArgs(2),
	Run:   runReceive,
}

type countingReader struct {
	r     io.Reader
	count *uint64
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	atomic.AddUint64(cr.count, uint64(n))
	return n, err
}

func init() {
	root.AddCommand(receiveCmd)
}

func runReceive(cmd *cobra.Command, args []string) {
	codeStr := args[0]
	folderName := args[1]

	code, err := strconv.Atoi(codeStr)
	if err != nil || code < 100 || code > 25499 {
		fmt.Println("error: Invalid code. Code must be between 100 and 25499")
		os.Exit(1)
	}

	// Validate last octet is in valid range (1-254)
	lastOctet := code / 100
	if lastOctet < 1 || lastOctet > 254 {
		fmt.Println("error: Invalid code. Decoded IP octet out of range")
		os.Exit(1)
	}

	// Convert folder name to absolute path in current directory
	var saveDir string
	if filepath.IsAbs(folderName) {
		saveDir = folderName
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Println("error getting current directory:", err)
			os.Exit(1)
		}
		saveDir = filepath.Join(cwd, folderName)
	}

	// Create save directory if it doesn't exist
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		fmt.Println("error creating directory:", err)
		os.Exit(1)
	}

	fmt.Println("\nWireGo - P2P File Sharing")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Printf("Code: %05d\n", code)
	fmt.Printf("Save to: %s\n", saveDir)
	fmt.Println(strings.Repeat("─", 40))

	// Decode the 5-digit code
	// code = lastOctet * 100 + portOffset
	// lastOctet already calculated during validation
	portOffset := code % 100
	port := 8080 + portOffset

	// Get local network prefix
	localIP, err := getLocalIP()
	if err != nil {
		fmt.Println("error getting local IP:", err)
		os.Exit(1)
	}

	// Extract network prefix (e.g., "192.168.1.")
	parts := strings.Split(localIP, ".")
	if len(parts) != 4 {
		fmt.Println("error: invalid local IP format")
		os.Exit(1)
	}
	networkPrefix := strings.Join(parts[:3], ".") + "."

	// Construct sender IP directly from code
	senderIP := fmt.Sprintf("%s%d", networkPrefix, lastOctet)

	fmt.Printf("\nConnecting to sender at %s:%d...\n", senderIP, port)

	// Download the file
	url := fmt.Sprintf("http://%s:%d/", senderIP, port)
	err = downloadFile(url, saveDir)
	if err != nil {
		fmt.Println("Error downloading:", err)
		os.Exit(1)
	}

	fmt.Println("\nDownload complete!")
	fmt.Printf("Files saved to: %s\n", saveDir)
}

func downloadFile(url, saveDir string) error {

	var totalData uint64
	startTime := pkg.Start()
	// Use a transport with connection timeout but no overall timeout for large files
	transport := &http.Transport{
		ResponseHeaderTimeout: 10 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   0, // No overall timeout - allow large files
	}

	resp, err := client.Get(url)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return fmt.Errorf("sender not found. Make sure sender is running and code is correct")
		}
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline") {
			return fmt.Errorf("connection timed out. Check if sender is on the same network")
		}
		return err
	}
	defer resp.Body.Close()

	cr := &countingReader{r: resp.Body, count: &totalData}

	// Extract filename from Content-Disposition
	filename := "downloaded_file"
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if idx := strings.Index(cd, "filename="); idx != -1 {
			filename = strings.Trim(cd[idx+9:], `"`)
		}
	}

	filePath := filepath.Join(saveDir, filename)

	out, err := os.Create(filePath)
	if err != nil {
		return nil
	}
	defer out.Close()
	// i can't load the whole response body into ram i need to buffer it

	buf := make([]byte, 512*1024) // 512 kB budder babe
	_, err = io.CopyBuffer(out, cr, buf)

	if err != nil {
		return err
	}

	pkg.Ttt(totalData, startTime)

	// Check if it's a zip file
	if strings.HasSuffix(strings.ToLower(filename), ".zip") {
		err = extractZip(filePath, saveDir)
		if err != nil {
			return err
		}
		// delete zip after extraction
		_ = os.Remove(filePath)
	}

	return nil
}

func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(destDir, f.Name)

		// Zip Slip protection
		if !strings.HasPrefix(filepath.Clean(fpath), filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return err
		}

		dst, err := os.Create(fpath)
		if err != nil {
			return err
		}

		src, err := f.Open()
		if err != nil {
			dst.Close()
			return err
		}

		_, err = io.Copy(dst, src)

		dst.Close()
		src.Close()

		if err != nil {
			return err
		}
	}

	return nil
}
