package proxy

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestServerPeerAuthorization(t *testing.T) {
	allowed := peer.ID("allowed")
	blocked := peer.ID("blocked")

	srv := &Server{
		cfg: ServerConfig{
			AllowedPeers: map[peer.ID]struct{}{
				allowed: {},
			},
		},
	}

	if !srv.isPeerAllowed(allowed) {
		t.Fatal("expected allowed peer to pass authorization")
	}

	if srv.isPeerAllowed(blocked) {
		t.Fatal("expected blocked peer to fail authorization")
	}
}

func TestEffectiveDialTimeout(t *testing.T) {
	srv := &Server{
		cfg: ServerConfig{
			DialTimeout: 10 * time.Second,
		},
	}

	cases := []struct {
		name          string
		requestmillis int64
		want          time.Duration
	}{
		{"request timeout wins when shorter", 1500, 1500 * time.Millisecond},
		{"falls back to server timeout when unset", 0, 10 * time.Second},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := srv.effectiveDialTimeout(tc.requestmillis); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestServerAcquireRespectsLimits(t *testing.T) {
	peerOne := peer.ID("peer-1")
	peerTwo := peer.ID("peer-2")
	srv := &Server{
		cfg: ServerConfig{
			MaxConcurrentStreams:        2,
			MaxConcurrentStreamsPerPeer: 1,
		},
		activeByPeer: make(map[peer.ID]int),
	}

	if err := srv.acquire(peerOne); err != nil {
		t.Fatalf("expected first peer to acquire slot, got %v", err)
	}
	if err := srv.acquire(peerOne); err == nil {
		t.Fatal("expected per-peer limit to reject second stream")
	}
	if err := srv.acquire(peerTwo); err != nil {
		t.Fatalf("expected second peer to acquire slot, got %v", err)
	}
	if err := srv.acquire(peer.ID("peer-3")); err == nil {
		t.Fatal("expected global limit to reject third concurrent stream")
	}
}
