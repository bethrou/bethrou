package config

import "testing"

func TestDiscoveryConfigRequiresTLSByDefault(t *testing.T) {
	cfg := &DiscoveryConfig{
		Enabled: true,
		Address: "redis://localhost:6379",
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected plaintext discovery transport to be rejected")
	}
}

func TestDiscoveryConfigAcceptsRediss(t *testing.T) {
	cfg := &DiscoveryConfig{
		Enabled: true,
		Address: "rediss://localhost:6379",
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected rediss discovery transport to validate, got %v", err)
	}
}

func TestRedactURLSecrets(t *testing.T) {
	got := RedactURLSecrets("rediss://user:secret@example.com:6379/0")
	want := "rediss://user:REDACTED@example.com:6379/0"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
