package identity_test

import (
	"os"
	"testing"

	"github.com/henrybarreto/bethrou/node/identity"

	"github.com/libp2p/go-libp2p/core/crypto"
)

func TestManager_LoadOrGenerate_NewKey(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := tmpDir + "/test_node.key"

	mgr := identity.NewManager(keyPath)

	priv, err := mgr.LoadOrGenerate()
	if err != nil {
		t.Fatalf("LoadOrGenerate failed: %v", err)
	}

	if priv == nil {
		t.Fatal("Expected private key, got nil")
	}

	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Fatal("Key file was not created")
	}
}

func TestManager_LoadOrGenerate_ExistingKey(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := tmpDir + "/test_node.key"

	mgr := identity.NewManager(keyPath)
	priv1, err := mgr.LoadOrGenerate()
	if err != nil {
		t.Fatalf("First LoadOrGenerate failed: %v", err)
	}

	priv2, err := mgr.LoadOrGenerate()
	if err != nil {
		t.Fatalf("Second LoadOrGenerate failed: %v", err)
	}

	if !priv1.Equals(priv2) {
		t.Fatal("Loaded key does not match generated key")
	}
}

func TestGenerate(t *testing.T) {
	priv, err := identity.Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if priv == nil {
		t.Fatal("Expected private key, got nil")
	}

	if priv.Type() != crypto.Ed25519 {
		t.Fatalf("Expected Ed25519 key, got %v", priv.Type())
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		key     crypto.PrivKey
		wantErr bool
	}{
		{
			name:    "nil key",
			key:     nil,
			wantErr: true,
		},
		{
			name:    "valid key",
			key:     mustGenerateKey(t),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := identity.Validate(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func mustGenerateKey(t *testing.T) crypto.PrivKey {
	priv, err := identity.Generate()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	return priv
}
