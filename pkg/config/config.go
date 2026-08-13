package config

import (
	"errors"
	"fmt"
	"net/url"
)

// NodeConfig represents a network node with its addresses and optional relay
type NodeConfig struct {
	ID    string   `yaml:"id" json:"id"`
	Addrs []string `yaml:"addrs" json:"addrs"`
	Relay string   `yaml:"relay,omitempty" json:"relay,omitempty"`
}

// Validate checks if the node configuration is valid
func (n *NodeConfig) Validate() error {
	if n.ID == "" {
		return errors.New("node ID is required")
	}

	if len(n.Addrs) == 0 && n.Relay == "" {
		return errors.New("at least one address or relay is required")
	}

	return nil
}

// DiscoveryConfig contains configuration for the discovery service
type DiscoveryConfig struct {
	Enabled                bool   `yaml:"enabled"`
	Address                string `yaml:"address"`
	Topic                  string `yaml:"topic"`
	Timeout                string `yaml:"timeout"`
	User                   string `yaml:"user"`
	Pass                   string `yaml:"pass"`
	AllowInsecureTransport bool   `yaml:"allow_insecure_transport,omitempty"`
	TLSServerName          string `yaml:"tls_server_name,omitempty"`
	TLSCAFile              string `yaml:"tls_ca_file,omitempty"`
}

// Validate checks if the discovery configuration is valid
func (d *DiscoveryConfig) Validate() error {
	if !d.Enabled {
		return nil
	}

	if d.Address == "" {
		return errors.New("discovery address is required when discovery is enabled")
	}

	u, err := url.Parse(d.Address)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return errors.New("discovery address must be a full redis:// or rediss:// URL")
	}

	switch u.Scheme {
	case "rediss":
	case "redis":
		if !d.AllowInsecureTransport {
			return errors.New("discovery requires TLS; use rediss:// or explicitly allow insecure transport")
		}
	default:
		return fmt.Errorf("unsupported discovery scheme: %s", u.Scheme)
	}

	if (d.TLSServerName != "" || d.TLSCAFile != "") && u.Scheme != "rediss" {
		return errors.New("TLS discovery options require a rediss:// address")
	}

	return nil
}

// String returns a string representation of DiscoveryConfig
func (d *DiscoveryConfig) String() string {
	return fmt.Sprintf("DiscoveryConfig{Enabled: %v, Address: %s, Topic: %s}",
		d.Enabled, RedactURLSecrets(d.Address), d.Topic)
}

// LogConfig contains logging configuration for the application
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Validate ensures LogConfig has acceptable values
func (l *LogConfig) Validate() error {
	if l.Level == "" {
		l.Level = "info"
	}

	if l.Format == "" {
		l.Format = "text"
	}

	if l.Format != "text" && l.Format != "json" {
		return fmt.Errorf("invalid log format: %s", l.Format)
	}

	return nil
}

func (l *LogConfig) String() string {
	return fmt.Sprintf("LogConfig{Level: %s, Format: %s}", l.Level, l.Format)
}

func RedactURLSecrets(raw string) string {
	if raw == "" {
		return raw
	}

	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}

	username := u.User.Username()
	if username == "" {
		u.User = url.UserPassword("REDACTED", "REDACTED")
	} else {
		u.User = url.UserPassword(username, "REDACTED")
	}

	return u.String()
}
