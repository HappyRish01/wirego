package cmd

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HappyRish01/wirego/pkg"
	"github.com/spf13/cobra"
)

var sendCmd = &cobra.Command{
	Use:   "send [file or directory]",
	Short: "Send a file or directory",
	Long:  "Send a file or directory to another peer. Use '.' for current directory.",
	Args:  cobra.ExactArgs(1),
	Run:   runSend,
}

var localMode bool

func init() {
	root.AddCommand(sendCmd)
	sendCmd.Flags().BoolVarP(&localMode, "local", "l", false, "Use local network mode (default is WebRTC)")
}

type countingWriter struct {
	w     io.Writer
	count *uint64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	atomic.AddUint64(cw.count, uint64(n))
	return n, err
}

func runSend(cmd *cobra.Command, args []string) {
	path := args[0]

	// checks path exists
	info, err := os.Stat(path)
	if err != nil {
		fmt.Printf("error: %s does not exist\n", path)
		os.Exit(1)
	}

	// Choose mode: local network or WebRTC (default)
	if localMode {
		sendViaLocalNetwork(path, info)
	} else {
		sendViaWebRTC(path, info)
	}
}

func sendViaLocalNetwork(path string, info os.FileInfo) {

	// get local IP
	localIP, err := getLocalIP()
	if err != nil {
		fmt.Println("error getting local IP:", err)
		os.Exit(1)
	}

	// extract last octet from IP
	lastOctet, err := getLastOctet(localIP)
	if err != nil {
		fmt.Println("error parsing IP:", err)
		os.Exit(1)
	}

	// generate port offset (0-999 for more codes)
	portOffset := generatePortOffset()
	port := 8080 + portOffset

	// create code: lastOctet * 1000 + portOffset
	// e.g., IP ending in 105 with offset 47 = 105047
	// This allows codes up to 254999 (254 * 1000 + 999)
	code := lastOctet*1000 + portOffset

	//timer - will be started inside handler
	var totalData uint64

	// start the HTTP server with custom mux (avoid global handler conflicts)
	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	// i need to wait for each client to download then close the server
	var wg sync.WaitGroup
	var hasTransfered sync.Once
	done := make(chan struct{})

	if info.IsDir() {
		// Serve directory as zip
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			wg.Add(1)
			defer wg.Done()

			hasTransfered.Do(func() {
				go func() {
					wg.Wait()
					close(done) // single from here
				}()
			})

			fmt.Println("\nReceiver connected! Starting transfer...")
			startTime := pkg.Start() // Start timer right before transfer
			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.zip\"", filepath.Base(path)))

			cw := &countingWriter{w: w, count: &totalData}
			zipWriter := zip.NewWriter(cw)
			defer zipWriter.Close()

			fileCount := 0
			dirCount := 0

			filepath.Walk(path, func(filePath string, fileInfo os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				relPath, _ := filepath.Rel(path, filePath)
				if relPath == "." {
					return nil // Skip root directory
				}

				// Create directory entry in ZIP (with trailing slash)
				if fileInfo.IsDir() {
					_, err := zipWriter.Create(relPath + "/")
					if err != nil {
						return err
					}
					dirCount++
					// fmt.Printf("    Adding directory: %s\n", relPath)
					return nil
				}

				// Create file entry in ZIP
				zipFile, err := zipWriter.Create(relPath)
				if err != nil {
					return err
				}

				file, err := os.Open(filePath)
				if err != nil {
					return err
				}
				defer file.Close()

				_, err = io.Copy(zipFile, file)
				if err != nil {
					return err
				}

				fileCount++
				// fmt.Printf("    Adding file: %s\n", relPath)
				return nil
			})

			fmt.Printf("\nTransfer complete! (%d files, %d directories)\n", fileCount, dirCount)
			pkg.Ttt(totalData, startTime)
		})
	} else {
		// Serve single file
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			wg.Add(1)
			defer wg.Done()

			hasTransfered.Do(func() {
				go func() {
					wg.Wait()
					close(done) // single from here
				}()
			})
			fmt.Println("\nReceiver connected! Starting transfer...")
			startTime := pkg.Start() // Start timer right before transfer
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(path)))

			file, err := os.Open(path)
			if err != nil {
				http.Error(w, "error opening file", http.StatusInternalServerError)
				return
			}
			defer file.Close()

			cw := &countingWriter{w: w, count: &totalData}
			// if we can increase the buffer that would be great for higher throughtput
			// need to check and what should be the appropriate limit
			buf := make([]byte, 512*1024)
			io.CopyBuffer(cw, file, buf)
			fmt.Println("Transfer complete!")
			pkg.Ttt(totalData, startTime)
		})
	}

	// there is a proble with the channel approac if more than 1 client joins simultaneously it'd crash
	// solution is clo

	// print connection info
	fmt.Println("\nWireGo - P2P File Sharing")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Printf("Code: %06d\n", code)
	fmt.Printf("IP: %s\n", localIP)
	fmt.Printf("Port: %d\n", port)
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println("\nWaiting for the receiver...")
	fmt.Printf("Receiver should run: wirego receive %06d <folder-name>\n", code)
	fmt.Println("\nPress Ctrl+C to cancel")

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			if strings.Contains(err.Error(), "address already in use") || strings.Contains(err.Error(), "Only one usage") {
				fmt.Printf("error: Port %d is already in use. Try again to get a different code.\n", port)
			} else {
				fmt.Println("server error:", err)
			}
			os.Exit(1)
		}
	}()

	<-done
	server.Shutdown(context.Background())
}

