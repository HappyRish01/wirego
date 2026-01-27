package cmd

import (
	"fmt"
	"os"

	"github.com/HappyRish01/wirego/pkg"
	"github.com/spf13/cobra"
)

var findMyIp = &cobra.Command{
	Use:   "ip",
	Short: "To find the public IP of yours",
	Long:  `This command finds your public IP address using Google STUN servers and the request is made via WebRTC`,
	Run: func(cmd *cobra.Command, args []string) {
		ip, err := pkg.FindPublicIP()

		if err != nil {
			fmt.Printf("error failed to find public IP")
			os.Exit(1)
		}

		if len(ip) == 0 {
			fmt.Println("sorry could n't find the IP")
			return
		}

		fmt.Println("your public IP address(es):")
		for _, addr := range ip {
			fmt.Printf("  %s\n", addr)
		}
	},
}

func init() {
	root.AddCommand(findMyIp)
}
