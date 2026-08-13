package proxy

import (
	"testing"
	"time"
)

func TestDestinationPolicyBlocksLoopbackAndPrivateByDefault(t *testing.T) {
	policy := DestinationPolicy{}

	cases := []string{
		"127.0.0.1:80",
		"localhost:443",
		"10.0.0.5:5432",
		"169.254.169.254:80",
	}

	for _, addr := range cases {
		if _, err := policy.Resolve(addr, time.Second); err == nil {
			t.Fatalf("expected %q to be blocked", addr)
		}
	}
}

func TestDestinationPolicyAllowsExplicitlyEnabledRanges(t *testing.T) {
	policy := DestinationPolicy{
		AllowPrivate:   true,
		AllowLoopback:  true,
		AllowLinkLocal: true,
		AllowMetadata:  true,
	}

	cases := []string{
		"127.0.0.1:80",
		"10.0.0.5:5432",
		"169.254.169.254:80",
	}

	for _, addr := range cases {
		if _, err := policy.Resolve(addr, time.Second); err != nil {
			t.Fatalf("expected %q to be allowed, got %v", addr, err)
		}
	}
}
