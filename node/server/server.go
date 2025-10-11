package server

import (
	"context"
	"fmt"
	stdlog "log"
	"os"

	"github.com/henrybarreto/bethrou/node/identity"
	pkgconfig "github.com/henrybarreto/bethrou/pkg/config"
	"github.com/henrybarreto/bethrou/pkg/discovery"
	"github.com/henrybarreto/bethrou/pkg/host"
	"github.com/henrybarreto/bethrou/pkg/logging"
	"github.com/henrybarreto/bethrou/pkg/proxy"
)

// Config is deprecated, use config.Config instead.
type Config struct {
	Key          string
	Listen       string
	RelayMode    bool
	ConnectRelay string
	Discovery    pkgconfig.DiscoveryConfig
}

func (c *Config) String() string {
	return fmt.Sprintf("{Address: %s, RelayMode: %t, ConnectRelay: %s, DiscoverEnabled: %t, DiscoverAddress: %s, DiscoverUser: %s, DiscoverTopic: %s, Key: %s}",
		c.Listen, c.RelayMode, c.ConnectRelay, c.Discovery.Enabled, c.Discovery.Address, c.Discovery.User, c.Discovery.Topic, c.Key)
}

func Start(ctx context.Context, cfg *Config) error {
	logging.Setup(nil)

	stdlog.SetOutput(logging.StdLog())
	logging.Logger.Info("Starting Bethrou node", "config", cfg.String())

	keyPath := cfg.Key
	if keyPath == "" {
		logging.Logger.Info("network key path not set; looking for network.key next to the binary")

		tried := []string{"network.key", "../network.key", "../../network.key"}
		found := false
		for _, p := range tried {
			if _, err := os.Stat(p); err == nil {
				found = true
				logging.Logger.Info("found network key", "path", p)
				keyPath = p
				break
			}
		}

		if !found {
			return fmt.Errorf("network key not found; set --key or place network.key next to the binary")
		}
	}

	idMgr := identity.NewManager("node.key")
	priv, err := idMgr.LoadOrGenerate()
	if err != nil {
		return fmt.Errorf("failed to load identity: %w", err)
	}

	h, err := host.NewNode(host.NodeConfig{
		ListenAddr:   cfg.Listen,
		PrivateKey:   priv,
		RelayMode:    cfg.RelayMode,
		ConnectRelay: cfg.ConnectRelay,
		Key:          keyPath,
	})
	if err != nil {
		return fmt.Errorf("failed to create node: %w", err)
	}
	defer func() {
		if err := h.Close(); err != nil {
			logging.Logger.Error("Error closing node", "error", err)
		}
	}()

	srv := proxy.NewServer(h.Host())

	logging.Logger.Info("Exit node ready, listening for proxy streams")
	logging.Logger.Info("Full exit node addresses")
	for _, addr := range h.Host().Addrs() {
		logging.Logger.Info("address", "addr", fmt.Sprintf("%s/p2p/%s", addr, h.Host().ID()))
	}

	if cfg.Discovery.Enabled {
		dsv, err := discovery.NewService(discovery.Config{
			Address: cfg.Discovery.Address,
			User:    cfg.Discovery.User,
			Pass:    cfg.Discovery.Pass,
			Topic:   cfg.Discovery.Topic,
		}, h.Host())
		if err != nil {
			return fmt.Errorf("failed to create discovery service: %w", err)
		}

		defer func() {
			if err := dsv.Close(); err != nil {
				logging.Logger.Error("Error closing discovery service", "error", err)
			}
		}()

		errCh := make(chan error, 1)
		go func() {
			if err := dsv.Start(ctx); err != nil && err != context.Canceled {
				errCh <- fmt.Errorf("discovery service error: %w", err)
			}
		}()

		go func() {
			select {
			case err := <-errCh:
				logging.Logger.Error("Discovery error", "error", err)
			case <-ctx.Done():
				return
			}
		}()
	}

	srv.Listen(ctx)

	logging.Logger.Info("Shutting down node")
	return nil
}
