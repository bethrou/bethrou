package cmd

import (
	"context"
	stdlog "log"
	"os"

	"github.com/henrybarreto/bethrou/client/client"
	"github.com/henrybarreto/bethrou/client/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	configPath string
	keyPath    string
)

func init() {
	connectCmd.Flags().StringVar(&configPath, "config", "./client.yaml", "Path to client config file")
	connectCmd.Flags().StringVar(&keyPath, "key", "", "Path to network.key file (overrides config)")

	rootCmd.AddCommand(connectCmd)
}

var connectCmd = &cobra.Command{
	Use:   "connect",
	Short: "connect to bethrou network",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if cmd.Flags().Changed("config") {
			stdlog.Printf("Using config file: %s", configPath)
		}

		cfg := &config.ClientConfig{}
		if _, err := os.Stat(configPath); err == nil {
			data, err := os.ReadFile(configPath)
			if err != nil {
				stdlog.Fatalf("failed to read config file %s: %v", configPath, err)
			}

			if err := yaml.Unmarshal(data, cfg); err != nil {
				stdlog.Fatalf("failed to parse config file %s: %v", configPath, err)
			}
		}

		if cmd.Flags().Changed("key") {
			cfg.Key = keyPath
		}

		if err := client.Connect(ctx, cfg); err != nil {
			stdlog.Fatalf("Client failed: %v", err)
		}
	},
}
