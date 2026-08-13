package client

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bethrou/bethrou/internal/config"
	socks "github.com/bethrou/bethrou/internal/socks"
	"github.com/bethrou/bethrou/pkg/control"
	host "github.com/bethrou/bethrou/pkg/host"
	"github.com/bethrou/bethrou/pkg/identity"
	"github.com/bethrou/bethrou/pkg/logging"
	"github.com/bethrou/bethrou/pkg/proxy"
	"github.com/libp2p/go-libp2p/core/crypto"
)

var (
	ErrAlreadyRunning = errors.New("session already running")
	ErrNotRunning     = errors.New("session not running")
)

// Session is a start/stop-able client, for interactive use (see client/tui)
// where the set of target nodes can change between runs without the
// process restarting. Unlike Connect, which runs once and blocks until the
// SOCKS5 server exits, a Session can be Started and Stopped repeatedly with
// a different node selection each time.
type Session struct {
	cfg  *config.ClientConfig
	priv crypto.PrivKey
	cc   *control.Client

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	hst     *host.Client
	pol     *proxy.Pool
	done    chan struct{}
	stopSrv func() error
	// loops tracks the heartbeat/health-check background goroutines (which
	// themselves track their own reconnect sub-goroutines) so Stop can join
	// them before closing hst, instead of leaving them to race the next
	// Start's freshly-created host/cli/pool.
	loops sync.WaitGroup
}

// NewSession loads (or generates) the persistent identity at
// cfg.IdentityKey, confirms it's enrolled as a client, and returns a
// not-yet-running Session. Call Nodes to discover exit nodes and Start to
// begin routing traffic through a chosen subset.
func NewSession(ctx context.Context, cfg *config.ClientConfig) (*Session, error) {
	if err := prepareConfig(cfg); err != nil {
		return nil, err
	}

	priv, err := identity.NewManager(cfg.IdentityKey).LoadOrGenerate()
	if err != nil {
		return nil, fmt.Errorf("failed to load identity: %w", err)
	}

	cc, err := control.New(cfg.APIURL, priv)
	if err != nil {
		return nil, fmt.Errorf("failed to build control-plane client: %w", err)
	}

	me, err := cc.Me(ctx)
	if err != nil {
		return nil, fmt.Errorf("client is not enrolled with the control plane (run 'client enroll --api-url %s --token <token>' first): %w", cfg.APIURL, err)
	}
	if me.Role != "client" {
		return nil, fmt.Errorf("this identity is enrolled as role %q, expected \"client\"", me.Role)
	}

	return &Session{cfg: cfg, priv: priv, cc: cc}, nil
}

// Nodes fetches every exit node currently available on this account, fresh
// from the control plane.
func (s *Session) Nodes(ctx context.Context) ([]control.NodeSummary, error) {
	me, err := s.cc.Me(ctx)
	if err != nil {
		return nil, err
	}
	return me.Nodes, nil
}

// Running reports whether the session is currently connected and serving.
func (s *Session) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// PoolSize returns how many exit nodes are currently connected, or 0 if
// not running.
func (s *Session) PoolSize() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return 0
	}
	return s.pol.Size()
}

// Start connects to targetIDs (direct-then-relay, same as Connect) and
// begins serving the local SOCKS5 proxy, plus heartbeat/health-check
// background loops, all tied to a context derived from ctx. Returns
// ErrAlreadyRunning if already started — call Stop first to change the
// node selection.
func (s *Session) Start(ctx context.Context, targetIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return ErrAlreadyRunning
	}
	if len(targetIDs) == 0 {
		return errors.New("no nodes selected")
	}

	hst, err := host.NewClient(s.cfg.Key, s.priv)
	if err != nil {
		return fmt.Errorf("failed to create host: %w", err)
	}

	pol := proxy.NewPool(proxy.PoolStrategy(s.cfg.Routing.Strategy))
	cli := proxy.NewClient(hst.Host(), pol)

	runCtx, cancel := context.WithCancel(ctx)

	if err := connectTargets(runCtx, s.cc, cli, targetIDs); err != nil {
		cancel()
		_ = hst.Close()
		return err
	}

	s.loops.Add(2)
	go func() {
		defer s.loops.Done()
		heartbeatLoop(runCtx, s.cc, 30*time.Second)
	}()
	go func() {
		defer s.loops.Done()
		healthCheckLoop(runCtx, s.cc, cli, pol, s.cfg.Routing)
	}()

	srv, err := socks.NewServer(runCtx, cli, s.cfg.Server, requestTimeout(s.cfg.Routing))
	if err != nil {
		cancel()
		_ = hst.Close()
		return fmt.Errorf("failed to create SOCKS server: %w", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := srv.ListenAndServe(); err != nil {
			logging.Error("SOCKS5 server error", "error", err)
		}
	}()

	s.cancel = cancel
	s.hst = hst
	s.pol = pol
	s.done = done
	s.running = true
	s.stopSrv = srv.Shutdown

	return nil
}

// Stop tears down the running session: stops accepting new SOCKS5
// connections, cancels the background loops, and closes the libp2p host.
// Safe to call when not running (returns ErrNotRunning).
func (s *Session) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return ErrNotRunning
	}

	s.cancel()
	if err := s.stopSrv(); err != nil {
		logging.Warn("error shutting down SOCKS5 server", "error", err)
	}
	<-s.done
	// Join heartbeat/health-check (and their in-flight reconnect
	// sub-goroutines) before closing hst, so none of them are still using
	// it once Close returns.
	s.loops.Wait()

	if err := s.hst.Close(); err != nil {
		logging.Warn("error closing host", "error", err)
	}

	s.running = false
	s.hst = nil
	s.pol = nil
	s.done = nil
	s.stopSrv = nil

	return nil
}
