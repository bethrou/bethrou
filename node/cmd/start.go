package cmd

import (
	"context"
	stdlog "log"

	"github.com/henrybarreto/bethrou/node/server"
	pkgconfig "github.com/henrybarreto/bethrou/pkg/config"
	"github.com/spf13/cobra"
)

var (
	listen          string
	relayMode       bool
	connectRelay    string
	keyPath         string
	discoverEnable  bool
	discoverAddress string
	discoverUser    string
	discoverPass    string
	discoverTopic   string
)

func init() {
	startCmd.Flags().StringVar(&listen, "listen", "/ip4/0.0.0.0/tcp/4000", "Listen address")
	startCmd.Flags().BoolVar(&relayMode, "relay-mode", false, "Enable relay service on this node")
	startCmd.Flags().StringVar(&connectRelay, "connect-relay", "", "Connect to an external relay multiaddr (for NAT traversal)")
	startCmd.Flags().StringVar(&keyPath, "key", "", "Path to network.key file (overrides default lookup)")
	startCmd.Flags().BoolVar(&discoverEnable, "discover", false, "Enable discover subscription (pub/sub)")
	startCmd.Flags().StringVar(&discoverAddress, "discover-address", "redis://localhost:6379", "Server URL for discover pub/sub")
	startCmd.Flags().StringVar(&discoverUser, "discover-user", "", "Optional redis username for discover")
	startCmd.Flags().StringVar(&discoverPass, "discover-pass", "", "Optional redis password for discover")
	startCmd.Flags().StringVar(&discoverTopic, "discover-topic", "", "Topic to subscribe for discover messages (defaults to node ID)")

	rootCmd.AddCommand(startCmd)
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start node",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		cfg := &server.Config{
			Listen:       listen,
			RelayMode:    relayMode,
			ConnectRelay: connectRelay,
			Discovery: pkgconfig.DiscoveryConfig{
				Enabled: discoverEnable,
				Address: discoverAddress,
				User:    discoverUser,
				Pass:    discoverPass,
				Topic:   discoverTopic,
			},
		}

		if cmd.Flags().Changed("key") {
			cfg.Key = keyPath
		}

		if err := server.Start(ctx, cfg); err != nil {
			stdlog.Fatalf("node failed: %v", err)
		}
	},
}
