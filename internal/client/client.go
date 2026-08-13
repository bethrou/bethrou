package client

import (
	"context"
	"fmt"
	stdlog "log"
	"sync"
	"time"

	"github.com/bethrou/bethrou/internal/config"
	socks "github.com/bethrou/bethrou/internal/socks"
	pkgconfig "github.com/bethrou/bethrou/pkg/config"
	"github.com/bethrou/bethrou/pkg/control"
	host "github.com/bethrou/bethrou/pkg/host"
	"github.com/bethrou/bethrou/pkg/identity"
	"github.com/bethrou/bethrou/pkg/logging"
	"github.com/bethrou/bethrou/pkg/proxy"
)

// prepareConfig fills in defaults a caller might have left unset and
// validates the result. Called by every backend entry point (Connect,
// NewSession) so both the CLI's one-shot connect and the TUI's session
// enforce the same invariants — neither client/cmd nor client/tui should
// need their own copy of this logic; see client/tui/tui.go's Run for the
// one case that still validates early (for a nicer error before it opens
// a log file), which is a redundant-but-harmless fail-fast, not a second
// source of truth for what "valid" means.
func prepareConfig(cfg *config.ClientConfig) error {
	if cfg.IdentityKey == "" {
		cfg.IdentityKey = "client.key"
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	return nil
}

func Connect(ctx context.Context, cfg *config.ClientConfig) error {
	logging.Setup(cfg.Log)

	if err := prepareConfig(cfg); err != nil {
		logging.Error("Configuration validation failed", "error", err)
		return err
	}

	stdlog.SetOutput(logging.StdLog())

	logging.Info("Starting client", "config", cfg.String())

	if cfg.APIURL == "" {
		return fmt.Errorf("api_url / --api-url is required")
	}

	priv, err := identity.NewManager(cfg.IdentityKey).LoadOrGenerate()
	if err != nil {
		return fmt.Errorf("failed to load identity: %w", err)
	}

	cc, err := control.New(cfg.APIURL, priv)
	if err != nil {
		return fmt.Errorf("failed to build control-plane client: %w", err)
	}

	me, err := cc.Me(ctx)
	if err != nil {
		return fmt.Errorf("client is not enrolled with the control plane (run 'client enroll --api-url %s --token <token>' first): %w", cfg.APIURL, err)
	}
	if me.Role != "client" {
		return fmt.Errorf("this identity is enrolled as role %q, expected \"client\"", me.Role)
	}

	hst, err := host.NewClient(cfg.Key, priv)
	if err != nil {
		return fmt.Errorf("failed to create host: %w", err)
	}

	defer func() {
		if err := hst.Close(); err != nil {
			logging.Error("Error closing host", "error", err)
		}
	}()

	logging.Info("Client host created", "id", hst.ID())

	pol := proxy.NewPool(proxy.PoolStrategy(cfg.Routing.Strategy))

	cli := proxy.NewClient(hst.Host(), pol)

	targets := cfg.TargetNodes
	if len(targets) == 0 {
		// No locally configured nodes: use whatever exit nodes the control
		// plane reports for this account. Re-fetched on every start, so a
		// node added/removed from the dashboard takes effect on next
		// (re)connect with no client-side config change.
		targets = make([]string, 0, len(me.Nodes))
		for _, n := range me.Nodes {
			targets = append(targets, n.ID)
		}
		if len(targets) == 0 {
			return fmt.Errorf("no exit nodes enrolled on this account yet — enroll a node first, or set target_nodes/--target-node to pin specific ones")
		}
	}

	logging.Info("Resolving and connecting to target exit nodes", "count", len(targets))
	if err := connectTargets(ctx, cc, cli, targets); err != nil {
		return err
	}

	logging.Info("Connected to exit nodes", "count", pol.Size())

	go heartbeatLoop(ctx, cc, 30*time.Second)
	go healthCheckLoop(ctx, cc, cli, pol, cfg.Routing)

	srv, err := socks.NewServer(ctx, cli, cfg.Server, requestTimeout(cfg.Routing))
	if err != nil {
		return fmt.Errorf("failed to create SOCKS server: %w", err)
	}

	logging.Info("SOCKS5 server running", "addr", cfg.Server.ListenAddr)

	if err := srv.ListenAndServe(); err != nil {
		return fmt.Errorf("SOCKS5 server error: %w", err)
	}

	return nil
}

func requestTimeout(routing *config.RoutingConfig) time.Duration {
	if routing.Timeout == "" {
		return 0
	}
	td, err := time.ParseDuration(routing.Timeout)
	if err != nil {
		return 0
	}
	return td
}

// healthCheckLoop periodically pings every pooled connection, dropping and
// reconnecting (in the background, via connectTargets) any that fail to
// respond. A no-op if routing.Health is unset or invalid.
func healthCheckLoop(ctx context.Context, cc *control.Client, cli *proxy.Client, pol *proxy.Pool, routing *config.RoutingConfig) {
	healthDur, err := time.ParseDuration(routing.Health)
	if err != nil || healthDur <= 0 {
		return
	}
	timeoutDur := requestTimeout(routing)

	logging.Info("Starting health checks", "interval", healthDur, "timeout", timeoutDur)

	// Tracks in-flight background reconnect attempts spawned below, so a
	// caller cancelling ctx (e.g. Session.Stop) can be sure none are still
	// touching cli/pol after this function returns, instead of them
	// outliving the session and racing the next Start's fresh cli/pol.
	var reconnectWG sync.WaitGroup
	defer reconnectWG.Wait()

	ticker := time.NewTicker(healthDur)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			conns := pol.All()
			for _, c := range conns {
				logging.Debug("Pinging node", "peer", c.PeerID, "addr", c.Addr)

				ctxProbe, cancel := context.WithTimeout(context.Background(), timeoutDur+5*time.Second)
				lat, err := cli.Ping(ctxProbe, c)
				cancel()
				if err != nil {
					logging.Warn("Health check failed; dropping and reconnecting", "peer", c.PeerID, "error", err)

					pol.Remove(c.PeerID)

					// Re-resolve and reconnect in the background (fresh
					// connect-info picks up any relay change) rather than
					// blocking the rest of this health-check pass.
					target := c.PeerID.String()
					reconnectWG.Add(1)
					go func() {
						defer reconnectWG.Done()
						if err := connectTargets(ctx, cc, cli, []string{target}); err != nil {
							logging.Warn("Reconnect failed", "node", target, "error", err)
						} else {
							logging.Info("Reconnected to node", "node", target)
						}
					}()

					continue
				}

				logging.Debug("Node healthy", "peer", c.PeerID, "latency", lat)

				pol.UpdateLatency(c.PeerID, lat)
			}
		case <-ctx.Done():
			return
		}
	}
}

