package cmd

import (
	"github.com/findardi/goscp-lite/internal"
	"github.com/spf13/cobra"
)

var uploadCmd = &cobra.Command{
	Use:     "upload",
	Aliases: []string{"u"},
	Short:   "upload file",
	Long:    "upload file to the server",
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		internal.Upload(user, host, port, keypath, args[0], args[1])
	},
}

func init() {
	rootCmd.AddCommand(uploadCmd)

	uploadCmd.Flags().StringVarP(&host, "host", "H", "", "host server")
	uploadCmd.Flags().IntVarP(&port, "port", "p", 22, "port server")
	uploadCmd.Flags().StringVarP(&user, "user", "u", "", "SSH username (default:root)")
	uploadCmd.Flags().StringVarP(&keypath, "key", "k", "", "Path to private key (default:auto-detect)")

	uploadCmd.MarkFlagRequired("host")
}
