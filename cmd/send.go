package cmd

import (
	"archive/zip"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var sendCmd = &cobra.Command{
	Use:   "send [file or directory]",
	Short: "Send a file or directory",
	Long:  "Send a file or directory to another peer. Use '.' for current directory.",
	Args:  cobra.ExactArgs(1),
	Run:   runSend,
}

func init() {
	root.AddCommand(sendCmd)
}

func runSend(cmd *cobra.Command, args []string) {
	path := args[0]

	// checks path exists
	info, err := os.Stat(path)
	if err != nil {
		fmt.Printf("error: %s does not exist\n", path)
		os.Exit(1)
	}

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

	// generate port offset (00-99)
	portOffset := generatePortOffset()
	port := 8080 + portOffset

	// create 5-digit code: lastOctet * 100 + portOffset
	// e.g., IP ending in 105 with offset 47 = 10547
	code := lastOctet*100 + portOffset

	// start the HTTP server
	server := &http.Server{
		Addr: fmt.Sprintf(":%d", port),
	}

	if info.IsDir() {
		// Serve directory as zip
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Println("\nReceiver connected! Starting transfer...")
			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.zip\"", filepath.Base(path)))

			zipWriter := zip.NewWriter(w)
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
					// fmt.Printf("   📁 Adding directory: %s\n", relPath)
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
				// fmt.Printf("   📄 Adding file: %s\n", relPath)
				return nil
			})

			fmt.Printf("\nTransfer complete! (%d files, %d directories)\n", fileCount, dirCount)
		})
	} else {
		// Serve single file
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Println("\nReceiver connected! Starting transfer...")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(path)))

			file, err := os.Open(path)
			if err != nil {
				http.Error(w, "error opening file", http.StatusInternalServerError)
				return
			}
			defer file.Close()

			io.Copy(w, file)
			fmt.Println("Transfer complete!")
		})
	}

	// print connection info
	fmt.Println("\nWireGo - P2P File Sharing")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Printf("Code: %05d\n", code)
	fmt.Printf("IP: %s\n", localIP)
	fmt.Printf("Port: %d\n", port)
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println("\nWaiting for the receiver...")
	fmt.Printf("Receiver should run: wirego receive %05d <folder-name>\n", code)
	fmt.Println("\nPress Ctrl+C to cancel")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Println("server error:", err)
	}
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
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(100) // 00-99
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
