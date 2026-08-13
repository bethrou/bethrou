package proxy

import "testing"

func TestNewPoolPreservesStrategy(t *testing.T) {
	pool := NewPool(RoundRobinStrategy)

	if got := pool.GetStrategy(); got != RoundRobinStrategy {
		t.Fatalf("expected strategy %q, got %q", RoundRobinStrategy, got)
	}
}
