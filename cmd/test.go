package cmd

import (
	"github.com/findardi/goscp-lite/internal"
	"github.com/spf13/cobra"
)

var testCmd = &cobra.Command{
	Use:     "test",
	Aliases: []string{"t"},
	Short:   "Test connection to server",
	Long:    "Test SSH and SFTP connection to remote server",
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		internal.Test(user, host, port, keypath)
	},
}

func init() {
	rootCmd.AddCommand(testCmd)

	testCmd.Flags().StringVarP(&host, "host", "H", "", "host server")
	testCmd.Flags().IntVarP(&port, "port", "p", 22, "port server")
	testCmd.Flags().StringVarP(&user, "user", "u", "", "SSH username (default:root)")
	testCmd.Flags().StringVarP(&keypath, "key", "k", "", "Path to private key (default:auto-detect)")

	testCmd.MarkFlagRequired("host")
}
