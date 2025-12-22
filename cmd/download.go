package cmd

import (
	"github.com/findardi/goscp-lite/internal"
	"github.com/spf13/cobra"
)

var downloadCmd = &cobra.Command{
	Use:     "download",
	Aliases: []string{"d"},
	Short:   "Download file from remote server via SFTP",
	Long:    "Download a remote file to local machine using SFTP protocol.\n\nArguments:\n  <remote-path>  Path to file on remote server\n  <local-path>   Destination path on local machine",
	Example: "  goscp download /remote/file.txt ./local/path/ -H example.com -p 123",
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		internal.Download(user, host, port, keypath, args[1], args[0], retry)
	},
}

func init() {
	rootCmd.AddCommand(downloadCmd)
}
