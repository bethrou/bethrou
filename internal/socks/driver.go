package socks

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/bethrou/bethrou/pkg/logging"
	"github.com/bethrou/bethrou/pkg/proxy"
	"github.com/ezh0v/socks5"
)

var _ socks5.Driver = (*Driver)(nil)

type Driver struct {
	proxy   *proxy.Client
	timeout time.Duration
}

func (d *Driver) Dial(network string, address string) (net.Conn, error) {
	ctx := context.Background()
	if d.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d.timeout)
		defer cancel()
	}

	conn, err := d.proxy.DialByStrategy(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("failed to dial through any node: %w", err)
	}

	logging.Trace("dial to node", "address", address, "network", network)

	return conn, nil
}

func (d *Driver) Listen(network string, address string) (net.Listener, error) {
	l, err := net.Listen(network, address)
	if err != nil {
		logging.Error("failed to listen", "error", err, "address", address, "network", network)

		return nil, err
	}

	logging.Debug("listening", "address", address, "network", network)

	return l, nil
}

func (d *Driver) ListenPacket(network string, address string) (net.PacketConn, error) {
	c, err := net.ListenPacket(network, address)
	if err != nil {
		logging.Error("failed to listen packet", "error", err, "address", address, "network", network)

		return nil, err
	}

	logging.Debug("listening packet", "address", address, "network", network)

	return c, nil
}

func (d *Driver) Resolve(network string, address string) (net.Addr, error) {
	switch network {
	case "udp":
		a, err := net.ResolveUDPAddr(network, address)
		if err != nil {
			logging.Error("failed to resolve udp address", "error", err, "address", address, "network", network)

			return nil, err
		}

		logging.Trace("resolved udp address", "address", address, "network", network)

		return a, nil
	case "tcp":
		a, err := net.ResolveTCPAddr(network, address)
		if err != nil {
			logging.Error("failed to resolve tcp address", "error", err, "address", address, "network", network)

			return nil, err
		}

		logging.Trace("resolved tcp address", "address", address, "network", network)

		return a, nil
	default:
		return nil, errors.New("unsupported network")
	}
}
