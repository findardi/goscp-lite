package cmd

import (
	"github.com/spf13/cobra"
)

var downloadCmd = &cobra.Command{
	Use:     "download",
	Aliases: []string{"d"},
	Short:   "download file",
	Long:    "dowload file from the server",
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {

	},
}

func init() {
	rootCmd.AddCommand(downloadCmd)

	downloadCmd.Flags().StringVarP(&host, "host", "H", "", "host server")
	downloadCmd.Flags().IntVarP(&port, "port", "p", 22, "port server")

	downloadCmd.MarkFlagRequired("host")
}
