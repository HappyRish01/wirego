package cmd

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var receiveCmd = &cobra.Command{
	Use:   "receive [code] [folder-name]",
	Short: "receive files from a sender",
	Long:  "receive files from a sender using the 5-digit code they provide.",
	Args:  cobra.ExactArgs(2),
	Run:   runReceive,
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
