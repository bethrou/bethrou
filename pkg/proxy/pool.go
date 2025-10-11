package proxy

import (
	"math/rand"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

type PoolStrategy string

const (
	RandomStrategy     PoolStrategy = "random"
	FastestStrategy    PoolStrategy = "latency"
	RoundRobinStrategy PoolStrategy = "round-robin"
)

type Pool struct {
	conns    []*Connection
	mu       sync.RWMutex
	strategy PoolStrategy
	rrIndex  int
}

func NewPool(strategy PoolStrategy) *Pool {
	return &Pool{
		conns:    make([]*Connection, 0),
		strategy: RandomStrategy,
	}
}

func (p *Pool) SetStrategy(s PoolStrategy) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.strategy = s
}

func (p *Pool) GetStrategy() PoolStrategy {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.strategy
}

func (p *Pool) SelectByStrategy(strategy PoolStrategy) *Connection {
	switch strategy {
	case RandomStrategy:
		return p.SelectRandom()
	case FastestStrategy:
		return p.SelectFastest()
	case RoundRobinStrategy:
		return p.SelectRoundRobin()
	default:
		return p.SelectRandom()
	}
}

func (p *Pool) Add(peerID peer.ID, addr string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.conns = append(p.conns, &Connection{
		PeerID:  peerID,
		Addr:    addr,
		Latency: 0,
	})
}

func (p *Pool) Remove(peerID peer.ID) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, conn := range p.conns {
		if conn.PeerID == peerID {
			p.conns = append(p.conns[:i], p.conns[i+1:]...)
			return
		}
	}
}

func (p *Pool) All() []*Connection {
	p.mu.RLock()
	defer p.mu.RUnlock()

	conns := make([]*Connection, len(p.conns))
	copy(conns, p.conns)

	return conns
}

func (p *Pool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return len(p.conns)
}

func (p *Pool) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.conns = make([]*Connection, 0)
}

func (p *Pool) UpdateLatency(peerID peer.ID, latency time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range p.conns {
		if conn.PeerID == peerID {
			conn.Latency = latency
			return
		}
	}
}

func (p *Pool) SelectRandom() *Connection {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.conns) == 0 {
		return nil
	}

	idx := rand.Intn(len(p.conns))
	return p.conns[idx]
}

func (p *Pool) SelectFastest() *Connection {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.conns) == 0 {
		return nil
	}

	var best *Connection
	for _, conn := range p.conns {
		if best == nil || (conn.Latency > 0 && conn.Latency < best.Latency) {
			best = conn
		}
	}

	if best == nil || best.Latency == 0 {
		idx := rand.Intn(len(p.conns))
		return p.conns[idx]
	}

	return best
}

func (p *Pool) SelectRoundRobin() *Connection {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.conns) == 0 {
		return nil
	}

	conn := p.conns[p.rrIndex%len(p.conns)]
	p.rrIndex = (p.rrIndex + 1) % len(p.conns)

	return conn
}
