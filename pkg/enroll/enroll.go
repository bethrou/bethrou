// Package enroll implements the one-time enrollment exchange between an
// agent and the control plane. Nothing here is persisted to disk: the
// caller's persistent libp2p identity key is the only credential that needs
// to survive a restart. On every subsequent run, use pkg/control's
// Client.Me to fetch role/relay info fresh into memory instead of caching
// this result.
package enroll

import (
	"context"
	"fmt"

	"github.com/bethrou/bethrou/pkg/control"
	"github.com/libp2p/go-libp2p/core/crypto"
)

// Result is the outcome of a successful enrollment.
type Result struct {
	ID   string
	Role string
}

// Enroll redeems a one-time enrollment token, registering priv's libp2p
// identity as a peer under the token's owning account.
func Enroll(ctx context.Context, apiURL, token string, priv crypto.PrivKey) (*Result, error) {
	cc, err := control.New(apiURL, priv)
	if err != nil {
		return nil, fmt.Errorf("failed to build control-plane client: %w", err)
	}

	result, err := cc.Enroll(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("enrollment failed: %w", err)
	}

	return &Result{ID: result.ID, Role: result.Role}, nil
}
