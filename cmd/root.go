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
	Short: "goscp is a cli tool for scp operation.",
	Long:  "goscp is a cli tool for scp operation powered by go.",
	Run:   func(cmd *cobra.Command, args []string) {},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Woops, An error while executing goscp '%s'\n", err)
		os.Exit(1)
	}
}