func sendViaWebRTC(path string, info os.FileInfo) {
	fmt.Println("\nWireGo - WebRTC File Sharing")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println("Initializing WebRTC connection...")

	// Create WebRTC connection
	webrtc, err := pkg.NewWebRTCConnection()
	if err != nil {
		fmt.Println("Error creating WebRTC connection:", err)
		os.Exit(1)
	}
	defer webrtc.Close()

	// Create offer and wait for ICE gathering
	fmt.Println("Creating offer and gathering ICE candidates...")
	offerSDP, err := webrtc.CreateOffer()
	if err != nil {
		fmt.Println("Error creating offer:", err)
		os.Exit(1)
	}

	// Send offer to signaling server and get code
	fmt.Println("Registering with signaling server...")
	sig := pkg.NewSignalingClient("")
	code, err := sig.CreateOffer(offerSDP)
	if err != nil {
		fmt.Println("Error:", err)
		fmt.Println("\nNote: Signaling server is not running.")
		fmt.Println("For local network transfers, use: wirego send <file> --local")
		os.Exit(1)
	}
	defer sig.Close()

	fmt.Println(strings.Repeat("─", 40))
	fmt.Printf("Remote Code: %s\n", code)
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println("\nWaiting for receiver to connect...")
	fmt.Printf("Receiver should run: wirego receive %s <folder-name>\n", code)
	fmt.Println("\nPress Ctrl+C to cancel")

	// Wait for answer from receiver
	fmt.Println("\nWaiting for receiver's answer...")
	answerSDP, err := sig.WaitForAnswer()
	if err != nil {
		fmt.Println("Error receiving answer:", err)
		os.Exit(1)
	}

	// Set remote answer
	err = webrtc.SetAnswer(answerSDP)
	if err != nil {
		fmt.Println("Error setting answer:", err)
		os.Exit(1)
	}

	// Wait for connection
	fmt.Println("Establishing P2P connection...")
	webrtc.WaitForOpen()
	fmt.Println("\nReceiver connected! Starting transfer...")

	// Use TransferManager for concurrent file transfers
	tm := pkg.NewTransferManager(webrtc.PC)
	if err := tm.SendFiles(path); err != nil {
		fmt.Printf("Transfer error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n✓ Transfer complete!")
}

func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	var fallbackIP string

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			ip := ipnet.IP.To4()
			if ip == nil {
				continue
			}

			// Skip link-local addresses (169.254.x.x)
			if ip[0] == 169 && ip[1] == 254 {
				continue
			}

			// Prefer private network IPs
			// 10.x.x.x, 172.16-31.x.x, 192.168.x.x
			if ip[0] == 10 ||
				(ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) ||
				(ip[0] == 192 && ip[1] == 168) {
				return ip.String(), nil
			}

			// Store as fallback
			if fallbackIP == "" {
				fallbackIP = ip.String()
			}
		}
	}

	if fallbackIP != "" {
		return fallbackIP, nil
	}

	return "", fmt.Errorf("no local IP found")
}

func generatePortOffset() int {
	src := rand.NewSource(time.Now().UnixNano())
	r := rand.New(src)
	return r.Intn(1000) // 0-999 (expanded range)
}

func getLastOctet(ip string) (int, error) {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return 0, fmt.Errorf("invalid IP format")
	}
	octet, err := strconv.Atoi(parts[3])
	if err != nil {
		return 0, err
	}
	return octet, nil
}
