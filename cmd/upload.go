package cmd

import (
	"github.com/findardi/goscp-lite/internal"
	"github.com/spf13/cobra"
)

var uploadCmd = &cobra.Command{
	Use:     "upload",
	Aliases: []string{"u"},
	Short:   "Upload file to remote server via SFTP",
	Long:    "Upload a local file to a remote server using SFTP protocol.\n\nArguments:\n  <local-path>   Path to local file\n  <remote-path>  Destination path on remote server",
	Example: "  goscp upload .file.txt /remote/path/ -H example.com -p 123",
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		internal.Upload(user, host, port, keypath, args[0], args[1], retry)
	},
}

func init() {
	rootCmd.AddCommand(uploadCmd)
}
