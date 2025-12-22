package cmd

import (
	"github.com/findardi/goscp-lite/internal"
	"github.com/spf13/cobra"
)

var testCmd = &cobra.Command{
	Use:     "test",
	Aliases: []string{"t"},
	Short:   "Test connection to server",
	Long:    "Test SSH and SFTP connectivity to a remote server.\nValidates authentication and displays connection status.",
	Example: "goscp test -H example.com -p 123 -u admin",
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		internal.Test(user, host, keypath, port)
	},
}

func init() {
	rootCmd.AddCommand(testCmd)
}
