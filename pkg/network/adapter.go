package network

import (
	"net"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
)

// Adapter adapts a libp2p Stream to a net.Conn interface
type Adapter struct {
	network.Stream
}

// LocalAddr returns the local network address
func (a *Adapter) LocalAddr() net.Addr {
	return &Addr{s: "libp2p"}
}

// RemoteAddr returns the remote network address
func (a *Adapter) RemoteAddr() net.Addr {
	return &Addr{s: "libp2p"}
}

// SetDeadline sets both read and write deadlines
func (a *Adapter) SetDeadline(t time.Time) error {
	return a.Stream.SetDeadline(t)
}

// SetReadDeadline sets the read deadline
func (a *Adapter) SetReadDeadline(t time.Time) error {
	return a.Stream.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline
func (a *Adapter) SetWriteDeadline(t time.Time) error {
	return a.Stream.SetWriteDeadline(t)
}

// Addr represents a libp2p network address
type Addr struct {
	s string
}

// Network returns the address network type
func (a *Addr) Network() string {
	return a.s
}

// String returns the string representation of the address
func (a *Addr) String() string {
	return a.s
}
