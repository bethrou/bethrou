package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "node",
	Short: "Bethrou node",
	Long:  "Bethrou node command line interface",
}

func Execute() error {
	return rootCmd.Execute()
}

func AddCommand(cmd *cobra.Command) {
	rootCmd.AddCommand(cmd)
}

func init() {
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	rootCmd.SetVersionTemplate("node\n")
}
