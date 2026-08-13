package config

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/bethrou/bethrou/pkg/config"
)

type ServerConfig struct {
	ListenAddr        string `yaml:"listen"`
	Auth              bool   `yaml:"auth"`
	User              string `yaml:"user,omitempty"`
	Pass              string `yaml:"pass,omitempty"`
	AllowInsecureBind bool   `yaml:"allow_insecure_bind,omitempty"`
}

func (s *ServerConfig) Validate() error {
	if s.ListenAddr == "" {
		return errors.New("SOCKS listen address is required")
	}

	if s.Auth {
		if s.User == "" || s.Pass == "" {
			return errors.New("SOCKS auth enabled but user or pass is empty")
		}
	}

	if !s.Auth && !s.AllowInsecureBind && isNonLoopbackBind(s.ListenAddr) {
		return errors.New("refusing non-loopback SOCKS bind without auth; set auth or allow_insecure_bind")
	}

	return nil
}

// String redacts Pass so ServerConfig never lands in logs verbatim.
func (s *ServerConfig) String() string {
	pass := ""
	if s.Pass != "" {
		pass = "[redacted]"
	}
	return fmt.Sprintf("ServerConfig{ListenAddr: %s, Auth: %t, User: %s, Pass: %s, AllowInsecureBind: %t}",
		s.ListenAddr, s.Auth, s.User, pass, s.AllowInsecureBind)
}

func isNonLoopbackBind(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return true
	}

	switch host {
	case "127.0.0.1", "::1", "localhost":
		return false
	case "", "0.0.0.0", "::":
		return true
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return true
	}

	return !ip.IsLoopback()
}

type RoutingConfig struct {
	Strategy string `yaml:"strategy"`
	Health   string `yaml:"health"`
	Timeout  string `yaml:"timeout"`
}

func (s *RoutingConfig) Validate() error {
	switch s.Strategy {
	case "", "random", "fastest", "round-robin":

	default:
		return fmt.Errorf("unsupported routing strategy: %s", s.Strategy)
	}

	if s.Health != "" {
		if _, err := time.ParseDuration(s.Health); err != nil {
			return fmt.Errorf("invalid routing.health duration: %w", err)
		}
	}

	if s.Timeout != "" {
		if _, err := time.ParseDuration(s.Timeout); err != nil {
			return fmt.Errorf("invalid routing.timeout duration: %w", err)
		}
	}

	return nil
}

type LogConfig = config.LogConfig

// ClientConfig configures a running SOCKS5 client. The client must be
// enrolled with the Bethrou control plane (see the "enroll" command) before
// it can connect: on every start, it authenticates to APIURL with its
// persistent identity key and fetches its role fresh (nothing about
// enrollment is cached to disk). TargetNodes optionally pins which exit
// node peer IDs to use; left empty, the client uses every node the control
// plane reports for this account instead — no local node configuration
// needed.
type ClientConfig struct {
	Key         string         `yaml:"key"`
	Server      *ServerConfig  `yaml:"server"`
	Routing     *RoutingConfig `yaml:"routing"`
	IdentityKey string         `yaml:"identity_key"`
	APIURL      string         `yaml:"api_url"`
	TargetNodes []string       `yaml:"target_nodes"`
	Log         *LogConfig     `yaml:"log"`
}

func (c *ClientConfig) Validate() error {
	if c.Key == "" {
		return errors.New("network key is required")
	}

	if c.Server == nil {
		return errors.New("server config is required")
	}

	if err := c.Server.Validate(); err != nil {
		return fmt.Errorf("server config validation failed: %w", err)
	}

	if c.Routing == nil {
		return errors.New("routing config is required")
	}

	if err := c.Routing.Validate(); err != nil {
		return fmt.Errorf("routing config validation failed: %w", err)
	}

	if c.APIURL == "" {
		return errors.New("api_url is required (run 'client enroll' first)")
	}

	// TargetNodes is optional: if unset, Connect discovers every exit node
	// on the account from the control plane instead. Explicitly setting it
	// still works, as a way to pick a subset.

	if c.Log == nil {
		return errors.New("log config is required")
	}

	if err := c.Log.Validate(); err != nil {
		return fmt.Errorf("log config validation failed: %w", err)
	}

	return nil
}

func (c *ClientConfig) String() string {
	key := ""
	if c.Key != "" {
		key = "[redacted]"
	}
	return fmt.Sprintf("ClientConfig{Key: %s, Server: %+v, Routing: %+v, APIURL: %s, TargetNodes: %d, Log: %+v}",
		key, c.Server, c.Routing, c.APIURL, len(c.TargetNodes), c.Log)
}
