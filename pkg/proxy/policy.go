package proxy

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// DestinationPolicy controls which outbound targets a node may dial.
type DestinationPolicy struct {
	AllowPrivate   bool
	AllowLoopback  bool
	AllowLinkLocal bool
	AllowMulticast bool
	AllowMetadata  bool
}

var metadataIPs = map[string]struct{}{
	"169.254.169.254": {},
}

// Resolve validates the destination address against the policy and returns
// the resolved, policy-approved TCP address to dial. Callers must dial the
// returned address literally (not the original hostname) to avoid a
// DNS-rebinding TOCTOU between validation and connection.
func (p DestinationPolicy) Resolve(address string, timeout time.Duration) (*net.TCPAddr, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid destination address %q: %w", address, err)
	}

	if isLocalhostHost(host) && !p.AllowLoopback {
		return nil, fmt.Errorf("destination %q is loopback and blocked by policy", host)
	}

	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve destination host %q: %w", host, err)
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("destination host %q resolved to no addresses", host)
	}

	var approved net.IP
	for _, ipAddr := range ips {
		ip := ipAddr.IP
		if ip.IsUnspecified() {
			return nil, fmt.Errorf("destination %q resolved to unspecified address %s", host, ip)
		}
		if ip.IsLoopback() && !p.AllowLoopback {
			return nil, fmt.Errorf("destination %q resolved to loopback address %s", host, ip)
		}
		if ip.IsPrivate() && !p.AllowPrivate {
			return nil, fmt.Errorf("destination %q resolved to private address %s", host, ip)
		}
		if (ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) && !p.AllowLinkLocal {
			return nil, fmt.Errorf("destination %q resolved to link-local address %s", host, ip)
		}
		if ip.IsMulticast() && !p.AllowMulticast {
			return nil, fmt.Errorf("destination %q resolved to multicast address %s", host, ip)
		}
		if _, ok := metadataIPs[ip.String()]; ok && !p.AllowMetadata {
			return nil, fmt.Errorf("destination %q resolved to metadata address %s", host, ip)
		}
		if approved == nil {
			approved = ip
		}
	}

	tcpPort, err := net.LookupPort("tcp", port)
	if err != nil {
		return nil, fmt.Errorf("invalid destination port %q: %w", port, err)
	}

	return &net.TCPAddr{IP: approved, Port: tcpPort}, nil
}

func isLocalhostHost(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	return strings.EqualFold(host, "localhost")
}
