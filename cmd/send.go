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

	// gen 3-digit code
	code := generateCode()

	// get the available port
	port := 8080 + (code % 100)

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
	// if info.IsDir() {
	// 	fmt.Printf("Sharing directory: %s\n", path)
	// } else {
	// 	fmt.Printf("Sharing file: %s\n", path)
	// }
	// fmt.Println(strings.Repeat("─", 40))
	fmt.Printf("Code: %03d\n", code)
	fmt.Printf("IP: %s\n", localIP)
	fmt.Printf("Port: %d\n", port)
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println("\nWaiting for the receiver...")
	fmt.Println("Receiver should run: wirego receive", code, "<save-directory>")
	fmt.Println("\nPress Ctrl+C to cancel")

	// store connection info for receiver
	// in this simple version, code encodes: IP's last octet * 1000 + port offset
	// the receiver will need to be on same network and scan for the port

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Println("server error:", err)
	}
}

func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}

	return "", fmt.Errorf("no local IP found")
}

func generateCode() int {
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(900) + 100 // 100-999
}
