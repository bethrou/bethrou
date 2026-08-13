package cmd

import (
	"fmt"
	"os"

	pkgconfig "github.com/bethrou/bethrou/pkg/config"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "client",
	Short: "Bethrou client portal",
	Long:  "Bethrou client portal command line interface",
	// main.go prints the returned error itself; don't let cobra print it
	// (and its usage dump, which is noise for runtime errors) a second time.
	SilenceErrors: true,
	SilenceUsage:  true,
}

// Execute is the single entry point client/main.go calls. It loads .env
// (if present) before any flag is registered, so registerConnectFlags/
// registerEnrollFlags's env-derived flag defaults (below) see it, then
// runs the command tree.
func Execute() error {
	if err := pkgconfig.LoadDotEnv(".env"); err != nil {
		return err
	}

	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	rootCmd.SetVersionTemplate("client\n")

	if _, err := os.Stat("client.yaml"); os.IsNotExist(err) {
		fmt.Println()
	}

	registerConnectFlags()
	registerEnrollFlags()

	return rootCmd.Execute()
}

func AddCommand(cmd *cobra.Command) {
	rootCmd.AddCommand(cmd)
}
