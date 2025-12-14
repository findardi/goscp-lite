package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	user    string
	keypath string
	host    string
	port    int
)

var rootCmd = &cobra.Command{
	Use:   "goscp",
	Short: "A lightweight SCP/SFTP CLI tool",
	Long:  "goscp is a lightweight command-line tool for secure file transfers\nusing SFTP protocols, powered by Go with SSH key authentication.",
	Run:   func(cmd *cobra.Command, args []string) {},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Woops, An error while executing goscp '%s'\n", err)
		os.Exit(1)
	}
}
