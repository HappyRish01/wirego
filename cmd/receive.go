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
	"github.com/pion/webrtc/v3"
	"github.com/spf13/cobra"
)

var receiveCmd = &cobra.Command{
	Use:   "receive [code] [folder-name]",
	Short: "receive files from a sender",
	Long:  "receive files from a sender using the code they provide.",
	Args:  cobra.ExactArgs(2),
	Run:   runReceive,
}

var localModeReceive bool

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
	receiveCmd.Flags().BoolVarP(&localModeReceive, "local", "l", false, "Use local network mode (default is WebRTC)")
}

func runReceive(cmd *cobra.Command, args []string) {
	codeStr := args[0]
	folderName := args[1]

	// Determine mode: --local flag forces local, otherwise check if numeric code in range 1000-254999
	if localModeReceive {
		// Explicit local mode
		receiveViaLocalNetwork(codeStr, folderName)
		return
	}

	// Try to parse as number
	// code, err := strconv.Atoi(codeStr)
	receiveViaWebRTC(codeStr, folderName)
	// if err == nil && code >= 1000 && code <= 254999 {
	// 	// Numeric 6-digit code in valid range (1000-254999) = local network
	// 	receiveViaLocalNetwork(codeStr, folderName)
	// } else {
	// 	// Anything else (alphanumeric or out of range) = WebRTC
	// }
}

func receiveViaLocalNetwork(codeStr string, folderName string) {

	code, err := strconv.Atoi(codeStr)
	if err != nil || code < 1000 {
		fmt.Println("error: Invalid code. Must be at least 1000")
		os.Exit(1)
	}

	// Decode: lastOctet = code / 1000, portOffset = code % 1000
	lastOctet := code / 1000
	portOffset := code % 1000

	// Validate last octet is in valid range (1-254)
	if lastOctet < 1 || lastOctet > 254 {
		fmt.Printf("error: Invalid code. IP octet (%d) must be between 1-254\n", lastOctet)
		os.Exit(1)
	}

	// Validate port offset is reasonable (0-999)
	if portOffset < 0 || portOffset > 999 {
		fmt.Printf("error: Invalid code. Port offset (%d) out of range\n", portOffset)
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
	fmt.Printf("Code: %06d\n", code)
	fmt.Printf("Save to: %s\n", saveDir)
	fmt.Println(strings.Repeat("─", 40))

	// Port already decoded during validation
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

	startTime := pkg.Start()      // Start timer right before data transfer
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

func receiveViaWebRTC(codeStr string, folderName string) {
	// Convert folder name to absolute path
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

	// Create save directory
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		fmt.Println("error creating directory:", err)
		os.Exit(1)
	}

	fmt.Println("\nWireGo - WebRTC File Sharing")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Printf("Remote Code: %s\n", codeStr)
	fmt.Printf("Save to: %s\n", saveDir)
	fmt.Println(strings.Repeat("─", 40))

	fmt.Println("\nFetching sender's offer from signaling server...")

	// Fetch offer from signaling server
	sig := pkg.NewSignalingClient(codeStr)
	offerSDP, err := sig.GetOffer(codeStr)
	if err != nil {
		fmt.Println("Error:", err)
		fmt.Println("\nNote: Make sure the sender is online and the code is correct.")
		fmt.Println("For local network transfers, use: wirego receive <code> <folder> --local")
		os.Exit(1)
	}
	defer sig.Close()

	// Create WebRTC connection
	fmt.Println("Creating WebRTC connection...")
	conn, err := pkg.NewWebRTCConnection()
	if err != nil {
		fmt.Println("Error creating WebRTC connection:", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Set remote offer
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerSDP,
	}
	err = conn.PC.SetRemoteDescription(offer)
	if err != nil {
		fmt.Println("Error setting remote description:", err)
		os.Exit(1)
	}

	// Create TransferManager for receiving
	tm := pkg.NewTransferManager(conn.PC)

	// Create answer and wait for ICE gathering
	fmt.Println("Creating answer and gathering ICE candidates...")
	answer, err := conn.PC.CreateAnswer(nil)
	if err != nil {
		fmt.Println("Error creating answer:", err)
		os.Exit(1)
	}

	err = conn.PC.SetLocalDescription(answer)
	if err != nil {
		fmt.Println("Error setting local description:", err)
		os.Exit(1)
	}

	// Wait for ICE gathering
	<-webrtc.GatheringCompletePromise(conn.PC)

	// Send answer back
	fmt.Println("Sending answer to sender...")
	err = sig.SendAnswer(conn.PC.LocalDescription().SDP)
	if err != nil {
		fmt.Println("Error sending answer:", err)
		os.Exit(1)
	}

	// Wait for connection and receive files
	fmt.Println("Establishing P2P connection...")
	if err := tm.ReceiveFiles(saveDir); err != nil {
		fmt.Printf("Transfer error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nTransfer complete!")
	fmt.Printf("Files saved to: %s\n", saveDir)
}
