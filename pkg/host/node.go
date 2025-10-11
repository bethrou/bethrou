package host

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/henrybarreto/bethrou/pkg/logging"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/pnet"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	"github.com/multiformats/go-multiaddr"
)

type NodeConfig struct {
	ListenAddr   string
	PrivateKey   crypto.PrivKey
	RelayMode    bool
	ConnectRelay string
	Key          string
}

type Node struct {
	host  host.Host
	relay *relayv2.Relay
}

type Notifee struct {
	Logger *slog.Logger
}

var _ network.Notifiee = (*Notifee)(nil)

func NewNotifee(logger *slog.Logger) *Notifee {
	return &Notifee{
		Logger: logger,
	}
}

func (n *Notifee) Connected(net network.Network, conn network.Conn) {
	n.Logger.Info("Connected", "local", conn.LocalMultiaddr(), "remote", conn.RemoteMultiaddr())
}

func (n *Notifee) Disconnected(net network.Network, conn network.Conn) {
	n.Logger.Info("Disconnected", "local", conn.LocalMultiaddr(), "remote", conn.RemoteMultiaddr())
}

func (n *Notifee) Listen(net network.Network, ma multiaddr.Multiaddr) {
	n.Logger.Info("Listening on", "addr", ma)
}

func (n *Notifee) ListenClose(net network.Network, ma multiaddr.Multiaddr) {
	n.Logger.Info("Stopped listening on", "addr", ma)
}

func NewNode(cfg NodeConfig) (*Node, error) {
	if err := validateServerConfig(cfg); err != nil {
		return nil, err
	}

	if cfg.Key == "" {
		return nil, fmt.Errorf("network key path is required")
	}

	file, err := os.OpenFile(cfg.Key, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to read network key from %s: %w", cfg.Key, err)
	}

	psk, err := pnet.DecodeV1PSK(file)
	if err != nil {
		return nil, fmt.Errorf("failed to decode psk from %s: %w", filepath.Base(cfg.Key), err)
	}

	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(cfg.ListenAddr),
		libp2p.Identity(cfg.PrivateKey),
		libp2p.PrivateNetwork(psk),
		libp2p.EnableAutoNATv2(),
		libp2p.EnableHolePunching(holepunch.DirectDialTimeout(30 * time.Second)),
	}

	if cfg.RelayMode || cfg.ConnectRelay != "" {
		opts = append(opts, libp2p.EnableRelay())
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	h.Network().Notify(NewNotifee(logging.Logger))

	node := &Node{host: h}

	logging.Logger.Info("Peer ID", "id", h.ID())
	logging.Logger.Info("Listening on", "addrs", h.Addrs())

	if cfg.RelayMode {
		if err := node.StartRelay(); err != nil {
			h.Close()

			return nil, err
		}
	}

	ctx := context.Background()

	if cfg.ConnectRelay != "" {
		if err := node.ConnectRelay(ctx, cfg.ConnectRelay); err != nil {
			logging.Logger.Warn("Warning: failed to connect to relay", "error", err)
		}
	}

	return node, nil
}

func (n *Node) Host() host.Host {
	return n.host
}

func (n *Node) StartRelay() error {
	logging.Logger.Info("Starting relay service on this node")

	relay, err := relayv2.New(n.host)
	if err != nil {
		return fmt.Errorf("failed to start relay service: %w", err)
	}

	n.relay = relay

	logging.Logger.Info("Relay service started successfully")
	logging.Logger.Info("Relay addresses")
	for _, addr := range n.host.Addrs() {
		logging.Logger.Info("address", "addr", fmt.Sprintf("%s/p2p/%s", addr, n.host.ID()))
	}

	return nil
}

func (n *Node) ConnectRelay(ctx context.Context, relayAddr string) error {
	logging.Logger.Info("Connecting to external relay", "relay", relayAddr)

	relayMA, err := multiaddr.NewMultiaddr(relayAddr)
	if err != nil {
		return fmt.Errorf("invalid relay multiaddr: %w", err)
	}

	relayInfo, err := peer.AddrInfoFromP2pAddr(relayMA)
	if err != nil {
		return fmt.Errorf("failed to parse relay address: %w", err)
	}

	if err := n.host.Connect(ctx, *relayInfo); err != nil {
		return fmt.Errorf("failed to connect to relay: %w", err)
	}

	logging.Logger.Info("Connected to external relay successfully")

	_, err = client.Reserve(ctx, n.host, *relayInfo)
	if err != nil {
		return fmt.Errorf("failed to reserve relay slot: %w", err)
	}

	return nil
}

func (n *Node) Close() error {
	if n.host != nil {
		return n.host.Close()
	}
	return nil
}

func validateServerConfig(cfg NodeConfig) error {
	if cfg.ListenAddr == "" {
		return fmt.Errorf("listen address is required")
	}

	if cfg.PrivateKey == nil {
		return fmt.Errorf("private key is required")
	}

	return nil
}
