package cmd

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var receiveCmd = &cobra.Command{
	Use:   "receive [code] [save-directory]",
	Short: "receive files from a sender",
	Long:  "receive files from a sender using the 3-digit code they provide.",
	Args:  cobra.ExactArgs(2),
	Run:   runReceive,
}

func init() {
	root.AddCommand(receiveCmd)
}

func runReceive(cmd *cobra.Command, args []string) {
	codeStr := args[0]
	saveDir := args[1]

	code, err := strconv.Atoi(codeStr)
	if err != nil || code < 100 || code > 999 {
		fmt.Println("Error: Invalid code. Must be a 3-digit number (100-999)")
		os.Exit(1)
	}

	// Create save directory if it doesn't exist
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		fmt.Println("Error creating directory:", err)
		os.Exit(1)
	}

	fmt.Println("\nWireGo - P2P File Sharing")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Printf("Code: %03d\n", code)
	fmt.Printf("Save to: %s\n", saveDir)
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println("\nSearching for sender on local network...")

	// Calculate port from code
	port := 8080 + (code % 100)

	// Get local network prefix
	localIP, err := getLocalIP()
	if err != nil {
		fmt.Println("Error getting local IP:", err)
		os.Exit(1)
	}

	// Extract network prefix (e.g., "192.168.1.")
	parts := strings.Split(localIP, ".")
	if len(parts) != 4 {
		fmt.Println("Error: Invalid local IP format")
		os.Exit(1)
	}
	networkPrefix := strings.Join(parts[:3], ".") + "."

	// Scan local network for the sender
	senderIP := scanForSender(networkPrefix, port)
	if senderIP == "" {
		fmt.Println("\nCould not find sender on local network.")
		fmt.Println("   Make sure:")
		fmt.Println("   - You're on the same network as the sender")
		fmt.Println("   - The sender is still running 'wirego send'")
		fmt.Println("   - The code is correct")
		os.Exit(1)
	}

	fmt.Printf("\nFound sender at %s:%d\n", senderIP, port)
	fmt.Println("Starting download...")

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

func scanForSender(networkPrefix string, port int) string {
	// Create a channel for results
	results := make(chan string, 255)
	timeout := time.Millisecond * 500

	// Scan all IPs in the network
	for i := 1; i <= 254; i++ {
		go func(ip string) {
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), timeout)
			if err == nil {
				conn.Close()
				results <- ip
			} else {
				results <- ""
			}
		}(networkPrefix + strconv.Itoa(i))
	}

	// Collect results
	var foundIP string
	for i := 0; i < 254; i++ {
		ip := <-results
		if ip != "" && foundIP == "" {
			foundIP = ip
		}
	}

	return foundIP
}

func downloadFile(url, saveDir string) error {
	client := &http.Client{
		Timeout: time.Hour, // Allow long downloads
	}

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check Content-Type to determine if it's a zip
	contentType := resp.Header.Get("Content-Type")
	contentDisposition := resp.Header.Get("Content-Disposition")

	// Extract filename from Content-Disposition
	filename := "downloaded_file"
	if contentDisposition != "" {
		if idx := strings.Index(contentDisposition, "filename="); idx != -1 {
			filename = contentDisposition[idx+9:]
			filename = strings.Trim(filename, "\"")
		}
	}

	// Read the entire response
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Check if it's a zip file
	if contentType == "application/zip" || strings.HasSuffix(filename, ".zip") {
		// Extract zip
		return extractZip(data, saveDir)
	}

	// Save as regular file
	// Remove .zip extension for display if present
	displayName := strings.TrimSuffix(filename, ".zip")
	if displayName == filename {
		displayName = filename
	}

	filePath := filepath.Join(saveDir, filename)
	return os.WriteFile(filePath, data, 0644)
}

func extractZip(data []byte, destDir string) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}

	for _, file := range reader.File {
		filePath := filepath.Join(destDir, file.Name)

		// Prevent zip slip vulnerability
		if !strings.HasPrefix(filepath.Clean(filePath), filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			os.MkdirAll(filePath, 0755)
			continue
		}

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return err
		}

		// Extract file
		outFile, err := os.Create(filePath)
		if err != nil {
			return err
		}

		rc, err := file.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}

		fmt.Printf("    Extracted: %s\n", file.Name)
	}

	return nil
}
