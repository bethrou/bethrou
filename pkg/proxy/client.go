package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/henrybarreto/bethrou/pkg/config"
	"github.com/henrybarreto/bethrou/pkg/logging"
	pkgnetwork "github.com/henrybarreto/bethrou/pkg/network"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// Connection represents a connection to a proxy node
type Connection struct {
	PeerID  peer.ID
	Addr    string
	Latency time.Duration
}

// Client is the client-side proxy dialer that connects to exit nodes
type Client struct {
	Host host.Host
	Pool *Pool
}

// NewClient creates a new client-side proxy dialer
func NewClient(h host.Host, p *Pool) *Client {
	return &Client{
		Host: h,
		Pool: p,
	}
}

// Ping checks the latency to a proxy node by opening and closing a stream
func (d *Client) Ping(ctx context.Context, conn *Connection) (time.Duration, error) {
	start := time.Now()

	stream, err := d.Host.NewStream(ctx, conn.PeerID, PingProtocolID)
	if err != nil {
		return 0, fmt.Errorf("probe new stream failed: %w", err)
	}

	_ = stream.Close()

	return time.Since(start), nil
}

// Dial establishes a proxy connection through a specific exit node
func (d *Client) Dial(ctx context.Context, peerID peer.ID, addr string) (net.Conn, error) {
	stream, err := d.Host.NewStream(network.WithAllowLimitedConn(ctx, "ProxyProtocolID"), peerID, ProxyProtocolID)
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}

	req := Request{ProxyAddress: addr}
	if err := json.NewEncoder(stream).Encode(req); err != nil {
		_ = stream.Close()
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	var resp ProxyResponse
	if err := json.NewDecoder(stream).Decode(&resp); err != nil {
		_ = stream.Close()
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.Status != "ok" {
		_ = stream.Close()
		return nil, fmt.Errorf("proxy failed: %s", resp.Message)
	}

	return &pkgnetwork.Adapter{Stream: stream}, nil
}

// dialConnection is a convenience method that dials using a Connection struct
func (d *Client) dialConnection(ctx context.Context, conn *Connection, addr string) (net.Conn, error) {
	if conn == nil {
		return nil, errors.New("connection is nil")
	}

	return d.Dial(ctx, conn.PeerID, addr)
}

func (d *Client) DialRandom(ctx context.Context, addr string) (net.Conn, error) {
	conn := d.Pool.SelectRandom()
	if conn == nil {
		return nil, errors.New("no exit nodes available")
	}

	return d.dialConnection(ctx, conn, addr)
}

func (d *Client) DialFastest(ctx context.Context, addr string) (net.Conn, error) {
	conns := d.Pool.SelectFastest()
	if conns == nil {
		return nil, errors.New("no exit nodes available")
	}

	return d.dialConnection(ctx, conns, addr)
}

func (d *Client) DialRoundRobin(ctx context.Context, addr string) (net.Conn, error) {
	conn := d.Pool.SelectRoundRobin()
	if conn == nil {
		return nil, errors.New("no exit nodes available")
	}

	return d.dialConnection(ctx, conn, addr)
}

// DialByStrategy dials an exit node based on the pool's current strategy.
func (d *Client) DialByStrategy(ctx context.Context, addr string) (net.Conn, error) {
	switch d.Pool.GetStrategy() {
	case RandomStrategy:
		return d.DialRandom(ctx, addr)
	case FastestStrategy:
		return d.DialFastest(ctx, addr)
	case RoundRobinStrategy:
		return d.DialRoundRobin(ctx, addr)
	default:
		return nil, errors.New("unknown dialing strategy")
	}
}

func (p *Client) connect(ctx context.Context, node config.NodeConfig) error {
	addrs := make([]string, 0, len(node.Addrs)+1)
	addrs = append(addrs, node.Addrs...)

	if node.Relay != "" {
		logging.Logger.Info("Node has relay address but relay connections disabled", "node", node.ID)

		relayMA, err := multiaddr.NewMultiaddr(node.Relay)
		if err != nil {
			return fmt.Errorf("invalid relay multiaddr: %w", err)
		}

		relayInfo, err := peer.AddrInfoFromP2pAddr(relayMA)
		if err != nil {
			return fmt.Errorf("invalid relay p2p address: %w", err)
		}

		if err := p.Host.Connect(ctx, *relayInfo); err != nil {
			return fmt.Errorf("failed to connect to relay: %w", err)
		}

		logging.Logger.Info("Connected to relay", "relay", relayInfo.ID)

		circuitAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("%s/p2p-circuit/p2p/%s", relayMA.String(), node.ID))
		if err != nil {
			return fmt.Errorf("failed to create circuit address: %w", err)
		}

		logging.Logger.Info("Attempting to connect using circuit address", "addr", circuitAddr.String())

		nodeInfo, err := peer.AddrInfoFromP2pAddr(circuitAddr)
		if err != nil {
			return fmt.Errorf("invalid circuit p2p address: %w", err)
		}

		if err := p.Host.Connect(ctx, *nodeInfo); err != nil {
			return fmt.Errorf("failed to connect to node via relay: %w", err)
		}

		logging.Logger.Info("Connected to node via relay", "node", node.ID, "relay", relayInfo.ID)

		p.Pool.Add(nodeInfo.ID, circuitAddr.String())

		return nil
	}

	logging.Logger.Info("Attempting to connect to node", "node", node.ID, "addrs_count", len(addrs))
	logging.Logger.Debug("Node addresses", "addrs", addrs)

	var connected bool
	var lastErr error
	for _, addr := range addrs {
		nodeMA, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			lastErr = err
			continue
		}

		nodeInfo, err := peer.AddrInfoFromP2pAddr(nodeMA)
		if err != nil {
			lastErr = err
			continue
		}

		if err := p.Host.Connect(ctx, *nodeInfo); err != nil {
			lastErr = err
			continue
		}

		p.Pool.Add(nodeInfo.ID, addr)

		connected = true
		break
	}

	if !connected {
		return errors.New("failed to connect to node: " + lastErr.Error())
	}

	return nil
}

func (p *Client) Connect(ctx context.Context, nodes []config.NodeConfig) error {
	for _, node := range nodes {
		if err := p.connect(ctx, node); err != nil {
			return err
		}
	}

	return nil
}
