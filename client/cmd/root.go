package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "client",
	Short: "Bethrou client portal",
	Long:  "Bethrou client portal command line interface",
}

func Execute() error {
	return rootCmd.Execute()
}

func AddCommand(cmd *cobra.Command) {
	rootCmd.AddCommand(cmd)
}

func init() {
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	rootCmd.SetVersionTemplate("client\n")

	if _, err := os.Stat("client.yaml"); os.IsNotExist(err) {
		fmt.Println()
	}
}
