package cmd

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
)

// this is the root command
var root = &cobra.Command{
	Use:   "wirego-cli",
	Short: "p2p fs cli application",
	Long:  "a cli application to share p2p over network",
}

func Run() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	go func() {
		for s := range sig {
			fmt.Println(s.String())
			os.Exit(0)
		}
	}()

	err := root.Execute()
	if err != nil {
		os.Exit(0)
	}
}
