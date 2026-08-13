package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/bethrou/bethrou/internal/client"
	"github.com/bethrou/bethrou/internal/config"
	"github.com/bethrou/bethrou/internal/tui"
	pkgconfig "github.com/bethrou/bethrou/pkg/config"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	configPath      string
	keyPath         string
	identityKeyPath string
	clientAPIURL    string
	targetNodes     []string
	allNodes        bool
)

// registerConnectFlags registers the connect subcommand's flags. Called
// from Execute (client/cmd/root.go), after LoadDotEnv, so the EnvOr/
// EnvBoolOr/etc. calls below (which read os.Getenv to compute flag
// defaults) see any values loaded from .env.
func registerConnectFlags() {
	connectCmd.Flags().StringVar(&configPath, "config", pkgconfig.EnvOr("BETHROU_CLIENT_CONFIG", "./client.yaml"), "Path to client config file (env: BETHROU_CLIENT_CONFIG)")
	connectCmd.Flags().StringVar(&keyPath, "key", pkgconfig.EnvOr("BETHROU_CLIENT_KEY", ""), "Path to network.key file, overrides config (env: BETHROU_CLIENT_KEY)")
	connectCmd.Flags().StringVar(&identityKeyPath, "identity-key", pkgconfig.EnvOr("BETHROU_CLIENT_IDENTITY_KEY", ""), "Path to the persistent client identity key, overrides config (env: BETHROU_CLIENT_IDENTITY_KEY)")
	connectCmd.Flags().StringVar(&clientAPIURL, "api-url", pkgconfig.EnvOr("BETHROU_CLIENT_API_URL", ""), "Control-plane API base URL, e.g. https://saas.example.com, overrides config (env: BETHROU_CLIENT_API_URL)")
	connectCmd.Flags().StringSliceVar(&targetNodes, "target-node", pkgconfig.EnvStringSliceOr("BETHROU_CLIENT_TARGET_NODES", nil), "Pin a specific exit node peer ID to connect to; repeat for multiple, overrides config (env: BETHROU_CLIENT_TARGET_NODES, comma-separated). Optional — if unset and running in a terminal, an interactive node picker opens instead.")
	connectCmd.Flags().BoolVar(&allNodes, "all-nodes", pkgconfig.EnvBoolOr("BETHROU_CLIENT_ALL_NODES", false), "In a non-interactive context, explicitly opt in to connecting through every exit node on the account when no --target-node/config nodes are set (env: BETHROU_CLIENT_ALL_NODES). Required there instead of defaulting silently.")

	rootCmd.AddCommand(connectCmd)
}

var connectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Connect to enrolled exit nodes and start the local SOCKS5 proxy",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()

		cfg := &config.ClientConfig{}
		if _, err := os.Stat(configPath); err == nil {
			data, err := os.ReadFile(configPath)
			if err != nil {
				return fmt.Errorf("failed to read config file %s: %w", configPath, err)
			}

			if err := yaml.Unmarshal(data, cfg); err != nil {
				return fmt.Errorf("failed to parse config file %s: %w", configPath, err)
			}
		}

		// Changed() alone would miss a value that came from an env var
		// (via the flag's env-derived default, see init() below) rather
		// than an explicit CLI flag, so also check the env var directly —
		// giving precedence CLI flag > env var/.env > config file.
		if cmd.Flags().Changed("key") || os.Getenv("BETHROU_CLIENT_KEY") != "" {
			cfg.Key = keyPath
		}
		if cmd.Flags().Changed("identity-key") || os.Getenv("BETHROU_CLIENT_IDENTITY_KEY") != "" {
			cfg.IdentityKey = identityKeyPath
		}
		if cfg.IdentityKey == "" {
			cfg.IdentityKey = "client.key"
		}
		if cmd.Flags().Changed("api-url") || os.Getenv("BETHROU_CLIENT_API_URL") != "" {
			cfg.APIURL = clientAPIURL
		}
		if cmd.Flags().Changed("target-node") || os.Getenv("BETHROU_CLIENT_TARGET_NODES") != "" {
			cfg.TargetNodes = targetNodes
		}

		// No node pinned locally, either via --target-node or config: in a
		// terminal, let the operator pick interactively instead of
		// silently connecting through every node on the account. In a
		// non-interactive context (script, systemd, container with no
		// TTY), require an explicit --all-nodes opt-in for that same
		// "use every node" behavior instead of defaulting to it silently.
		interactive := isatty.IsTerminal(os.Stdout.Fd()) && isatty.IsTerminal(os.Stdin.Fd())
		if len(cfg.TargetNodes) == 0 {
			if interactive {
				return tui.Run(ctx, cfg)
			}
			if !allNodes {
				return fmt.Errorf("no target nodes configured; set --target-node (repeatable) or config target_nodes, or pass --all-nodes to explicitly connect through every exit node on the account")
			}
		}

		return client.Connect(ctx, cfg)
	},
}
