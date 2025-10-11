package client

import (
	"context"
	"fmt"
	stdlog "log"
	"time"

	"github.com/henrybarreto/bethrou/client/config"
	socks "github.com/henrybarreto/bethrou/client/socks"
	discovery "github.com/henrybarreto/bethrou/pkg/discovery"
	host "github.com/henrybarreto/bethrou/pkg/host"
	"github.com/henrybarreto/bethrou/pkg/logging"
	"github.com/henrybarreto/bethrou/pkg/proxy"
)

func Connect(ctx context.Context, cfg *config.ClientConfig) error {
	if err := cfg.Validate(); err != nil {
		logging.Logger.Error("Configuration validation failed", "error", err)

		return fmt.Errorf("invalid configuration: %w", err)
	}

	logging.Setup(cfg.Log)
	stdlog.SetOutput(logging.StdLog())

	logging.Logger.Info("Starting client", "config", cfg.String())

	hst, err := host.NewClient(cfg.Key)
	if err != nil {
		return fmt.Errorf("failed to create host: %w", err)
	}

	defer func() {
		if err := hst.Close(); err != nil {
			logging.Logger.Error("Error closing host", "error", err)
		}
	}()

	logging.Logger.Info("Client host created", "id", hst.ID())

	pol := proxy.NewPool(proxy.PoolStrategy(cfg.Routing.Strategy))

	cli := proxy.NewClient(hst.Host(), pol)

	var nodes []config.NodeConfig

	if cfg.Nodes != nil {
		nodes = append(nodes, cfg.Nodes...)
		logging.Logger.Info("Loaded static nodes from config", "count", len(cfg.Nodes))
	}

	if cfg.Discovery.Enabled {
		dnodes, err := discover(ctx, cfg.Discovery)
		if err != nil {
			return fmt.Errorf("discovery failed: %w", err)
		}

		if len(nodes) == 0 {
			nodes = dnodes

			logging.Logger.Info("No static nodes found; using discovered nodes", "count", len(nodes))
		} else {
			seen := make(map[string]struct{}, len(nodes)+len(dnodes))
			for _, n := range nodes {
				seen[n.ID] = struct{}{}
			}

			added := 0
			for _, n := range dnodes {
				if _, ok := seen[n.ID]; ok {
					continue
				}
				nodes = append(nodes, n)
				seen[n.ID] = struct{}{}
				added++
			}

			logging.Logger.Info("Discovered nodes", "total", len(dnodes), "added", added)
		}
	}

	logging.Logger.Info("Connecting to exit nodes")
	if err := cli.Connect(ctx, nodes); err != nil {
		return fmt.Errorf("failed to connect to exit nodes: %w", err)
	}

	logging.Logger.Info("Connected to exit nodes", "count", pol.Size())

	if cfg.Routing.Health != "" {
		healthDur, err := time.ParseDuration(cfg.Routing.Health)
		if err == nil && healthDur > 0 {

			timeoutDur := 0 * time.Second
			if cfg.Routing.Timeout != "" {
				if td, err := time.ParseDuration(cfg.Routing.Timeout); err == nil {
					timeoutDur = td
				}
			}

			go func() {
				logging.Logger.Info("Starting health checks", "interval", healthDur, "timeout", timeoutDur)

				ticker := time.NewTicker(healthDur)
				defer ticker.Stop()
				for {
					select {
					case <-ticker.C:
						conns := pol.All()
						for _, c := range conns {
							logging.Logger.Debug("Pinging node", "peer", c.PeerID, "addr", c.Addr)

							ctxProbe, cancel := context.WithTimeout(context.Background(), timeoutDur+5*time.Second)
							lat, err := cli.Ping(ctxProbe, c)
							cancel()
							if err != nil {
								logging.Logger.Warn("Health check failed", "peer", c.PeerID, "error", err)

								pol.UpdateLatency(c.PeerID, time.Hour)

								continue
							}

							logging.Logger.Debug("Node healthy", "peer", c.PeerID, "latency", lat)

							pol.UpdateLatency(c.PeerID, lat)
						}
					case <-ctx.Done():
						return
					}
				}
			}()
		}
	}

	srv, err := socks.NewServer(ctx, cli, cfg.Server)
	if err != nil {
		return fmt.Errorf("failed to create SOCKS server: %w", err)
	}

	logging.Logger.Info("SOCKS5 server running", "addr", cfg.Server.ListenAddr)

	if err := srv.ListenAndServe(); err != nil {
		return fmt.Errorf("SOCKS5 server error: %w", err)
	}

	return nil
}

// disocver uses the discovery service to find nodes.
func discover(ctx context.Context, cfg *config.DiscoveryConfig) ([]config.NodeConfig, error) {
	if cfg.Topic == "" {
		return nil, fmt.Errorf("discovery topic is required")
	}

	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		timeout = 10 * time.Second
	}

	svc, err := discovery.NewService(discovery.Config{
		Address: cfg.Address,
		Topic:   cfg.Topic,
		Timeout: timeout,
		User:    cfg.User,
		Pass:    cfg.Pass,
	}, nil)
	if err != nil {
		return nil, err
	}

	defer func() { _ = svc.Close() }()

	logging.Logger.Info("Running discovery", "topic", cfg.Topic, "timeout", timeout)

	nodes, err := svc.Discover(ctx)
	if err != nil {
		return nil, err
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("discovery returned no nodes")
	}

	return nodes, nil
}
