package host

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/pnet"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	"github.com/multiformats/go-multiaddr"
)

// Client wraps a libp2p host with common functionality
type Client struct {
	host host.Host
}

// NewClient creates a new libp2p host with the given configuration
func NewClient(keyPath string) (*Client, error) {
	if keyPath == "" {
		return nil, fmt.Errorf("network key path is required")
	}

	// Load the pre-shared key
	psk, err := loadPSK(keyPath)
	if err != nil {
		return nil, err
	}

	// Build libp2p options
	opts := []libp2p.Option{
		libp2p.NoListenAddrs,
		libp2p.PrivateNetwork(psk),
		libp2p.EnableRelay(),
		libp2p.EnableAutoNATv2(),
		libp2p.EnableHolePunching(holepunch.DirectDialTimeout(30 * time.Second)),
	}

	// Create the libp2p host
	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	return &Client{host: h}, nil
}

// Host returns the underlying libp2p host
func (h *Client) Host() host.Host {
	return h.host
}

// ID returns the peer ID of this host
func (h *Client) ID() peer.ID {
	return h.host.ID()
}

// Connect connects to a peer at the given multiaddr
func (h *Client) Connect(ctx context.Context, addr string) (peer.ID, error) {
	ma, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return "", fmt.Errorf("invalid multiaddr: %w", err)
	}

	addrInfo, err := peer.AddrInfoFromP2pAddr(ma)
	if err != nil {
		return "", fmt.Errorf("failed to parse peer address: %w", err)
	}

	if err := h.host.Connect(ctx, *addrInfo); err != nil {
		return "", fmt.Errorf("failed to connect to peer: %w", err)
	}

	return addrInfo.ID, nil
}

// ConnectMultiple tries to connect to a peer using multiple addresses
// Returns the peer ID and the successful address
func (h *Client) ConnectMultiple(ctx context.Context, addrs []string) (peer.ID, string, error) {
	if len(addrs) == 0 {
		return "", "", fmt.Errorf("no addresses provided")
	}

	var lastErr error
	for _, addr := range addrs {
		peerID, err := h.Connect(ctx, addr)
		if err != nil {
			lastErr = err
			continue
		}

		return peerID, addr, nil
	}

	return "", "", fmt.Errorf("all connection attempts failed, last error: %w", lastErr)
}

// Close closes the host
func (h *Client) Close() error {
	if h.host != nil {
		return h.host.Close()
	}
	return nil
}

// loadPSK loads a pre-shared key from a file
func loadPSK(path string) (pnet.PSK, error) {
	file, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open network key file: %w", err)
	}
	defer file.Close()

	psk, err := pnet.DecodeV1PSK(file)
	if err != nil {
		return nil, fmt.Errorf("failed to decode psk: %w", err)
	}

	return psk, nil
}
