package identity

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"

	"github.com/libp2p/go-libp2p/core/crypto"
)

const (
	defaultKeyFile = "identity.key"
	keyFilePerms   = 0o600
)

type Manager struct {
	keyPath string
}

func NewManager(keyPath string) *Manager {
	if keyPath == "" {
		keyPath = defaultKeyFile
	}
	return &Manager{keyPath: keyPath}
}

func (m *Manager) LoadOrGenerate() (crypto.PrivKey, error) {
	if _, err := os.Stat(m.keyPath); os.IsNotExist(err) {
		return m.generateAndSave()
	}

	return m.load()
}

func (m *Manager) load() (crypto.PrivKey, error) {
	b, err := os.ReadFile(m.keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	priv, err := crypto.UnmarshalPrivateKey(b)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal private key: %w", err)
	}

	return priv, nil
}

func (m *Manager) generateAndSave() (crypto.PrivKey, error) {
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	b, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	// O_EXCL: if another process won the race and created the key file
	// between our earlier os.Stat and here, fail closed and load its key
	// instead of silently overwriting it (which would orphan whatever
	// already trusted the file that lost the race).
	f, err := os.OpenFile(m.keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, keyFilePerms)
	if errors.Is(err, os.ErrExist) {
		return m.load()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create key file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(b); err != nil {
		return nil, fmt.Errorf("failed to write key file: %w", err)
	}

	return priv, nil
}

func Generate() (crypto.PrivKey, error) {
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}
	return priv, nil
}

func Validate(priv crypto.PrivKey) error {
	if priv == nil {
		return errors.New("private key is nil")
	}
	return nil
}