// connectTargets resolves each target node's connect-info from the control
// plane and connects to it, attempting a direct connection first and
// falling back to the control-plane's authenticated relay if direct fails.
func connectTargets(ctx context.Context, cc *control.Client, cli *proxy.Client, targets []string) error {
	var lastErr error
	connected := 0

	for _, target := range targets {
		info, err := cc.ConnectInfo(ctx, target)
		if err != nil {
			logging.Warn("Failed to resolve connect-info", "node", target, "error", err)
			lastErr = err
			continue
		}

		// Direct attempt first.
		direct := pkgconfig.NodeConfig{ID: info.ID, Addrs: info.Addrs}
		if err := cli.Connect(ctx, []pkgconfig.NodeConfig{direct}); err == nil {
			connected++
			continue
		} else {
			logging.Warn("Direct connection failed; falling back to relay", "node", target, "error", err)
		}

		// Relay fallback: try each known relay in turn via the control
		// plane's authenticated relay(s).
		if len(info.RelayAddrs) == 0 {
			lastErr = fmt.Errorf("no relay available for node %s and direct connection failed", target)
			continue
		}

		relayed := false
		for _, relayAddr := range info.RelayAddrs {
			viaRelay := pkgconfig.NodeConfig{ID: info.ID, Relay: relayAddr}
			if err := cli.Connect(ctx, []pkgconfig.NodeConfig{viaRelay}); err != nil {
				logging.Warn("Relay connection failed", "node", target, "relay", relayAddr, "error", err)
				lastErr = err
				continue
			}
			relayed = true
			break
		}
		if !relayed {
			continue
		}

		connected++
	}

	if connected == 0 {
		if lastErr != nil {
			return fmt.Errorf("failed to connect to any target node: %w", lastErr)
		}
		return fmt.Errorf("failed to connect to any target node")
	}

	return nil
}

func heartbeatLoop(ctx context.Context, cc *control.Client, interval time.Duration) {
	send := func() {
		if err := cc.Heartbeat(ctx, nil); err != nil {
			logging.Warn("heartbeat failed", "error", err)
			return
		}
		logging.Trace("heartbeat sent")
	}

	send()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			send()
		}
	}
}
