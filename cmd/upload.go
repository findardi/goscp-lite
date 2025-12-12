package cmd

import (
	"github.com/spf13/cobra"
)

var uploadCmd = &cobra.Command{
	Use:     "upload",
	Aliases: []string{"u"},
	Short:   "upload file",
	Long:    "upload file to the server",
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
	},
}

func init() {
	rootCmd.AddCommand(uploadCmd)

	uploadCmd.Flags().StringVarP(&host, "host", "H", "", "host server")
	uploadCmd.Flags().IntVarP(&port, "port", "p", 22, "port server")

	uploadCmd.MarkFlagRequired("host")
}
