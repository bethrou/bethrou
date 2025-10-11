package socks

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/ezh0v/socks5"
	"github.com/henrybarreto/bethrou/pkg/logging"
	"github.com/henrybarreto/bethrou/pkg/proxy"
)

var _ socks5.Driver = (*Driver)(nil)

type Driver struct {
	proxy *proxy.Client
}

func (d *Driver) Dial(network string, address string) (net.Conn, error) {
	ctx := context.Background()

	conn, err := d.proxy.DialByStrategy(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("failed to dial through any node: %w", err)
	}

	// logging.Logger.Debug("Dial to node", "address", address, "network", network)

	return conn, nil
}

func (d *Driver) Listen(network string, address string) (net.Listener, error) {
	l, err := net.Listen(network, address)
	if err != nil {
		logging.Logger.Error("failed to listen", "error", err, "address", address, "network", network)

		return nil, err
	}

	// logging.Logger.Debug("Listening", "address", address, "network", network)

	return l, nil
}

func (d *Driver) ListenPacket(network string, address string) (net.PacketConn, error) {
	c, err := net.ListenPacket(network, address)
	if err != nil {
		logging.Logger.Error("failed to listen packet", "error", err, "address", address, "network", network)

		return nil, err
	}

	// logging.Logger.Debug("Listening packet", "address", address, "network", network)

	return c, nil
}

func (d *Driver) Resolve(network string, address string) (net.Addr, error) {
	switch network {
	case "udp":
		a, err := net.ResolveUDPAddr(network, address)
		if err != nil {
			logging.Logger.Error("failed to resolve udp address", "error", err, "address", address, "network", network)

			return nil, err
		}

		// logging.Logger.Debug("Resolved udp address", "address", address, "network", network)

		return a, nil
	case "tcp":
		a, err := net.ResolveTCPAddr(network, address)
		if err != nil {
			logging.Logger.Error("failed to resolve tcp address", "error", err, "address", address, "network", network)

			return nil, err
		}

		// logging.Logger.Debug("Resolved tcp address", "address", address, "network", network)

		return a, nil
	default:
		return nil, errors.New("unsupported network")
	}
}
